package compile

import (
	"github.com/project-kessel/starlark-unified-schema/internal/output"
)

// visitorAdapter bridges the public compile.SchemaVisitor interface to the
// internal output.SchemaVisitor interface. This allows external code to
// implement the public interface while the internal machinery works with
// the internal one.
type visitorAdapter struct {
	visitor SchemaVisitor
}

// Verify that visitorAdapter implements output.SchemaVisitor at compile time.
var _ output.SchemaVisitor = (*visitorAdapter)(nil)

func (a *visitorAdapter) BeginType(name string) {
	a.visitor.BeginType(name)
}

func (a *visitorAdapter) VisitResource(typeName string, reporter string, commonMembers *output.Members, reporterMembers *output.Members, extendsResource *output.ResourceTypeReference) error {
	// Convert internal types to public types.
	var publicCommon *Members
	if commonMembers != nil {
		publicCommon = &Members{
			DataFields:     commonMembers.DataFields,
			RelationFields: commonMembers.RelationFields,
			Permissions:    commonMembers.Permissions,
		}
	}

	var publicReporter *Members
	if reporterMembers != nil {
		publicReporter = &Members{
			DataFields:     reporterMembers.DataFields,
			RelationFields: reporterMembers.RelationFields,
			Permissions:    reporterMembers.Permissions,
		}
	}

	var publicExtends *ResourceTypeReference
	if extendsResource != nil {
		publicExtends = &ResourceTypeReference{
			Name:     extendsResource.Name,
			Reporter: extendsResource.Reporter,
		}
	}

	return a.visitor.VisitResource(typeName, reporter, publicCommon, publicReporter, publicExtends)
}

func (a *visitorAdapter) VisitDataField(name string, required bool, description *string, dataType any) any {
	return a.visitor.VisitDataField(name, required, description, dataType)
}

func (a *visitorAdapter) VisitTextDataType(minLength *int, maxLength *int, regex *string) any {
	return a.visitor.VisitTextDataType(minLength, maxLength, regex)
}

func (a *visitorAdapter) VisitUUIDDataType() any {
	return a.visitor.VisitUUIDDataType()
}

func (a *visitorAdapter) VisitNumericIDDataType(min *int, max *int) any {
	return a.visitor.VisitNumericIDDataType(min, max)
}

func (a *visitorAdapter) VisitBooleanDataType() any {
	return a.visitor.VisitBooleanDataType()
}

func (a *visitorAdapter) VisitDateTimeDataType() any {
	return a.visitor.VisitDateTimeDataType()
}

func (a *visitorAdapter) VisitEnumDataType(values []string) any {
	return a.visitor.VisitEnumDataType(values)
}

func (a *visitorAdapter) VisitNullableDataType(inner any) any {
	return a.visitor.VisitNullableDataType(inner)
}

func (a *visitorAdapter) VisitCompositeDataType(dataTypes []any) any {
	return a.visitor.VisitCompositeDataType(dataTypes)
}

func (a *visitorAdapter) VisitArrayDataType(items any) any {
	return a.visitor.VisitArrayDataType(items)
}

func (a *visitorAdapter) VisitObjectDataType(properties []any, required []string) any {
	return a.visitor.VisitObjectDataType(properties, required)
}

func (a *visitorAdapter) VisitAnd(left any, right any) any {
	return a.visitor.VisitAnd(left, right)
}

func (a *visitorAdapter) VisitOr(left any, right any) any {
	return a.visitor.VisitOr(left, right)
}

func (a *visitorAdapter) VisitUnless(left any, right any) any {
	return a.visitor.VisitUnless(left, right)
}

func (a *visitorAdapter) VisitReferenceExpression(name string) any {
	return a.visitor.VisitReferenceExpression(name)
}

func (a *visitorAdapter) VisitSubReferenceExpression(name string, sub string) any {
	return a.visitor.VisitSubReferenceExpression(name, sub)
}

func (a *visitorAdapter) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any {
	return a.visitor.VisitRelation(name, reporter, typeName, cardinality, idType)
}

func (a *visitorAdapter) BeginPermission(name string) {
	a.visitor.BeginPermission(name)
}

func (a *visitorAdapter) VisitPermission(name string, body any) any {
	return a.visitor.VisitPermission(name, body)
}

func (a *visitorAdapter) Results() ([]output.OutputEntry, error) {
	publicResults, err := a.visitor.Results()
	if err != nil {
		return nil, err
	}

	// Convert public OutputEntry to internal OutputEntry.
	internalResults := make([]output.OutputEntry, len(publicResults))
	for i, r := range publicResults {
		internalResults[i] = output.OutputEntry{
			Path:     r.Path,
			Contents: r.Contents,
		}
	}

	return internalResults, nil
}
