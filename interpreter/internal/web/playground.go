package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// defaultWASMURL is the relative URL the playground page fetches the compiled
// WASM binary from at runtime. graph-playground writes it next to the HTML.
const defaultWASMURL = "graph-playground.wasm"

// PlaygroundOptions controls BuildPlayground.
type PlaygroundOptions struct {
	// Layout is the Cytoscape layout name (e.g. "dagre"); empty defaults to dagre.
	Layout string
	// WASMURL is the relative URL the page fetches the compiled compiler from;
	// empty defaults to graph-playground.wasm.
	WASMURL string
}

// BuildPlayground assembles the live playground page from a set of Starlark
// schema source files, keyed by path relative to the schema root (e.g.
// "kessel.star", "workspace/reporters/features/workspace.star"). Unlike Build —
// which is a pure consumer of a finished graph.json — this page carries the
// schema *source* and compiles it in the browser: the sources are inlined to
// seed the editor and feed the WASM compiler (window.kesselCompile), which runs
// the same lang.CompileGraph + web.Elements pipeline the CLI uses.
//
// The compiled WASM binary and Go's wasm_exec.js are loaded as sidecars (they
// are large and toolchain-tied), so the page must be served over http.
func BuildPlayground(files map[string][]byte, opts PlaygroundOptions) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no schema files provided")
	}

	layout := opts.Layout
	if layout == "" {
		layout = defaultLayout
	}
	wasmURL := opts.WASMURL
	if wasmURL == "" {
		wasmURL = defaultWASMURL
	}

	// Inline the sources as a JSON object of path -> source. json.Marshal sorts
	// map keys, so the output is deterministic.
	strFiles := make(map[string]string, len(files))
	for name, contents := range files {
		strFiles[name] = string(contents)
	}
	filesJSON, err := marshalFiles(strFiles)
	if err != nil {
		return "", err
	}
	// Guard against a "</script>" sequence in the sources closing the data tag.
	safeFiles := strings.ReplaceAll(filesJSON, "</", "<\\/")

	page := playgroundHTML
	page = strings.Replace(page, placeholderCodemirrorCSS, codemirrorCSS, 1)
	page = strings.Replace(page, placeholderCodemirrorTheme, codemirrorThemeCSS, 1)
	page = strings.Replace(page, placeholderCytoscape, cytoscapeJS, 1)
	page = strings.Replace(page, placeholderDagre, dagreJS, 1)
	page = strings.Replace(page, placeholderCytoscapeDagre, cytoscapeDagreJS, 1)
	page = strings.Replace(page, placeholderCodemirrorJS, codemirrorJS, 1)
	page = strings.Replace(page, placeholderCodemirrorPython, codemirrorPythonJS, 1)
	page = strings.Replace(page, placeholderFiles, safeFiles, 1)
	page = strings.Replace(page, placeholderLayout, layout, 1)
	page = strings.Replace(page, placeholderWASMURL, wasmURL, 1)
	page = strings.Replace(page, placeholderRender, renderJS, 1)
	page = strings.Replace(page, placeholderPlaygroundApp, playgroundJS, 1)
	return page, nil
}

// marshalFiles renders the path -> source map as indented JSON for inlining.
func marshalFiles(files map[string]string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(files); err != nil {
		return "", fmt.Errorf("marshaling files: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
