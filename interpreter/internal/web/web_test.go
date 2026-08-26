package web

import (
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/stretchr/testify/require"
)

// fixture mirrors internal/render's Mermaid test so the two consumers can be
// compared, extended with facet members so the transform's data-field and
// permission passthrough is covered: host has a common data field and a reporter
// data field; workspace's features facet carries a permission rewrite tree and
// inherits its rbac facet.
var fixture = []byte(`{
	"version": "1",
	"nodes": [
		{"id": "host", "kind": "resource", "typeName": "host",
			"common": {"dataFields": [{"name": "name", "required": true, "type": {"kind": "text", "maxLength": 255}}]},
			"reporters": {"hbi": {"dataFields": [{"name": "satellite_id", "required": false, "type": {"kind": "nullable", "inner": {"kind": "uuid"}}}]}}},
		{"id": "service", "kind": "resource", "typeName": "service", "reporters": {"features": {}}},
		{"id": "workspace", "kind": "resource", "typeName": "workspace", "reporters": {"rbac": {},
			"features": {"permissions": [{"name": "view", "body": {"kind": "or", "left": {"kind": "reference", "name": "owner"}, "right": {"kind": "subreference", "name": "parent", "sub": "view"}}}]}}}
	],
	"edges": [
		{"kind": "relation", "source": "host", "target": "workspace", "name": "workspace_id", "cardinality": "ExactlyOne", "scope": "common", "targetReporter": "rbac"},
		{"kind": "relation", "source": "workspace", "target": "service", "name": "direct_service_preferences", "cardinality": "Many", "scope": "reporter", "sourceReporter": "features", "targetReporter": "features"},
		{"kind": "relation", "source": "workspace", "target": "workspace", "name": "parent", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "rbac", "targetReporter": "rbac", "self": true},
		{"kind": "inherits", "source": "workspace", "target": "workspace", "sourceReporter": "features", "targetReporter": "rbac"}
	]
}`)

func TestBuildElements(t *testing.T) {
	doc, err := graphdoc.Parse(fixture)
	require.NoError(t, err)

	got, err := marshalElements(BuildElements(doc))
	require.NoError(t, err)

	want := `[
  {
    "data": {
      "group": "type",
      "hasCommon": true,
      "id": "host",
      "kind": "resource",
      "label": "host",
      "reporters": [
        "hbi"
      ],
      "typeName": "host"
    },
    "classes": "resource"
  },
  {
    "data": {
      "dataFields": [
        {
          "name": "name",
          "required": true,
          "type": {
            "kind": "text",
            "maxLength": 255
          }
        }
      ],
      "group": "common",
      "id": "host__common",
      "label": "common",
      "parent": "host",
      "typeName": "host"
    },
    "classes": "common"
  },
  {
    "data": {
      "dataFields": [
        {
          "name": "satellite_id",
          "required": false,
          "type": {
            "kind": "nullable",
            "inner": {
              "kind": "uuid"
            }
          }
        }
      ],
      "group": "reporter",
      "id": "host__hbi",
      "label": "hbi",
      "parent": "host",
      "reporter": "hbi",
      "typeName": "host"
    },
    "classes": "reporter"
  },
  {
    "data": {
      "group": "type",
      "hasCommon": false,
      "id": "service",
      "kind": "resource",
      "label": "service",
      "reporters": [
        "features"
      ],
      "typeName": "service"
    },
    "classes": "resource"
  },
  {
    "data": {
      "group": "reporter",
      "id": "service__features",
      "label": "features",
      "parent": "service",
      "reporter": "features",
      "typeName": "service"
    },
    "classes": "reporter"
  },
  {
    "data": {
      "group": "type",
      "hasCommon": false,
      "id": "workspace",
      "kind": "resource",
      "label": "workspace",
      "reporters": [
        "features",
        "rbac"
      ],
      "typeName": "workspace"
    },
    "classes": "resource"
  },
  {
    "data": {
      "group": "reporter",
      "id": "workspace__features",
      "label": "features",
      "parent": "workspace",
      "permissions": [
        {
          "name": "view",
          "body": {
            "kind": "or",
            "left": {
              "kind": "reference",
              "name": "owner"
            },
            "right": {
              "kind": "subreference",
              "name": "parent",
              "sub": "view"
            }
          }
        }
      ],
      "reporter": "features",
      "typeName": "workspace"
    },
    "classes": "reporter"
  },
  {
    "data": {
      "group": "reporter",
      "id": "workspace__rbac",
      "label": "rbac",
      "parent": "workspace",
      "reporter": "rbac",
      "typeName": "workspace"
    },
    "classes": "reporter"
  },
  {
    "data": {
      "cardinality": "ExactlyOne",
      "id": "host__common->workspace__rbac:workspace_id",
      "kind": "relation",
      "label": "workspace_id",
      "name": "workspace_id",
      "scope": "common",
      "self": false,
      "source": "host__common",
      "target": "workspace__rbac",
      "targetReporter": "rbac"
    },
    "classes": "relation"
  },
  {
    "data": {
      "id": "host__common==>host__hbi",
      "kind": "shared",
      "source": "host__common",
      "target": "host__hbi"
    },
    "classes": "shared"
  },
  {
    "data": {
      "cardinality": "Many",
      "id": "workspace__features->service__features:direct_service_preferences",
      "kind": "relation",
      "label": "direct_service_preferences (*)",
      "name": "direct_service_preferences",
      "scope": "reporter",
      "self": false,
      "source": "workspace__features",
      "sourceReporter": "features",
      "target": "service__features",
      "targetReporter": "features"
    },
    "classes": "relation"
  },
  {
    "data": {
      "id": "workspace__features=>inherits:workspace__rbac",
      "kind": "inherits",
      "label": "extends",
      "source": "workspace__features",
      "target": "workspace__rbac"
    },
    "classes": "inherits"
  },
  {
    "data": {
      "cardinality": "AtMostOne",
      "id": "workspace__rbac->workspace__rbac:parent",
      "kind": "relation",
      "label": "parent (0..1)",
      "name": "parent",
      "scope": "reporter",
      "self": true,
      "source": "workspace__rbac",
      "sourceReporter": "rbac",
      "target": "workspace__rbac",
      "targetReporter": "rbac"
    },
    "classes": "relation self"
  }
]`

	require.Equal(t, want, string(got))
}

// TestElementsMatchesTransform asserts the exported Elements transform produces
// the same element JSON as BuildElements + marshalElements — one tested transform,
// used directly by the in-browser WASM compiler.
func TestElementsMatchesTransform(t *testing.T) {
	doc, err := graphdoc.Parse(fixture)
	require.NoError(t, err)
	fromTransform, err := marshalElements(BuildElements(doc))
	require.NoError(t, err)

	fromElements, err := Elements(fixture)
	require.NoError(t, err)
	require.Equal(t, string(fromTransform), string(fromElements))
}
