// Package compile_test provides comprehensive tests for the public Compile API.
//
// These tests exercise the full SchemaVisitor interface and Compile function:
//   - Basic resource definitions with data fields
//   - Resources with relations (various cardinalities)
//   - Resources with permissions (simple and complex expressions)
//   - Resource inheritance (extends)
//   - Multiple file processing
//   - All data types (text, uuid, numeric_id, boolean, date_time, enum, nullable, union, array)
//   - Permission expressions (and, or, unless, references, subreferences)
//   - Error handling (missing files, parse errors, visitor errors)
//   - Deterministic ordering
//   - Edge cases (empty files, non-.star files)
package compile

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSpyVisitor implements SchemaVisitor for testing purposes.
// It captures all visitor calls in a structured format for JSON comparison.
type testSpyVisitor struct {
	root node
}

type node map[string]any

func newTestSpyVisitor() *testSpyVisitor {
	return &testSpyVisitor{
		root: make(node),
	}
}

func (v *testSpyVisitor) BeginType(name string) {}

func (v *testSpyVisitor) VisitResource(typeName string, reporter string, commonMembers *Members, reporterMembers *Members, extendsResource *ResourceTypeReference) error {
	entry, exists := v.root[typeName].(node)
	if !exists {
		entry = createNode(map[string]any{"common": nil, "reporters": node{}})
		v.root[typeName] = entry
	}
	if commonMembers != nil && entry["common"] == nil {
		entry["common"] = createNode(map[string]any{
			"fields":    commonMembers.DataFields,
			"relations": commonMembers.RelationFields,
			"permissions": commonMembers.Permissions,
		})
	}
	if reporter != "" {
		reporters := entry["reporters"].(node)
		if _, dup := reporters[reporter]; dup {
			return fmt.Errorf("resource %s: reporter '%s' registered more than once", typeName, reporter)
		}

		data := createNode(map[string]any{
			"fields":      reporterMembers.DataFields,
			"relations":   reporterMembers.RelationFields,
			"permissions": reporterMembers.Permissions,
		})
		if extendsResource != nil {
			data["extends"] = createNode(map[string]any{
				"name":     extendsResource.Name,
				"reporter": extendsResource.Reporter,
			})
		}
		reporters[reporter] = data
	}

	return nil
}

func (v *testSpyVisitor) VisitDataField(name string, required bool, description *string, dataType any) any {
	result := createNode(map[string]any{"name": name, "required": required, "type": dataType})
	if description != nil {
		result["description"] = *description
	}
	return result
}

func (v *testSpyVisitor) VisitTextDataType(minLength *int, maxLength *int, regex *string) any {
	return createNode(map[string]any{"kind": "text", "minLength": minLength, "maxLength": maxLength, "regex": regex})
}

func (v *testSpyVisitor) VisitUUIDDataType() any {
	return createNode(map[string]any{"kind": "uuid"})
}

func (v *testSpyVisitor) VisitNumericIDDataType(min *int, max *int) any {
	return createNode(map[string]any{"kind": "numeric_id", "min": min, "max": max})
}

func (v *testSpyVisitor) VisitBooleanDataType() any {
	return createNode(map[string]any{"kind": "boolean"})
}

func (v *testSpyVisitor) VisitDateTimeDataType() any {
	return createNode(map[string]any{"kind": "date_time"})
}

func (v *testSpyVisitor) VisitEnumDataType(values []string) any {
	return createNode(map[string]any{"kind": "enum", "values": values})
}

func (v *testSpyVisitor) VisitNullableDataType(inner any) any {
	return createNode(map[string]any{"kind": "nullable", "inner": inner})
}

func (v *testSpyVisitor) VisitCompositeDataType(dataTypes []any) any {
	return createNode(map[string]any{"kind": "composite", "types": dataTypes})
}

func (v *testSpyVisitor) VisitArrayDataType(items any) any {
	return createNode(map[string]any{"kind": "array", "items": items})
}

func (v *testSpyVisitor) VisitObjectDataType(properties []any, required []string) any {
	return createNode(map[string]any{"kind": "object", "properties": properties, "required": required})
}

func (v *testSpyVisitor) VisitAnd(left any, right any) any {
	return createNode(map[string]any{"kind": "and", "left": left, "right": right})
}

func (v *testSpyVisitor) VisitOr(left any, right any) any {
	return createNode(map[string]any{"kind": "or", "left": left, "right": right})
}

func (v *testSpyVisitor) VisitUnless(left any, right any) any {
	return createNode(map[string]any{"kind": "unless", "left": left, "right": right})
}

func (v *testSpyVisitor) VisitReferenceExpression(name string) any {
	return createNode(map[string]any{"kind": "reference", "name": name})
}

func (v *testSpyVisitor) VisitSubReferenceExpression(name string, sub string) any {
	return createNode(map[string]any{"kind": "subreference", "name": name, "sub": sub})
}

func (v *testSpyVisitor) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any {
	return createNode(map[string]any{
		"kind":        "relation",
		"name":        name,
		"reporter":    reporter,
		"typeName":    typeName,
		"cardinality": cardinality,
		"dataType":    idType,
	})
}

func (v *testSpyVisitor) BeginPermission(name string) {}

func (v *testSpyVisitor) VisitPermission(name string, body any) any {
	return createNode(map[string]any{"kind": "permission", "name": name, "body": body})
}

func (v *testSpyVisitor) Results() ([]OutputEntry, error) {
	return nil, nil
}

func (v *testSpyVisitor) assertJSON(t *testing.T, expected string) bool {
	actual, err := json.Marshal(v.root)
	if !assert.NoError(t, err) {
		return false
	}
	success := assert.JSONEq(t, expected, string(actual))
	if !success {
		t.Logf("Actual JSON did not match expected: %s", string(actual))
	}
	return success
}

func createNode(data map[string]any) node {
	result := node{}
	for key, value := range data {
		// Skip nil, empty slices, and empty maps, and nil pointers
		switch v := value.(type) {
		case nil:
			continue
		case []any:
			if len(v) == 0 {
				continue
			}
		case map[string]any:
			if len(v) == 0 {
				continue
			}
		case *string:
			if v == nil {
				continue
			}
		case *int:
			if v == nil {
				continue
			}
		case *bool:
			if v == nil {
				continue
			}
		}

		result[key] = value
	}
	return result
}

// loadKesselStar loads the kessel.star prelude from the schema directory.
func loadKesselStar(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile("../../schema/kessel.star")
	require.NoError(t, err, "failed to read kessel.star prelude")
	return content
}

func TestCompile_SimpleResource(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "text", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid(),
    common={
        "workspace_id": field(text(), required=True),
    },
    fields={
        "insights_id": field(uuid()),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	spy.assertJSON(t, `{
		"host": {
			"common": {
				"fields": [
					{"name": "workspace_id", "required": true, "type": {"kind": "text"}}
				]
			},
			"reporters": {
				"hbi": {
					"fields": [
						{"name": "insights_id", "required": false, "type": {"kind": "uuid"}}
					]
				}
			}
		}
	}`)
}

func TestCompile_ResourceWithPermissions(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "text", "uuid", "many", "self")

workspace = resource(
    reporter="rbac",
    id_type=uuid(),
    common={
        "name": field(text(), required=True),
    },
    fields={
        "member": many(self()),
    },
    permissions={
        "view": lambda p: p.member,
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify that the resource, fields, and permissions were captured
	assert.Contains(t, spy.root, "workspace")
	resource := spy.root["workspace"].(node)
	assert.Contains(t, resource, "common")
	assert.Contains(t, resource, "reporters")

	reporters := resource["reporters"].(node)
	assert.Contains(t, reporters, "rbac")
	rbac := reporters["rbac"].(node)

	// Verify permissions exist
	assert.Contains(t, rbac, "permissions")
	permissions := rbac["permissions"].([]any)
	assert.Len(t, permissions, 1)
}

func TestCompile_ResourceWithInheritance(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "text", "uuid")

# Base resource
base = resource(
    reporter="base-reporter",
    id_type=uuid(),
    common={
        "common_field": field(text(), required=True),
    },
    fields={
        "base_field": field(text()),
    }
)

# Extended resource
extended = resource(
    reporter="extended-reporter",
    extends=base,
    fields={
        "extended_field": field(text()),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify base resource
	assert.Contains(t, spy.root, "base")

	// Verify extended resource
	assert.Contains(t, spy.root, "extended")
	extended := spy.root["extended"].(node)
	reporters := extended["reporters"].(node)
	extendedReporter := reporters["extended-reporter"].(node)

	// Verify extends reference
	assert.Contains(t, extendedReporter, "extends")
	extendsRef := extendedReporter["extends"].(node)
	assert.Equal(t, "base", extendsRef["name"])
	assert.Equal(t, "base-reporter", extendsRef["reporter"])
}

func TestCompile_MultipleFiles(t *testing.T) {
	kessel := loadKesselStar(t)

	file1 := `
load("kessel.star", "resource", "field", "uuid")

workspace = resource(
    reporter="rbac",
    id_type=uuid()
)
`

	file2 := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"file1.star":  []byte(file1),
		"file2.star":  []byte(file2),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify both resources were processed
	assert.Contains(t, spy.root, "workspace")
	assert.Contains(t, spy.root, "host")
}

func TestCompile_AllDataTypes(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "text", "uuid", "numeric_id", "boolean", "date_time", "enum", "nullable", "union", "array", "object")

example = resource(
    reporter="test",
    id_type=uuid(),
    fields={
        "text_field": field(text(minLength=1, maxLength=100, regex="^[a-z]+$")),
        "uuid_field": field(uuid()),
        "numeric_field": field(numeric_id(min=1, max=1000)),
        "bool_field": field(boolean()),
        "datetime_field": field(date_time()),
        "enum_field": field(enum(["active", "inactive"])),
        "nullable_field": field(nullable(text())),
        "union_field": field(union(text(), numeric_id())),
        "array_field": field(array(text())),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify the resource was processed with all data types
	assert.Contains(t, spy.root, "example")
	resource := spy.root["example"].(node)
	reporters := resource["reporters"].(node)
	testReporter := reporters["test"].(node)
	fields := testReporter["fields"].([]any)

	// Should have all 9 fields
	assert.Len(t, fields, 9)
}

func TestCompile_MissingKesselStar(t *testing.T) {
	schema := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"test.star": []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)

	// Should fail because kessel.star is not provided
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kessel.star")
}

func TestCompile_ParseError(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource")

# Syntax error: unclosed string
host = resource(
    reporter="hbi
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)

	// Should fail with parse error
	assert.Error(t, err)
}

func TestCompile_VisitorError(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	// errorVisitor always returns an error from VisitResource
	errorVisitor := &errorReturningVisitor{expectedErr: fmt.Errorf("visitor error")}
	err := Compile(files, errorVisitor)

	// Should propagate visitor error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "visitor error")
}

func TestCompile_NonStarFilesIgnored(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
		"README.md":   []byte("# Documentation"),
		"config.json": []byte(`{"key": "value"}`),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Should only process .star files
	assert.Contains(t, spy.root, "host")
}

func TestCompile_DeterministicOrdering(t *testing.T) {
	kessel := loadKesselStar(t)

	// Create multiple files with names that would be processed differently if not sorted
	files := map[string][]byte{
		"kessel.star": kessel,
		"z.star": []byte(`load("kessel.star", "resource", "field", "uuid")
z = resource(reporter="z", id_type=uuid())`),
		"a.star": []byte(`load("kessel.star", "resource", "field", "uuid")
a = resource(reporter="a", id_type=uuid())`),
		"m.star": []byte(`load("kessel.star", "resource", "field", "uuid")
m = resource(reporter="m", id_type=uuid())`),
	}

	spy1 := newTestSpyVisitor()
	err1 := Compile(files, spy1)
	require.NoError(t, err1)

	spy2 := newTestSpyVisitor()
	err2 := Compile(files, spy2)
	require.NoError(t, err2)

	// Results should be identical
	json1, _ := json.Marshal(spy1.root)
	json2, _ := json.Marshal(spy2.root)
	assert.JSONEq(t, string(json1), string(json2))
}

func TestCompile_RelationsWithCardinalities(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid", "many", "one", "at_most_one", "at_least_one", "self")

workspace = resource(
    reporter="rbac",
    id_type=uuid(),
    fields={
        "parent": at_most_one(self()),
        "owner": one(self()),
        "members": many(self()),
        "viewers": at_least_one(self()),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify all relations with different cardinalities were processed
	assert.Contains(t, spy.root, "workspace")
	resource := spy.root["workspace"].(node)
	reporters := resource["reporters"].(node)
	rbac := reporters["rbac"].(node)
	relations := rbac["relations"].([]any)

	// Should have 4 relations
	assert.Len(t, relations, 4)

	// Verify cardinalities are captured
	cardinalities := make(map[string]string)
	for _, rel := range relations {
		r := rel.(node)
		name := r["name"].(string)
		cardinality := r["cardinality"].(string)
		cardinalities[name] = cardinality
	}

	assert.Equal(t, "AtMostOne", cardinalities["parent"])
	assert.Equal(t, "ExactlyOne", cardinalities["owner"])
	assert.Equal(t, "Many", cardinalities["members"])
	assert.Equal(t, "AtLeastOne", cardinalities["viewers"])
}

func TestCompile_PermissionExpressions(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid", "many", "self")

workspace = resource(
    reporter="rbac",
    id_type=uuid(),
    fields={
        "member": many(self()),
        "admin": many(self()),
        "viewer": many(self()),
    },
    permissions={
        "view": lambda p: p.member.union(p.viewer),
        "edit": lambda p: p.member.intersect(p.admin),
        "delete": lambda p: p.admin.exclude(p.viewer),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify permissions with complex expressions were processed
	assert.Contains(t, spy.root, "workspace")
	resource := spy.root["workspace"].(node)
	reporters := resource["reporters"].(node)
	rbac := reporters["rbac"].(node)
	permissions := rbac["permissions"].([]any)

	// Should have 3 permissions
	assert.Len(t, permissions, 3)

	// Verify permission bodies contain the right expression types
	permMap := make(map[string]node)
	for _, perm := range permissions {
		p := perm.(node)
		name := p["name"].(string)
		permMap[name] = p
	}

	// View should have an "or" expression
	viewBody := permMap["view"]["body"].(node)
	assert.Equal(t, "or", viewBody["kind"])

	// Edit should have an "and" expression
	editBody := permMap["edit"]["body"].(node)
	assert.Equal(t, "and", editBody["kind"])

	// Delete should have an "unless" expression
	deleteBody := permMap["delete"]["body"].(node)
	assert.Equal(t, "unless", deleteBody["kind"])
}

func TestCompile_DifferentIDTypes(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid", "text", "numeric_id")

uuid_resource = resource(
    reporter="uuid-reporter",
    id_type=uuid()
)

text_resource = resource(
    reporter="text-reporter",
    id_type=text()
)

numeric_resource = resource(
    reporter="numeric-reporter",
    id_type=numeric_id()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify all three resources with different ID types were processed
	assert.Contains(t, spy.root, "uuid_resource")
	assert.Contains(t, spy.root, "text_resource")
	assert.Contains(t, spy.root, "numeric_resource")
}

func TestCompile_SubReferences(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid", "many", "self")

workspace = resource(
    reporter="rbac",
    id_type=uuid(),
    fields={
        "child": many(self()),
        "descendants": many(self()),
    },
    permissions={
        "view": lambda p: p.child.union(p.child.descendants),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify permission with subreference was processed
	assert.Contains(t, spy.root, "workspace")
	resource := spy.root["workspace"].(node)
	reporters := resource["reporters"].(node)
	rbac := reporters["rbac"].(node)
	permissions := rbac["permissions"].([]any)

	// Should have 1 permission
	assert.Len(t, permissions, 1)

	perm := permissions[0].(node)
	body := perm["body"].(node)

	// Body should be an "or" with a reference and a subreference
	assert.Equal(t, "or", body["kind"])
	left := body["left"].(node)
	right := body["right"].(node)

	assert.Equal(t, "reference", left["kind"])
	assert.Equal(t, "subreference", right["kind"])
	assert.Equal(t, "child", right["name"])
	assert.Equal(t, "descendants", right["sub"])
}

func TestCompile_EmptyFiles(t *testing.T) {
	kessel := loadKesselStar(t)

	files := map[string][]byte{
		"kessel.star": kessel,
		"empty.star":  []byte("# Just a comment\n"),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Should process without error even with empty files
	assert.Empty(t, spy.root)
}

func TestCompile_DescriptionField(t *testing.T) {
	kessel := loadKesselStar(t)

	desc := "This is a workspace identifier"
	schema := `
load("kessel.star", "resource", "field", "text", "uuid")

workspace = resource(
    reporter="rbac",
    id_type=uuid(),
    fields={
        "name": field(text(), required=True, description="This is a workspace identifier"),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify description was captured
	resource := spy.root["workspace"].(node)
	reporters := resource["reporters"].(node)
	rbac := reporters["rbac"].(node)
	fields := rbac["fields"].([]any)

	assert.Len(t, fields, 1)
	field := fields[0].(node)
	assert.Equal(t, "name", field["name"])
	assert.Equal(t, desc, field["description"])
}

func TestCompile_ConstrainedDataTypes(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "text", "uuid", "numeric_id")

example = resource(
    reporter="test",
    id_type=uuid(),
    fields={
        "constrained_text": field(text(minLength=5, maxLength=50, regex="^[A-Z]+$")),
        "constrained_numeric": field(numeric_id(min=100, max=999)),
    }
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	spy := newTestSpyVisitor()
	err := Compile(files, spy)
	require.NoError(t, err)

	// Verify constraints were captured
	resource := spy.root["example"].(node)
	reporters := resource["reporters"].(node)
	testReporter := reporters["test"].(node)
	fields := testReporter["fields"].([]any)

	assert.Len(t, fields, 2)

	// Verify text constraints
	textField := fields[0].(node)
	textType := textField["type"].(node)
	assert.Equal(t, "text", textType["kind"])
	assert.Equal(t, 5, *textType["minLength"].(*int))
	assert.Equal(t, 50, *textType["maxLength"].(*int))
	assert.Equal(t, "^[A-Z]+$", *textType["regex"].(*string))

	// Verify numeric constraints
	numericField := fields[1].(node)
	numericType := numericField["type"].(node)
	assert.Equal(t, "numeric_id", numericType["kind"])
	assert.Equal(t, 100, *numericType["min"].(*int))
	assert.Equal(t, 999, *numericType["max"].(*int))
}

func TestCompile_VisitorResults(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	// Test visitor that returns results
	visitor := &resultsReturningVisitor{
		results: []OutputEntry{
			{Path: "output1.txt", Contents: []byte("content1")},
			{Path: "output2.txt", Contents: []byte("content2")},
		},
	}

	err := Compile(files, visitor)
	require.NoError(t, err)

	// Verify Results() was callable
	results, err := visitor.Results()
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "output1.txt", results[0].Path)
	assert.Equal(t, []byte("content1"), results[0].Contents)
}

func TestCompile_VisitorResultsError(t *testing.T) {
	kessel := loadKesselStar(t)

	schema := `
load("kessel.star", "resource", "field", "uuid")

host = resource(
    reporter="hbi",
    id_type=uuid()
)
`

	files := map[string][]byte{
		"kessel.star": kessel,
		"test.star":   []byte(schema),
	}

	// Test visitor that returns an error from Results()
	visitor := &resultsReturningVisitor{
		resultsErr: fmt.Errorf("results error"),
	}

	// Compile should succeed
	err := Compile(files, visitor)
	require.NoError(t, err)

	// But Results() should return error
	_, err = visitor.Results()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "results error")
}

// errorReturningVisitor is a minimal visitor that returns an error from VisitResource.
type errorReturningVisitor struct {
	expectedErr error
}

func (v *errorReturningVisitor) BeginType(name string)                                                                                     {}
func (v *errorReturningVisitor) VisitResource(typeName string, reporter string, commonMembers *Members, reporterMembers *Members, extendsResource *ResourceTypeReference) error {
	return v.expectedErr
}
func (v *errorReturningVisitor) VisitDataField(name string, required bool, description *string, dataType any) any { return nil }
func (v *errorReturningVisitor) VisitTextDataType(minLength *int, maxLength *int, regex *string) any               { return nil }
func (v *errorReturningVisitor) VisitUUIDDataType() any                                                             { return nil }
func (v *errorReturningVisitor) VisitNumericIDDataType(min *int, max *int) any                                      { return nil }
func (v *errorReturningVisitor) VisitBooleanDataType() any                                                          { return nil }
func (v *errorReturningVisitor) VisitDateTimeDataType() any                                                         { return nil }
func (v *errorReturningVisitor) VisitEnumDataType(values []string) any                                              { return nil }
func (v *errorReturningVisitor) VisitNullableDataType(inner any) any                                                { return nil }
func (v *errorReturningVisitor) VisitCompositeDataType(dataTypes []any) any                                         { return nil }
func (v *errorReturningVisitor) VisitArrayDataType(items any) any                                                   { return nil }
func (v *errorReturningVisitor) VisitObjectDataType(properties []any, required []string) any                        { return nil }
func (v *errorReturningVisitor) VisitAnd(left any, right any) any                                                   { return nil }
func (v *errorReturningVisitor) VisitOr(left any, right any) any                                                    { return nil }
func (v *errorReturningVisitor) VisitUnless(left any, right any) any                                                { return nil }
func (v *errorReturningVisitor) VisitReferenceExpression(name string) any                                           { return nil }
func (v *errorReturningVisitor) VisitSubReferenceExpression(name string, sub string) any                            { return nil }
func (v *errorReturningVisitor) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any { return nil }
func (v *errorReturningVisitor) BeginPermission(name string)                                                        {}
func (v *errorReturningVisitor) VisitPermission(name string, body any) any                                          { return nil }
func (v *errorReturningVisitor) Results() ([]OutputEntry, error)                                                    { return nil, nil }

// resultsReturningVisitor is a visitor that returns configured results.
type resultsReturningVisitor struct {
	results    []OutputEntry
	resultsErr error
}

func (v *resultsReturningVisitor) BeginType(name string)                                                             {}
func (v *resultsReturningVisitor) VisitResource(typeName string, reporter string, commonMembers *Members, reporterMembers *Members, extendsResource *ResourceTypeReference) error {
	return nil
}
func (v *resultsReturningVisitor) VisitDataField(name string, required bool, description *string, dataType any) any { return nil }
func (v *resultsReturningVisitor) VisitTextDataType(minLength *int, maxLength *int, regex *string) any               { return nil }
func (v *resultsReturningVisitor) VisitUUIDDataType() any                                                             { return nil }
func (v *resultsReturningVisitor) VisitNumericIDDataType(min *int, max *int) any                                      { return nil }
func (v *resultsReturningVisitor) VisitBooleanDataType() any                                                          { return nil }
func (v *resultsReturningVisitor) VisitDateTimeDataType() any                                                         { return nil }
func (v *resultsReturningVisitor) VisitEnumDataType(values []string) any                                              { return nil }
func (v *resultsReturningVisitor) VisitNullableDataType(inner any) any                                                { return nil }
func (v *resultsReturningVisitor) VisitCompositeDataType(dataTypes []any) any                                         { return nil }
func (v *resultsReturningVisitor) VisitArrayDataType(items any) any                                                   { return nil }
func (v *resultsReturningVisitor) VisitObjectDataType(properties []any, required []string) any                        { return nil }
func (v *resultsReturningVisitor) VisitAnd(left any, right any) any                                                   { return nil }
func (v *resultsReturningVisitor) VisitOr(left any, right any) any                                                    { return nil }
func (v *resultsReturningVisitor) VisitUnless(left any, right any) any                                                { return nil }
func (v *resultsReturningVisitor) VisitReferenceExpression(name string) any                                           { return nil }
func (v *resultsReturningVisitor) VisitSubReferenceExpression(name string, sub string) any                            { return nil }
func (v *resultsReturningVisitor) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any { return nil }
func (v *resultsReturningVisitor) BeginPermission(name string)                                                        {}
func (v *resultsReturningVisitor) VisitPermission(name string, body any) any                                          { return nil }
func (v *resultsReturningVisitor) Results() ([]OutputEntry, error) {
	return v.results, v.resultsErr
}
