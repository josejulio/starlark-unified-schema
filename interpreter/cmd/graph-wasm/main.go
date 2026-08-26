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

func failure(msg string) map[string]any {
	return map[string]any{"ok": false, "error": msg}
}

func main() {
	js.Global().Set("kesselCompile", js.FuncOf(compile))
	// Keep the Go runtime alive so the exported function stays callable.
	select {}
}
