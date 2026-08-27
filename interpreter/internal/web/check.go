package web

import (
	"github.com/project-kessel/starlark-unified-schema/internal/analyze"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// ExplainCheck is the authoritative graph.json -> check-cost transform for the
// browser: given a compiled graph.json and a "TYPE[.REPORTER]#RELATION" target,
// it returns the annotated proof tree as JSON — byte-identical to what
// `graph-analyze -check -format json` produces on the CLI, since both call the
// same analyze.ExplainCheck. The in-browser WASM compiler (cmd/graph-wasm)
// exposes this so the playground's inspector can annotate permissions with their
// read cost without re-implementing the analysis in JavaScript.
func ExplainCheck(data []byte, target string) ([]byte, error) {
	doc, err := graphdoc.Parse(data)
	if err != nil {
		return nil, err
	}
	object, relation, err := analyze.ParseCheckTarget(target)
	if err != nil {
		return nil, err
	}
	root, err := analyze.ExplainCheck(doc, object, relation)
	if err != nil {
		return nil, err
	}
	rendered, err := analyze.FormatCheckJSON(root)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}
