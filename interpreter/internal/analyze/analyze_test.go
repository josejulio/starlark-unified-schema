package analyze

import (
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/stretchr/testify/require"
)

func TestIslands(t *testing.T) {
	// mainland: host -> workspace -> service (workspace also self-relates).
	// island cluster: foo <-> bar (linked to each other, not the mainland).
	// isolated: lonely (no relations); orphan (only inheritance, which does not count).
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "host", "typeName": "host", "reporters": {"hbi": {}}},
			{"id": "workspace", "typeName": "workspace", "reporters": {"rbac": {}}},
			{"id": "service", "typeName": "service", "reporters": {"features": {}}},
			{"id": "foo", "typeName": "foo", "reporters": {"a": {}}},
			{"id": "bar", "typeName": "bar", "reporters": {"a": {}}},
			{"id": "lonely", "typeName": "lonely", "reporters": {"a": {}}},
			{"id": "orphan", "typeName": "orphan", "reporters": {"a": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "host", "target": "workspace", "name": "workspace_id"},
			{"kind": "relation", "source": "workspace", "target": "service", "name": "svc"},
			{"kind": "relation", "source": "workspace", "target": "workspace", "name": "parent", "self": true},
			{"kind": "relation", "source": "foo", "target": "bar", "name": "buddy"},
			{"kind": "inherits", "source": "orphan", "target": "host"}
		]
	}`)

	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	r := Islands(doc)

	require.Equal(t, 7, r.NodeCount)
	require.Equal(t, 3, r.RelationEdge) // self-relation and inheritance excluded
	require.Equal(t, []string{"host", "service", "workspace"}, r.LargestComponent.Members)
	require.Equal(t, []Component{
		{Members: []string{"bar", "foo"}},
		{Members: []string{"lonely"}},
		{Members: []string{"orphan"}},
	}, r.Islands)
	require.Equal(t, []string{"lonely", "orphan"}, r.Isolated)
}

func TestIslandsNoIslands(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "a", "typeName": "a", "reporters": {"r": {}}},
			{"id": "b", "typeName": "b", "reporters": {"r": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "a", "target": "b", "name": "rel"}
		]
	}`)

	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	r := Islands(doc)
	require.Empty(t, r.Islands)
	require.Empty(t, r.Isolated)
	require.Equal(t, []string{"a", "b"}, r.LargestComponent.Members)
}
