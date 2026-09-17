package ksil

import (
	"strings"
	"testing"

	"github.com/project-kessel/ksl-schema-language/pkg/intermediate"
	"github.com/project-kessel/starlark-unified-schema/internal/output"
	"github.com/stretchr/testify/assert"
)

func TestKSILVisitorRelation(t *testing.T) {
	v := NewKSILVisitor()

	v.BeginType("with_relation")
	r := v.VisitRelation("relation", "test", "with_relation", "AtMostOne", v.VisitUUIDDataType())
	assert.NoError(t, v.VisitResource("with_relation", "test", &output.Members{}, &output.Members{
		RelationFields: []any{r},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name: "with_relation",
					Relations: []*intermediate.Relation{{
						Name: "relation",
						Body: &intermediate.RelationBody{
							Kind:        "self",
							Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "with_relation"}},
							Cardinality: "AtMostOne",
						},
					}},
				},
			},
		},
	})
}

func TestKSILVisitorWildcardRelation(t *testing.T) {
	v := NewKSILVisitor()

	r := v.VisitRelation("all_services", "features", "service", "All", v.VisitUUIDDataType())

	assert.NoError(t, v.VisitResource("workspace", "features", &output.Members{}, &output.Members{
		RelationFields: []any{r},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"features.json": {
			Name: "features",
			Types: []*intermediate.Type{
				{
					Name: "workspace",
					Relations: []*intermediate.Relation{
						{
							Name: "all_services",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "features", Name: "service", All: true}},
								Cardinality: "Any",
							},
						},
					},
				},
			},
		},
	})
}

func TestKSILVisitorRelationWithPermission(t *testing.T) {
	v := NewKSILVisitor()

	v.BeginType("with_relation")
	r := v.VisitRelation("relation", "test", "with_relation", "AtMostOne", v.VisitUUIDDataType())
	p := v.VisitPermission("permission", v.VisitReferenceExpression("relation"))
	assert.NoError(t, v.VisitResource("with_relation", "test", &output.Members{}, &output.Members{
		RelationFields: []any{r},
		Permissions:    []any{p},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name: "with_relation",
					Relations: []*intermediate.Relation{
						{
							Name: "relation",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "with_relation"}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "permission",
							Body: &intermediate.RelationBody{
								Kind:     "reference",
								Relation: "relation",
							},
						},
					},
				},
			},
		},
	})
}

func TestKSILVisitorWithLogic(t *testing.T) {
	v := NewKSILVisitor()

	v.BeginType("with_logic")
	a := v.VisitRelation("a", "test", "with_logic", "AtMostOne", v.VisitUUIDDataType())
	b := v.VisitRelation("b", "test", "with_logic", "AtMostOne", v.VisitUUIDDataType())
	c := v.VisitRelation("c", "test", "with_logic", "AtMostOne", v.VisitUUIDDataType())
	p := v.VisitPermission("permission", v.VisitAnd(
		v.VisitReferenceExpression("a"),
		v.VisitOr(
			v.VisitReferenceExpression("b"),
			v.VisitUnless(
				v.VisitReferenceExpression("a"),
				v.VisitReferenceExpression("c"),
			),
		),
	)) // a.intersect(b.union(a.except(c)))
	assert.NoError(t, v.VisitResource("with_logic", "test", &output.Members{}, &output.Members{
		RelationFields: []any{a, b, c},
		Permissions:    []any{p},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name: "with_logic",
					Relations: []*intermediate.Relation{
						{
							Name: "a",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "with_logic"}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "b",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "with_logic"}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "c",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "with_logic"}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "permission",
							Body: &intermediate.RelationBody{
								Kind: "intersect",
								Left: &intermediate.RelationBody{
									Kind:     "reference",
									Relation: "a",
								},
								Right: &intermediate.RelationBody{
									Kind: "union",
									Left: &intermediate.RelationBody{
										Kind:     "reference",
										Relation: "b",
									},
									Right: &intermediate.RelationBody{
										Kind: "except",
										Left: &intermediate.RelationBody{
											Kind:     "reference",
											Relation: "a",
										},
										Right: &intermediate.RelationBody{
											Kind:     "reference",
											Relation: "c",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestKSILVisitorCrossTypeSubRelation(t *testing.T) {
	v := NewKSILVisitor()

	v.BeginType("some")
	o := v.VisitRelation("other", "test", "other", "ExactlyOne", v.VisitUUIDDataType())
	p := v.VisitPermission("permission", v.VisitSubReferenceExpression("other", "sub"))
	assert.NoError(t, v.VisitResource("some", "test", &output.Members{}, &output.Members{
		RelationFields: []any{o},
		Permissions:    []any{p},
	}, nil))

	v.BeginType("other")
	s := v.VisitRelation("sub", "test", "other", "AtMostOne", v.VisitUUIDDataType())
	assert.NoError(t, v.VisitResource("other", "test", &output.Members{}, &output.Members{
		RelationFields: []any{s},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name: "some",
					Relations: []*intermediate.Relation{
						{
							Name: "other",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "other"}},
								Cardinality: "ExactlyOne",
							},
						},
						{
							Name: "permission",
							Body: &intermediate.RelationBody{
								Kind:        "nested_reference",
								Relation:    "other",
								SubRelation: "sub",
							},
						},
					},
				},
				{
					Name: "other",
					Relations: []*intermediate.Relation{
						{
							Name: "sub",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "other"}},
								Cardinality: "AtMostOne",
							},
						},
					},
				},
			},
		},
	})
}

func TestKSILVisitorCrossNamespaceRelation(t *testing.T) {
	v := NewKSILVisitor()

	v.BeginType("some")
	o := v.VisitRelation("other", "test_other", "other", "ExactlyOne", v.VisitUUIDDataType())
	assert.NoError(t, v.VisitResource("some", "test", &output.Members{}, &output.Members{
		RelationFields: []any{o},
	}, nil))

	v.BeginType("other")
	assert.NoError(t, v.VisitResource("other", "test_other", &output.Members{}, &output.Members{}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: "test",
			Types: []*intermediate.Type{
				{
					Name: "some",
					Relations: []*intermediate.Relation{
						{
							Name: "other",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test_other", Name: "other"}},
								Cardinality: "ExactlyOne",
							},
						},
					},
				},
			},
		},
		"test_other.json": &intermediate.Namespace{
			Name: "test_other",
			Types: []*intermediate.Type{
				{
					Name:      "other",
					Relations: []*intermediate.Relation{},
				},
			},
		},
	})
}

func TestKSILVisitorSubclassType(t *testing.T) {
	v := NewKSILVisitor()

	ns := "test"

	v.BeginType("super")
	p := v.VisitRelation("parent", ns, "super", "AtMostOne", v.VisitUUIDDataType())
	assert.NoError(t, v.VisitResource("super", ns, &output.Members{}, &output.Members{
		RelationFields: []any{p},
	}, nil))

	v.BeginType("sub")
	d := v.VisitRelation("direct_enabled", ns, "super", "AtMostOne", v.VisitUUIDDataType())
	e := v.VisitPermission("enabled", v.VisitOr(
		v.VisitReferenceExpression("direct_enabled"),
		v.VisitSubReferenceExpression("parent", "enabled"),
	))
	assert.NoError(t, v.VisitResource("sub", ns, &output.Members{}, &output.Members{
		RelationFields: []any{d},
		Permissions:    []any{e},
	}, &output.ResourceTypeReference{Name: "super", Reporter: ns}))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"test.json": &intermediate.Namespace{
			Name: ns,
			Types: []*intermediate.Type{
				{
					Name: "super",
					Relations: []*intermediate.Relation{
						{
							Name: "parent",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "super"}},
								Cardinality: "AtMostOne",
							},
						},
					},
				},
			},
			ExtensionDefinitions: []*intermediate.ExtensionDefinition{
				{
					Name:       "sub",
					Visibility: "internal",
					Params:     []string{},
					Types: []*intermediate.DynamicType{
						{
							Name:       &intermediate.DynamicName{Kind: "literal", Value: "super"},
							Namespace:  &ns,
							Visibility: "public",
							Relations: []*intermediate.DynamicRelation{
								{
									Name: &intermediate.DynamicName{
										Kind:  "literal",
										Value: "test_sub_direct_enabled",
									},
									Body: intermediate.DynamicRelationBody{
										Kind:        "self",
										Types:       []*intermediate.TypeReference{{Namespace: "test", Name: "super"}},
										Cardinality: "AtMostOne",
									},
								},
								{
									Name: &intermediate.DynamicName{Kind: "literal", Value: "test_sub_enabled"},
									Body: intermediate.DynamicRelationBody{
										Kind: "union",
										Left: &intermediate.DynamicRelationBody{
											Kind:     "reference",
											Relation: &intermediate.DynamicName{Kind: "literal", Value: "test_sub_direct_enabled"},
										},
										Right: &intermediate.DynamicRelationBody{
											Kind:        "nested_reference",
											Relation:    &intermediate.DynamicName{Kind: "literal", Value: "parent"},
											SubRelation: &intermediate.DynamicName{Kind: "literal", Value: "test_sub_enabled"},
										},
									},
								},
							},
						},
					},
				},
			},
			ExtensionReferences: []*intermediate.ExtensionReference{{
				Namespace: ns,
				Name:      "sub",
				Params:    map[string]string{},
			}},
		},
	})
}

func TestKSILVisitorTypeWithCommon(t *testing.T) {
	v := NewKSILVisitor()

	c := v.VisitRelation("workspace", "rbac", "workspace", "AtMostOne", v.VisitUUIDDataType())
	p := v.VisitPermission("view", v.VisitSubReferenceExpression("workspace", "inventory_host_view"))

	assert.NoError(t, v.VisitResource("host", "hbi", &output.Members{
		RelationFields: []any{c},
	}, &output.Members{
		Permissions: []any{p},
	}, nil))

	verifyKSILResults(t, v, map[string]*intermediate.Namespace{
		"hbi.json": &intermediate.Namespace{
			Name: "hbi",
			Types: []*intermediate.Type{
				{
					Name: "host",
					Relations: []*intermediate.Relation{
						{
							Name: "workspace",
							Body: &intermediate.RelationBody{
								Kind:        "self",
								Types:       []*intermediate.TypeReference{{Namespace: "rbac", Name: "workspace"}},
								Cardinality: "AtMostOne",
							},
						},
						{
							Name: "view",
							Body: &intermediate.RelationBody{
								Kind:        "nested_reference",
								Relation:    "workspace",
								SubRelation: "inventory_host_view",
							},
						},
					},
				},
			},
		},
	})
}

func verifyKSILResults(t *testing.T, visitor *KSILVisitor, expected map[string]*intermediate.Namespace) {
	t.Helper()

	outputs, err := visitor.Results()
	if !assert.NoError(t, err) {
		return
	}

	used := map[string]bool{}

	for _, output := range outputs {
		expectedEntry, ok := expected[output.Path]
		if !assert.Truef(t, ok, "no expected entry found for path: %s", output.Path) {
			return
		}

		used[output.Path] = true

		_, err := expectedEntry.ToSemantic()
		assert.NoError(t, err, "expected result failed to project to a semantic model for path: %s", output.Path)

		var expectedJS strings.Builder
		assert.NoError(t, intermediate.Store(expectedEntry, &expectedJS))

		assert.JSONEqf(t, expectedJS.String(), string(output.Contents), "expected and actual json did not match. Expected: %s\n\nActual: %s", expectedJS.String(), string(output.Contents))
	}

	for path, _ := range expected {
		assert.Truef(t, used[path], "Expected path %s was not used", path)
	}
}
