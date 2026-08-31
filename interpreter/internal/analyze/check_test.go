package analyze

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/project-kessel/starlark-unified-schema/internal/lang"
	"github.com/stretchr/testify/require"
)

// TestExplainCheckRealSchema is the golden guarantee against the committed schema:
// it compiles schema/ the same way the CLI and the WASM playground do, then
// explains workspace.features#enabled_services. That permission is
//
//	enabled_services = _paid_services AND _desired_services
//	_paid_services   = direct_billing_account→services OR parent→_paid_services
//	_desired_services = direct_service_preferences OR parent→_desired_services
//
// parent is an AtMostOne self-relation, so both sub-permissions walk the workspace
// tree: the check is depth-bound, O(D_workspace), with no fan-out.
func TestExplainCheckRealSchema(t *testing.T) {
	doc := compileRealSchema(t)

	root, err := ExplainCheck(doc, FacetRef{TypeName: "workspace", Reporter: "features"}, "enabled_services")
	require.NoError(t, err)

	// Headline cost: bounded by the hierarchy depth, recursive, no fan-out.
	require.Equal(t, "O(D_workspace)", root.Cost.BigO)
	require.True(t, root.Cost.Recursive)
	require.Equal(t, 0, root.Cost.FanoutSites)
	require.Equal(t, 1, root.Cost.DispatchDepth)

	// Root shape: the permission expands to an AND of its two sub-permissions.
	require.Equal(t, "permission", root.Kind)
	require.Equal(t, "enabled_services", root.Name)
	require.Equal(t, "op", root.Body.Kind)
	require.Equal(t, "and", root.Body.Op)

	// The single cost variable is the workspace hierarchy depth, sourced from the
	// parent relation.
	vars := collectVars(root)
	require.Len(t, vars, 1)
	require.Equal(t, "D_workspace", vars[0].Name)
	require.Equal(t, "workspace.parent", vars[0].Source)

	// The recursion is detected: parent→_paid_services re-enters _paid_services on
	// the features facet (found by cross-facet fallback: parent targets rbac).
	paid := root.Body.Children[0] // AND left operand
	require.Equal(t, "permission", paid.Kind)
	require.Equal(t, "_paid_services", paid.Name)
	arrow := paid.Body.Children[1] // OR right operand: parent→_paid_services
	require.Equal(t, "arrow", arrow.Kind)
	require.Equal(t, "parent", arrow.Name)
	require.Equal(t, "workspace", arrow.TargetType)
	require.Equal(t, "features", arrow.TargetReporter)
	require.Equal(t, "recursive", arrow.Children[0].Kind)

	// The text report renders headline + tree + variables.
	text := FormatCheckText(FacetRef{"workspace", "features"}, "enabled_services", root)
	require.Contains(t, text, "Cost:      O(D_workspace)")
	require.Contains(t, text, `permission "enabled_services" on features/workspace`)
	require.Contains(t, text, "↺ _paid_services on features/workspace (recursion)")
	require.Contains(t, text, "D_workspace")
}

// TestExplainCheckDirectRelation checks that a bare relation (not a permission) is
// a constant-time leaf.
func TestExplainCheckDirectRelation(t *testing.T) {
	doc := compileRealSchema(t)

	root, err := ExplainCheck(doc, FacetRef{TypeName: "workspace", Reporter: "features"}, "direct_service_preferences")
	require.NoError(t, err)
	require.Equal(t, "relation", root.Kind)
	require.Equal(t, "O(1)", root.Cost.BigO)
	require.False(t, root.Cost.Recursive)
}

// TestExplainCheckFanout exercises fan-out and the product of many-arrows, which
// the real schema does not contain: a doc reaches team members through a
// many-relation (editors), and manager-of-members through two nested many-arrows
// (deep).
func TestExplainCheckFanout(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "team", "typeName": "team", "reporters": {"r": {
				"permissions": [{"name": "all_managers", "body": {"kind": "subreference", "name": "members", "sub": "manager"}}]
			}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "editors", "body": {"kind": "subreference", "name": "teams", "sub": "members"}},
					{"name": "deep", "body": {"kind": "subreference", "name": "teams", "sub": "all_managers"}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "user", "target": "user", "name": "manager", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "u", "targetReporter": "u", "self": true},
			{"kind": "relation", "source": "team", "target": "user", "name": "members", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"},
			{"kind": "relation", "source": "doc", "target": "team", "name": "teams", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// Single many-arrow: doc.editors = teams→members.
	editors, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "editors")
	require.NoError(t, err)
	require.Equal(t, "O(N_teams)", editors.Cost.BigO)
	require.Equal(t, 1, editors.Cost.FanoutSites)
	require.False(t, editors.Cost.Recursive)

	// Nested many-arrows multiply: doc.deep = teams→(members→manager).
	deep, err := ExplainCheck(doc, FacetRef{"doc", "r"}, "deep")
	require.NoError(t, err)
	require.Equal(t, "O(N_members·N_teams)", deep.Cost.BigO)
	require.Equal(t, 2, deep.Cost.FanoutSites)
	require.Equal(t, 2, deep.Cost.DispatchDepth)
}

func TestExplainCheckErrors(t *testing.T) {
	doc := compileRealSchema(t)

	_, err := ExplainCheck(doc, FacetRef{TypeName: "nope"}, "view")
	require.ErrorContains(t, err, "unknown resource type")

	_, err = ExplainCheck(doc, FacetRef{TypeName: "workspace", Reporter: "nope"}, "view")
	require.ErrorContains(t, err, "no reporter")

	// workspace has both rbac and features facets — an unqualified object is ambiguous.
	_, err = ExplainCheck(doc, FacetRef{TypeName: "workspace"}, "enabled_services")
	require.ErrorContains(t, err, "multiple reporters")

	_, err = ExplainCheck(doc, FacetRef{TypeName: "workspace", Reporter: "features"}, "does_not_exist")
	require.ErrorContains(t, err, "neither a permission nor a relation")
}

// compileRealSchema builds the graph document from the committed schema tree using
// the same pipeline as the CLI and the WASM playground.
func compileRealSchema(t *testing.T) graphdoc.Document {
	t.Helper()
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

	data, err := lang.CompileGraph(files)
	require.NoError(t, err)
	doc, err := graphdoc.Parse(data)
	require.NoError(t, err)
	return doc
}
