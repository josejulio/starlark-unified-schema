package lang

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/project-kessel/starlark-unified-schema/internal/output"
)

// CompileGraph builds the canonical graph.json (see GRAPH.md) from an in-memory
// set of Starlark source files, keyed by path relative to the schema root (e.g.
// "kessel.star", "workspace/reporters/features/workspace.star"). It touches no
// filesystem, so it runs unchanged inside a WASM sandbox — this is the seam the
// in-browser producer (cmd/graph-wasm) compiles through.
//
// The caller must include the kessel.star prelude in files; schema modules load
// it via load("kessel.star", ...). Modules are processed in sorted path order so
// the result is deterministic regardless of map iteration order (GraphVisitor
// also sorts its nodes and edges), yielding output byte-identical to the native
// filesystem path in cmd/interpreter for the same inputs.
func CompileGraph(files map[string][]byte) ([]byte, error) {
	reader := newInMemorySourceFileReader("schema")

	names := make([]string, 0, len(files))
	for name, contents := range files {
		if err := reader.AddFile(name, contents); err != nil {
			return nil, fmt.Errorf("adding %s: %w", name, err)
		}
		if filepath.Ext(name) == ".star" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	loader := newLoaderForReader("schema", reader)
	processor := NewProcessor(loader)

	graph := output.NewGraphVisitor()
	if err := processor.Process(graph, names...); err != nil {
		return nil, err
	}

	results, err := graph.Results()
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.Path == "graph.json" {
			return r.Contents, nil
		}
	}
	return nil, fmt.Errorf("graph visitor produced no graph.json entry")
}
