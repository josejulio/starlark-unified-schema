package web

import (
	"github.com/project-kessel/starlark-unified-schema/internal/analyze"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// Reach is the authoritative graph.json -> reachability-paths transform for the
// browser: given a compiled graph.json and a "TYPE[.REPORTER]#RELATION[@SUBJECTTYPE]"
// target, it returns the ReachReport as JSON — byte-identical to what
// `graph-analyze -paths -format json` produces on the CLI, since both call the
// same analyze.Reach. The in-browser WASM compiler (cmd/graph-wasm) exposes this
// so the playground can enumerate grant paths and highlight them on the graph.
func Reach(data []byte, target string) ([]byte, error) {
	doc, err := graphdoc.Parse(data)
	if err != nil {
		return nil, err
	}
	object, relation, subjectType, err := analyze.ParseReachTarget(target)
	if err != nil {
		return nil, err
	}
	report, err := analyze.Reach(doc, object, relation, subjectType)
	if err != nil {
		return nil, err
	}
	rendered, err := analyze.FormatReachJSON(report)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}
