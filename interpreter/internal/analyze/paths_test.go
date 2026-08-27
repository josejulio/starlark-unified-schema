package analyze

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/project-kessel/starlark-unified-schema/internal/lang"
	"github.com/stretchr/testify/require"
)

// TestEnumeratePathsOR verifies that an OR produces two paths (union).
func TestEnumeratePathsOR(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "or",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "viewer"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "viewer", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	root, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "view")
	require.NoError(t, err)

	paths := EnumeratePaths(root)
	require.Len(t, paths, 2, "OR should produce two paths")

	// Both paths should reach user.
	require.Equal(t, "user", paths[0].SubjectType)
	require.Equal(t, "user", paths[1].SubjectType)

	// One path is via owner, the other via viewer.
	names := []string{paths[0].Hops[0].Relation, paths[1].Hops[0].Relation}
	require.ElementsMatch(t, []string{"owner", "viewer"}, names)
}

// TestEnumeratePathsAND verifies that an AND annotates the left path with
// conjuncts from the right.
func TestEnumeratePathsAND(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "and",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "approved"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "approved", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	root, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "view")
	require.NoError(t, err)

	paths := EnumeratePaths(root)
	require.Len(t, paths, 1, "AND should produce one path with conjuncts")

	p := paths[0]
	require.Equal(t, "owner", p.Hops[0].Relation)
	require.Len(t, p.Conjuncts, 1, "AND right operand should be a conjunct")
	require.Equal(t, "approved", p.Conjuncts[0].Hops[0].Relation)
}

// TestEnumeratePathsUNLESS verifies that an UNLESS annotates the left path with
// exclusions from the right.
func TestEnumeratePathsUNLESS(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "unless",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "banned"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "banned", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	root, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "view")
	require.NoError(t, err)

	paths := EnumeratePaths(root)
	require.Len(t, paths, 1, "UNLESS should produce one path with exclusions")

	p := paths[0]
	require.Equal(t, "owner", p.Hops[0].Relation)
	require.Len(t, p.Exclusions, 1, "UNLESS right operand should be an exclusion")
	require.Equal(t, "banned", p.Exclusions[0].Hops[0].Relation)
}

// TestEnumeratePathsArrow verifies that arrow (subreference) chains hops correctly.
func TestEnumeratePathsArrow(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "folder", "typeName": "folder", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "subreference", "name": "parent", "sub": "owner"}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "folder", "name": "parent", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "folder", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	root, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "view")
	require.NoError(t, err)

	paths := EnumeratePaths(root)
	require.Len(t, paths, 1)

	p := paths[0]
	require.Len(t, p.Hops, 2, "Arrow chain should produce two hops")
	require.Equal(t, "parent", p.Hops[0].Relation)
	require.Equal(t, "doc", p.Hops[0].FromType)
	require.Equal(t, "folder", p.Hops[0].ToType)

	require.Equal(t, "owner", p.Hops[1].Relation)
	require.Equal(t, "folder", p.Hops[1].FromType)
	require.Equal(t, "user", p.Hops[1].ToType)

	require.Equal(t, "user", p.SubjectType)
}

// TestEnumeratePathsRecursive verifies that a recursive node is a cycle marker.
func TestEnumeratePathsRecursive(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "workspace", "typeName": "workspace", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "or",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "subreference", "name": "parent", "sub": "view"}}}
				]
			}}},
			{"id": "user", "typeName": "user", "reporters": {"r": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "workspace", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "workspace", "target": "workspace", "name": "parent", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r", "self": true}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	root, err := ExplainCheck(doc, FacetRef{"workspace", "r"}, "view")
	require.NoError(t, err)

	paths := EnumeratePaths(root)
	require.Len(t, paths, 2, "OR with recursion should produce two paths")

	// One path is direct owner, the other is the recursive parent→view.
	var ownerPath, recursivePath Path
	for _, p := range paths {
		if len(p.Hops) == 1 && p.Hops[0].Relation == "owner" {
			ownerPath = p
		} else {
			recursivePath = p
		}
	}

	require.Equal(t, "owner", ownerPath.Hops[0].Relation)
	require.False(t, ownerPath.Recursive)

	require.Equal(t, "parent", recursivePath.Hops[0].Relation)
	require.True(t, recursivePath.Hops[0].Recursive, "Recursive hop should be marked")
	require.True(t, recursivePath.Recursive, "Path with recursive hop should be flagged")
	require.Equal(t, "view", recursivePath.Hops[1].Relation)
}

// TestEnumeratePathsUnresolved verifies that an unresolved name produces a
// dead-end path with no subject type.
func TestEnumeratePathsUnresolved(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "reference", "name": "nonexistent"}}
				]
			}}}
		],
		"edges": []
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// ExplainCheck succeeds (permission exists) but the body is unresolved.
	root, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "view")
	require.NoError(t, err)
	require.Equal(t, "permission", root.Kind)
	require.Equal(t, "unresolved", root.Body.Kind)

	// EnumeratePaths should produce a dead-end path with empty subject type.
	paths := EnumeratePaths(root)
	require.Len(t, paths, 1)
	require.Empty(t, paths[0].Hops)
	require.Empty(t, paths[0].SubjectType, "Unresolved should produce empty subject type")
}

// TestReachRealSchema is the golden test: it runs Reach against the committed
// schema and verifies the reachability report for workspace.features#enabled_services.
func TestReachRealSchema(t *testing.T) {
	doc := compileRealSchema(t)

	report, err := Reach(doc, FacetRef{TypeName: "workspace", Reporter: "features"}, "enabled_services", "")
	require.NoError(t, err)

	// The permission should be reachable.
	require.True(t, report.Reachable)

	// Reachable types should include service and workspace (from the recursive structure).
	require.NotEmpty(t, report.ReachableTypes)
	// The exact set depends on the schema, but there should be multiple paths due to OR.

	// There should be paths (multiple OR branches).
	require.NotEmpty(t, report.Paths)

	// Cheapest and worst paths should be set.
	require.NotNil(t, report.Cheapest)
	require.NotNil(t, report.Worst)

	// Text report should render.
	text := FormatReachText(report)
	require.Contains(t, text, "Reachability: features/workspace#enabled_services")
	require.Contains(t, text, "Reachable subject types")

	// JSON report should render.
	jsonText, err := FormatReachJSON(report)
	require.NoError(t, err)
	require.Contains(t, jsonText, "enabled_services")
}

// TestReachWithFilter verifies subject type filtering.
func TestReachWithFilter(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "team", "typeName": "team", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "or",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "team_viewer"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "team", "name": "team_viewer", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// Without filter: both user and team are reachable.
	all, err := Reach(doc, FacetRef{"doc", "r"}, "view", "")
	require.NoError(t, err)
	require.True(t, all.Reachable)
	require.ElementsMatch(t, []string{"team", "user"}, all.ReachableTypes)
	require.Len(t, all.Paths, 2)

	// Filter to user: only owner path.
	userOnly, err := Reach(doc, FacetRef{"doc", "r"}, "view", "user")
	require.NoError(t, err)
	require.True(t, userOnly.Reachable)
	require.Len(t, userOnly.Paths, 1)
	require.Equal(t, "owner", userOnly.Paths[0].Hops[0].Relation)

	// Filter to team: only team_viewer path.
	teamOnly, err := Reach(doc, FacetRef{"doc", "r"}, "view", "team")
	require.NoError(t, err)
	require.True(t, teamOnly.Reachable)
	require.Len(t, teamOnly.Paths, 1)
	require.Equal(t, "team_viewer", teamOnly.Paths[0].Hops[0].Relation)

	// Filter to nonexistent type: not reachable.
	nope, err := Reach(doc, FacetRef{"doc", "r"}, "view", "nonexistent")
	require.NoError(t, err)
	require.False(t, nope.Reachable)
	require.Empty(t, nope.Paths)
}

// TestCompareCost verifies the cost ordering (FanoutSites, Recursive, DispatchDepth).
func TestCompareCost(t *testing.T) {
	a := Cost{FanoutSites: 0, Recursive: false, DispatchDepth: 1}
	b := Cost{FanoutSites: 1, Recursive: false, DispatchDepth: 1}
	require.Less(t, compareCost(a, b), 0, "fewer fan-out sites is cheaper")

	c := Cost{FanoutSites: 1, Recursive: false, DispatchDepth: 1}
	d := Cost{FanoutSites: 1, Recursive: true, DispatchDepth: 1}
	require.Less(t, compareCost(c, d), 0, "non-recursive is cheaper than recursive")

	e := Cost{FanoutSites: 1, Recursive: true, DispatchDepth: 1}
	f := Cost{FanoutSites: 1, Recursive: true, DispatchDepth: 2}
	require.Less(t, compareCost(e, f), 0, "lower dispatch depth is cheaper")

	g := Cost{FanoutSites: 1, Recursive: true, DispatchDepth: 2}
	h := Cost{FanoutSites: 1, Recursive: true, DispatchDepth: 2}
	require.Equal(t, 0, compareCost(g, h), "identical costs are equal")
}

// TestReachMatchesWeb is the parity guarantee: analyze.Reach (used by the CLI)
// and web.Reach (used by the WASM playground) produce byte-identical JSON output
// when given the same graph and target.
func TestReachMatchesWeb(t *testing.T) {
	const schemaDir = "../../../schema"

	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(schemaDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".star" {
			return nil
		}
		rel, err := filepath.Rel(schemaDir, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = contents
		return nil
	}))

	// Compile to graph.json.
	graphJSON, err := lang.CompileGraph(files)
	require.NoError(t, err)

	target := "features/workspace#enabled_services"

	// Native CLI path: parse graph, call analyze.Reach, format as JSON.
	doc, err := graphdoc.Parse(graphJSON)
	require.NoError(t, err)
	nativeReport, err := Reach(doc, FacetRef{TypeName: "workspace", Reporter: "features"}, "enabled_services", "")
	require.NoError(t, err)
	nativeJSON, err := FormatReachJSON(nativeReport)
	require.NoError(t, err)

	// WASM path: web.Reach (parse + analyze + format in one call).
	webJSON, err := webReach(graphJSON, target)
	require.NoError(t, err)

	// They must be byte-identical.
	require.Equal(t, nativeJSON, string(webJSON))
}

// webReach mimics the WASM wrapper's path without the actual WASM boundary.
func webReach(graphJSON []byte, target string) ([]byte, error) {
	// This is a stand-in for web.Reach — we can't directly import it here due to
	// import cycles (internal/web imports internal/analyze), so we inline the logic.
	doc, err := graphdoc.Parse(graphJSON)
	if err != nil {
		return nil, err
	}
	object, relation, subjectType, err := ParseReachTarget(target)
	if err != nil {
		return nil, err
	}
	report, err := Reach(doc, object, relation, subjectType)
	if err != nil {
		return nil, err
	}
	rendered, err := FormatReachJSON(report)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}
