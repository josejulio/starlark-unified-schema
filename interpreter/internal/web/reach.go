package web

import (
	"github.com/project-kessel/starlark-unified-schema/internal/analyze"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// CheckReachable is the authoritative graph.json -> reachability-verdict transform
// for the browser: given a compiled graph.json and a
// "TYPE.REPORTER#RELATION@TYPE.REPORTER" target, it returns the verdict as JSON —
// byte-identical to what `graph-analyze -reach -format json` produces on the CLI,
// since both call the same analyze.CheckReachable. The in-browser WASM compiler
// (cmd/graph-wasm) exposes this so the playground can verify structural
// reachability without re-implementing the analysis in JavaScript.
func CheckReachable(data []byte, target string) ([]byte, error) {
	doc, err := graphdoc.Parse(data)
	if err != nil {
		return nil, err
	}
	object, relation, subject, err := analyze.ParseReachTarget(target)
	if err != nil {
		return nil, err
	}
	verdict, err := analyze.CheckReachable(doc, object, relation, subject)
	if err != nil {
		return nil, err
	}
	rendered, err := analyze.FormatReachJSON(verdict)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}
