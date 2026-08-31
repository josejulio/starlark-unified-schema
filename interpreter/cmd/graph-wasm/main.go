//go:build js && wasm

// Command graph-wasm is the in-browser schema compiler. Built for GOOS=js
// GOARCH=wasm, it registers a single global function, kesselCompile, that turns
// a set of in-memory Starlark source files into the interactive graph the
// playground renders — the whole pipeline (Starlark -> graph.json -> Cytoscape
// elements) running client-side with no server.
//
// It reuses the exact same code paths as the native tools: lang.CompileGraph
// (the Processor + GraphVisitor) produces graph.json, and web.Elements applies
// the one authoritative graph.json -> elements transform. The browser never
// re-implements either, so a compile in the page matches the CLI byte for byte.
//
// Build: GOOS=js GOARCH=wasm go build -o graph-playground.wasm ./cmd/graph-wasm
package main

import (
	"syscall/js"

	"github.com/project-kessel/starlark-unified-schema/internal/lang"
	"github.com/project-kessel/starlark-unified-schema/internal/web"
)

// compile is the JS-facing entry point. It takes one argument: an object mapping
// each schema file path (relative to the schema root, e.g. "kessel.star") to its
// source text. It returns an object:
//
//	{ ok: true,  graph: "<graph.json>", elements: "<cytoscape elements json>" }
//	{ ok: false, error: "<message>" }
//
// Errors (including panics) are returned as structured data rather than thrown,
// so the page can render them inline in the editor.
func compile(this js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = failure(js.ValueOf(r).String())
		}
	}()

	if len(args) < 1 || args[0].Type() != js.TypeObject {
		return failure("kesselCompile expects one argument: an object of {path: source}")
	}

	files := map[string][]byte{}
	keys := js.Global().Get("Object").Call("keys", args[0])
	for i := 0; i < keys.Length(); i++ {
		name := keys.Index(i).String()
		files[name] = []byte(args[0].Get(name).String())
	}

	graph, err := lang.CompileGraph(files)
	if err != nil {
		return failure(err.Error())
	}

	elements, err := web.Elements(graph)
	if err != nil {
		return failure(err.Error())
	}

	return map[string]any{
		"ok":       true,
		"graph":    string(graph),
		"elements": string(elements),
	}
}

// explainCheck is the JS-facing entry point for check-cost analysis. It takes two
// arguments — a compiled graph.json string and a "REPORTER/TYPE#RELATION" (or
// "TYPE#RELATION" when unambiguous) target — and returns an object:
//
//	{ ok: true,  check: "<proof-tree json>" }
//	{ ok: false, error: "<message>" }
//
// The proof tree is byte-identical to `graph-analyze -check -format json`, since
// both call analyze.ExplainCheck via web.ExplainCheck. The playground calls this
// per permission to annotate the inspector with read cost.
func explainCheck(this js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = failure(js.ValueOf(r).String())
		}
	}()

	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return failure("kesselExplainCheck expects two string arguments: graph.json and REPORTER/TYPE#RELATION")
	}

	check, err := web.ExplainCheck([]byte(args[0].String()), args[1].String())
	if err != nil {
		return failure(err.Error())
	}

	return map[string]any{
		"ok":    true,
		"check": string(check),
	}
}

// checkReachable is the JS-facing entry point for reachability verification. It
// takes two arguments — a compiled graph.json string and a
// "REPORTER/TYPE#RELATION@REPORTER/TYPE" target — and returns an object:
//
//	{ ok: true,  reach: "<verdict json>" }
//	{ ok: false, error: "<message>" }
//
// The verdict is byte-identical to `graph-analyze -reach -format json`, since both
// call analyze.CheckReachable via web.CheckReachable. The playground uses this to
// verify and highlight reachability paths.
func checkReachable(this js.Value, args []js.Value) (result any) {
	defer func() {
		if r := recover(); r != nil {
			result = failure(js.ValueOf(r).String())
		}
	}()

	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return failure("kesselCheckReach expects two string arguments: graph.json and REPORTER/TYPE#RELATION@REPORTER/TYPE")
	}

	reach, err := web.CheckReachable([]byte(args[0].String()), args[1].String())
	if err != nil {
		return failure(err.Error())
	}

	return map[string]any{
		"ok":    true,
		"reach": string(reach),
	}
}

func failure(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}

func main() {
	js.Global().Set("kesselCompile", js.FuncOf(compile))
	js.Global().Set("kesselExplainCheck", js.FuncOf(explainCheck))
	js.Global().Set("kesselCheckReach", js.FuncOf(checkReachable))
	// Keep the Go runtime alive so the exported functions stay callable.
	select {}
}
