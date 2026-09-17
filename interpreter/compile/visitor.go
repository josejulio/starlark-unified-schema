// Package compile provides a public API for driving the Starlark schema
// interpreter with custom visitors. It exposes the SchemaVisitor interface
// and types needed to process schemas from external Go modules.
//
// This package is consumed by external tooling (e.g., project-kessel/schema-graph-tools)
// and forms a stable public API. Changes to the SchemaVisitor interface are
// breaking changes that require coordination with consumers.
package compile

// Members represents the structural components of a resource type.
type Members struct {
	DataFields     []any
	RelationFields []any
	Permissions    []any
}

// ResourceTypeReference identifies a resource type by name and reporter.
type ResourceTypeReference struct {
	Name     string
	Reporter string
}

// OutputEntry represents a single output artifact (e.g., a generated file).
type OutputEntry struct {
	Path     string
	Contents []byte
}

// SchemaVisitor is the interface that custom visitors must implement to
// process Starlark schema definitions. The interpreter drives the visitor
// methods in a defined order as it processes schema files.
//
// The visitor receives cooked, domain-level values (strings, *Members, etc.),
// never raw starlark.Value AST nodes. This allows visitors to live in external
// modules without depending on interpreter internals.
type SchemaVisitor interface {
	// BeginType signals the start of processing a resource type.
	BeginType(name string)

	// VisitResource processes a complete resource type definition.
	VisitResource(typeName string, reporter string, commonMembers *Members, reporterMembers *Members, extendsResource *ResourceTypeReference) error

	// VisitDataField processes a data field definition and returns an opaque
	// handle that the visitor can use to track it.
	VisitDataField(name string, required bool, description *string, dataType any) any

	// Data type visitors - each returns an opaque handle for the type.
	VisitTextDataType(minLength *int, maxLength *int, regex *string) any
	VisitUUIDDataType() any
	VisitNumericIDDataType(min *int, max *int) any
	VisitBooleanDataType() any
	VisitDateTimeDataType() any
	VisitEnumDataType(values []string) any
	VisitNullableDataType(inner any) any
	VisitCompositeDataType(dataTypes []any) any
	VisitArrayDataType(items any) any
	VisitObjectDataType(properties []any, required []string) any

	// Permission expression visitors - each returns an opaque handle.
	VisitAnd(left any, right any) any
	VisitOr(left any, right any) any
	VisitUnless(left any, right any) any
	VisitReferenceExpression(name string) any
	VisitSubReferenceExpression(name string, sub string) any

	// VisitRelation processes a relation field definition.
	VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any

	// Permission visitors.
	BeginPermission(name string)
	VisitPermission(name string, body any) any

	// Results returns the visitor's output artifacts after processing completes.
	Results() ([]OutputEntry, error)
}
