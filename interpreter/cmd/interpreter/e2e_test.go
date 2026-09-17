package main

import (
	"testing"

	"github.com/project-kessel/ksl-schema-language/pkg/intermediate"
	"github.com/project-kessel/starlark-unified-schema/internal/lang"
	"github.com/project-kessel/starlark-unified-schema/internal/output"
	"github.com/project-kessel/starlark-unified-schema/internal/output/ksil"
	"github.com/project-kessel/starlark-unified-schema/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestE2E(t *testing.T) {
	processor, reader := setupForTest(t)

	util.AddFile(t, reader, "principal.star", `
load("kessel.star", "resource", "uuid")
principal = resource("test", id_type=uuid())
`)

	util.AddFile(t, reader, "container.star", `
load("kessel.star", "resource", "at_most_one", "many", "self", "uuid")
load("principal.star", "principal")
container = resource("test", id_type=uuid(), fields={
	"parent": at_most_one(self()),
	"direct_allowed_users": many(principal)
}, permissions={
	"allowed_users": lambda c: c.direct_allowed_users.union(c.parent.allowed_users)
})`)

	util.AddFile(t, reader, "special_container.star", `
load("kessel.star", "resource", "wildcard")
load("principal.star", "principal")
load("container.star", test_container="container")
container = resource("special", extends=test_container, fields={
	"direct_flag": wildcard(principal)
}, permissions={
	"flag": lambda r: r.direct_flag.union(r.parent.flag)
})
`)

	util.AddFile(t, reader, "common.star", `
load("kessel.star", "one")
load("container.star", "container")
res = {
	"container": one(container)
}
`)

	util.AddFile(t, reader, "res.star", `
load("kessel.star", "resource", "uuid", "field", "nullable", "union", "text")
load("common.star", common="res")
res = resource("test", common=common, id_type=uuid(), fields={
	"system_1_id": field(type=nullable(union(uuid(), text(regex="^\\d{10}$")))),
	"system_2_id": field(type=nullable(uuid())),
	"hostname": field(type=nullable(text(maxLength=255)))
}, permissions={
	"operation": lambda r: r.container.allowed_users
})
`)

	testNS := "test"

	verifyOutputs(t, processor, map[string]*intermediate.Namespace{
		"test.json": {
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name:      "principal",
					Relations: []*intermediate.Relation{},
				},
				{
					Name: "container",
					Relations: []*intermediate.Relation{
						{
							Name: "parent",
							Body: &intermediate.RelationBody{
								Kind: "self",
								Types: []*intermediate.TypeReference{{
									Namespace: "test",
									Name:      "container",
								}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "direct_allowed_users",
							Body: &intermediate.RelationBody{
								Kind: "self",
								Types: []*intermediate.TypeReference{{
									Namespace: "test",
									Name:      "principal",
								}},
								Cardinality: "Any",
							},
						},
						{
							Name: "allowed_users",
							Body: &intermediate.RelationBody{
								Kind: "union",
								Left: &intermediate.RelationBody{
									Kind:     "reference",
									Relation: "direct_allowed_users",
								},
								Right: &intermediate.RelationBody{
									Kind:        "nested_reference",
									Relation:    "parent",
									SubRelation: "allowed_users",
								},
							},
						},
					},
				},
				{
					Name: "res",
					Relations: []*intermediate.Relation{
						{
							Name: "container",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "container"}},
								Cardinality: "ExactlyOne",
							},
						}, //Inherited from common representation
						{
							Name: "operation",
							Body: &intermediate.RelationBody{
								Kind:        "nested_reference",
								Relation:    "container",
								SubRelation: "allowed_users",
							},
						},
					},
				},
			},
		},
		"special.json": {
			Name: "special",
			ExtensionDefinitions: []*intermediate.ExtensionDefinition{
				{
					Name:       "container",
					Visibility: "internal",
					Params:     []string{},
					Types: []*intermediate.DynamicType{
						{
							Name:       &intermediate.DynamicName{Kind: "literal", Value: "container"},
							Namespace:  &testNS,
							Visibility: "public",
							Relations: []*intermediate.DynamicRelation{
								{
									Name: &intermediate.DynamicName{Kind: "literal", Value: "special_container_direct_flag"},
									Body: intermediate.DynamicRelationBody{
										Kind:        "self",
										Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "principal", All: true}},
										Cardinality: "Any",
									},
								},
								{
									Name: &intermediate.DynamicName{Kind: "literal", Value: "special_container_flag"},
									Body: intermediate.DynamicRelationBody{
										Kind: "union",
										Left: &intermediate.DynamicRelationBody{
											Kind:     "reference",
											Relation: &intermediate.DynamicName{Kind: "literal", Value: "special_container_direct_flag"},
										},
										Right: &intermediate.DynamicRelationBody{
											Kind:        "nested_reference",
											Relation:    &intermediate.DynamicName{Kind: "literal", Value: "parent"},
											SubRelation: &intermediate.DynamicName{Kind: "literal", Value: "special_container_flag"},
										},
									},
								},
							},
						},
					},
				},
			},
			ExtensionReferences: []*intermediate.ExtensionReference{{
				Namespace: "special",
				Name:      "container",
				Params:    map[string]string{},
			}},
		},
	}, map[string]util.JsonSchemaTestCase{
		"principal/common_representation.json": {
			Valid: []string{
				`{}`,
			},
		},
		"principal/reporters/test/principal.json": {
			Valid: []string{
				`{}`,
			},
		},
		"container/common_representation.json": {
			Valid: []string{
				`{}`,
			},
		},
		"container/reporters/test/container.json": {
			Valid: []string{
				`{
					"parent": "f6e29208-ab8d-11f1-b92a-6ae246604903", 
					"direct_allowed_users": ["122eacf4-ab8e-11f1-a632-6ae246604903", "19388cae-ab8e-11f1-8936-6ae246604903"]
				}`,
			},
		},
		"container/reporters/special/container.json": {
			Valid: []string{
				`{
					"direct_flag": "test/principal:*"
				}`,
			},
		},
		"res/common_representation.json": {
			Valid: []string{
				`{
					"container": "f9c7da5e-ab8e-11f1-bffd-6ae246604903"
				}`,
			},
		},
		"res/reporters/test/res.json": {
			Valid: []string{
				`{
					"system_1_id": "1234567890",
					"system_2_id": "85f476d6-ab8f-11f1-9e40-6ae246604903",
					"hostname": "the-host.the-domain.tld"
				}`,
				`{
					"system_1_id": "b06faed0-ab8f-11f1-b830-6ae246604903",
					"system_2_id": null,
					"hostname": "the-host.the-domain.tld"
				}`,
			},
		},
	})
}

func setupForTest(t *testing.T) (*lang.Processor, *lang.InmemorySourceFileReader) {
	t.Helper()
	reader := lang.NewInMemorySourceFileReader("schema", "../../../schema")
	loader := lang.NewLoaderForReader("schema", reader)
	processor := lang.NewProcessor(loader)

	err := reader.AddRealSchemaFile("kessel.star")
	if err != nil {
		t.Log("error in test setup: ", err)
		t.FailNow()
	}

	return processor, reader
}

func verifyOutputs(t *testing.T, processor *lang.Processor, ksilModules map[string]*intermediate.Namespace, jsonSchemaExamples map[string]util.JsonSchemaTestCase) {
	t.Helper()

	jsonSchema := output.NewJSONSchemaVisitor()
	err := processor.Process(jsonSchema)
	if assert.NoError(t, err, "error processing jsonschema visitor") {
		util.VerifyJSONSchemaResults(t, jsonSchema, jsonSchemaExamples)
	}

	ksilVisitor := ksil.NewKSILVisitor()
	err = processor.Process(ksilVisitor)
	if assert.NoError(t, err, "error processing ksil visitor") {
		util.VerifyKSILResults(t, ksilVisitor, ksilModules)
	}
}
