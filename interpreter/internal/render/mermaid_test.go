package render

import (
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/stretchr/testify/require"
)

func TestMermaidStructuralDiagram(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "host", "typeName": "host", "reporters": {"hbi": {}}},
			{"id": "service", "typeName": "service", "reporters": {"features": {}}},
			{"id": "workspace", "typeName": "workspace", "reporters": {"rbac": {}, "features": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "host", "target": "workspace", "name": "workspace_id", "cardinality": "ExactlyOne", "scope": "common", "targetReporter": "rbac"},
			{"kind": "relation", "source": "workspace", "target": "service", "name": "direct_service_preferences", "cardinality": "Many", "scope": "reporter", "sourceReporter": "features", "targetReporter": "features"},
			{"kind": "relation", "source": "workspace", "target": "workspace", "name": "parent", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "rbac", "targetReporter": "rbac", "self": true},
			{"kind": "inherits", "source": "workspace", "target": "workspace", "sourceReporter": "features", "targetReporter": "rbac"}
		]
	}`)

	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	got := Mermaid(doc, "", true)

	want := `flowchart LR
  subgraph host
    host__common["common"]
    host__hbi["hbi"]
  end
  subgraph service
    service__features["features"]
  end
  subgraph workspace
    workspace__features["features"]
    workspace__rbac["rbac"]
  end

  %% shared common representation
  host__common -.-> host__hbi

  %% relations
  host__common -->|"workspace_id"| workspace__rbac
  workspace__features -->|"direct_service_preferences (*)"| service__features
  workspace__rbac -->|"parent (0..1)"| workspace__rbac

  %% inheritance
  workspace__features -.->|"extends"| workspace__rbac

  linkStyle 0 stroke:#8a5cf6,stroke-width:2px;

  subgraph Legend
    direction LR
    legend0["(0..1) = at most one"]
    legend1["(*) = many"]
  end
`

	require.Equal(t, want, got)
}
