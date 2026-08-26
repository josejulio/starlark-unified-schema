package lang

import (
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processAndBuildGraph runs the processor with a GraphVisitor and returns the
// contents of the single graph.json output entry.
func processAndBuildGraph(t *testing.T, processor *Processor) []byte {
	t.Helper()

	graph := output.NewGraphVisitor()
	require.NoError(t, processor.Process(graph))

	results, err := graph.Results()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "graph.json", results[0].Path)

	return results[0].Contents
}

func TestGraphVisitorRelationsAndPermissions(t *testing.T) {
	reader := newInMemorySourceFileReader("schema")
	processor := setupProcessorWithKessel(t, reader)

	reader.AddFile("test/graph.star", []byte(`
load("kessel.star", "resource", "uuid", "self", "at_most_one", "wildcard")

container = resource("test", id_type=uuid(), fields={
	"flag": wildcard(self())
})

this_resource = resource("test", id_type=uuid(), fields={
	"parent": at_most_one(self()),
	"container": at_most_one(container)
}, permissions={
	"view": lambda r: r.container.flag.union(r.parent.view)
})
`))

	graph := processAndBuildGraph(t, processor)

	assert.JSONEq(t, `{
		"version": "1",
		"nodes": [
			{
				"id": "container",
				"kind": "resource",
				"typeName": "container",
				"reporters": { "test": {} }
			},
			{
				"id": "this_resource",
				"kind": "resource",
				"typeName": "this_resource",
				"reporters": {
					"test": {
						"permissions": [
							{
								"name": "view",
								"body": {
									"kind": "or",
									"left": {"kind": "subreference", "name": "container", "sub": "flag"},
									"right": {"kind": "subreference", "name": "parent", "sub": "view"}
								}
							}
						]
					}
				}
			}
		],
		"edges": [
			{
				"id": "container#reporter:test.flag",
				"kind": "relation",
				"source": "container",
				"target": "container",
				"name": "flag",
				"cardinality": "All",
				"scope": "reporter",
				"sourceReporter": "test",
				"targetReporter": "test",
				"self": true
			},
			{
				"id": "this_resource#reporter:test.container",
				"kind": "relation",
				"source": "this_resource",
				"target": "container",
				"name": "container",
				"cardinality": "AtMostOne",
				"scope": "reporter",
				"sourceReporter": "test",
				"targetReporter": "test"
			},
			{
				"id": "this_resource#reporter:test.parent",
				"kind": "relation",
				"source": "this_resource",
				"target": "this_resource",
				"name": "parent",
				"cardinality": "AtMostOne",
				"scope": "reporter",
				"sourceReporter": "test",
				"targetReporter": "test",
				"self": true
			}
		]
	}`, string(graph))
}

func TestGraphVisitorCommonRelationAndInheritance(t *testing.T) {
	reader := newInMemorySourceFileReader("schema")
	processor := setupProcessorWithKessel(t, reader)

	reader.AddFile("test/graph_inherit.star", []byte(`
load("kessel.star", "resource", "uuid", "at_most_one", "self", "wildcard")

principal = resource("rbac", id_type=uuid())

common = {
	"owner": at_most_one(principal)
}

folder = resource("test", id_type=uuid(), common=common, fields={
	"parent": at_most_one(self())
})

special_folder = resource("test", extends=folder, fields={
	"direct_flag": wildcard(self())
})
`))

	graph := processAndBuildGraph(t, processor)

	assert.JSONEq(t, `{
		"version": "1",
		"nodes": [
			{
				"id": "folder",
				"kind": "resource",
				"typeName": "folder",
				"reporters": { "test": {} }
			},
			{
				"id": "principal",
				"kind": "resource",
				"typeName": "principal",
				"reporters": { "rbac": {} }
			},
			{
				"id": "special_folder",
				"kind": "resource",
				"typeName": "special_folder",
				"reporters": {
					"test": { "extends": {"typeName": "folder", "reporter": "test"} }
				}
			}
		],
		"edges": [
			{
				"id": "folder#common.owner",
				"kind": "relation",
				"source": "folder",
				"target": "principal",
				"name": "owner",
				"cardinality": "AtMostOne",
				"scope": "common",
				"targetReporter": "rbac"
			},
			{
				"id": "folder#reporter:test.parent",
				"kind": "relation",
				"source": "folder",
				"target": "folder",
				"name": "parent",
				"cardinality": "AtMostOne",
				"scope": "reporter",
				"sourceReporter": "test",
				"targetReporter": "test",
				"self": true
			},
			{
				"id": "special_folder#reporter:test.direct_flag",
				"kind": "relation",
				"source": "special_folder",
				"target": "special_folder",
				"name": "direct_flag",
				"cardinality": "All",
				"scope": "reporter",
				"sourceReporter": "test",
				"targetReporter": "test",
				"self": true
			},
			{
				"id": "special_folder#reporter:test=>inherits",
				"kind": "inherits",
				"source": "special_folder",
				"target": "folder",
				"sourceReporter": "test",
				"targetReporter": "test"
			}
		]
	}`, string(graph))
}
