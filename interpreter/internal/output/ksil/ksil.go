// Package ksil holds the KSIL (KSL intermediate) output visitor. It lives in its
// own package — rather than in internal/output alongside the other visitors —
// because it is the only visitor that depends on ksl-schema-language, which in
// turn pulls in SpiceDB's generated protobuf types and google.golang.org/grpc.
// Keeping it separate lets the graph/JSON-Schema consumers (and especially the
// WebAssembly playground build) import internal/output without linking grpc and
// the SpiceDB protos they never call.
package ksil

import (
	"bytes"
	"fmt"

	"github.com/project-kessel/ksl-schema-language/pkg/intermediate"
	"github.com/project-kessel/starlark-unified-schema/internal/output"
)

type KSILVisitor struct {
	namespaces map[string]*intermediate.Namespace
}

func NewKSILVisitor() *KSILVisitor {
	return &KSILVisitor{
		namespaces: make(map[string]*intermediate.Namespace),
	}
}

func (k *KSILVisitor) VisitAnd(left any, right any) any {
	return &intermediate.RelationBody{
		Kind:  "intersect",
		Left:  left.(*intermediate.RelationBody),
		Right: right.(*intermediate.RelationBody),
	}
}

func (k *KSILVisitor) VisitOr(left any, right any) any {
	return &intermediate.RelationBody{
		Kind:  "union",
		Left:  left.(*intermediate.RelationBody),
		Right: right.(*intermediate.RelationBody),
	}
}

func (k *KSILVisitor) VisitUnless(left any, right any) any {
	return &intermediate.RelationBody{
		Kind:  "except",
		Left:  left.(*intermediate.RelationBody),
		Right: right.(*intermediate.RelationBody),
	}
}

func (k *KSILVisitor) BeginRelation(name string) {

}

func (k *KSILVisitor) BeginType(name string) {}

func (k *KSILVisitor) VisitResource(typeName string, reporter string, commonMembers *output.Members, reporterMembers *output.Members, extendsResource *output.ResourceTypeReference) error {
	if _, exists := k.namespaces[reporter]; !exists {
		k.namespaces[reporter] = &intermediate.Namespace{
			Name:                 reporter,
			Types:                []*intermediate.Type{},
			ExtensionDefinitions: []*intermediate.ExtensionDefinition{},
			ExtensionReferences:  []*intermediate.ExtensionReference{},
		}
	}

	// Convert relations from []any to []*intermediate.Relation
	typedRelations := make([]*intermediate.Relation, 0,
		len(commonMembers.RelationFields)+len(reporterMembers.RelationFields)+
			len(commonMembers.Permissions)+len(reporterMembers.Permissions))

	for _, rel := range commonMembers.RelationFields {
		typedRelations = append(typedRelations, rel.(*intermediate.Relation))
	}
	for _, rel := range reporterMembers.RelationFields {
		typedRelations = append(typedRelations, rel.(*intermediate.Relation))
	}
	for _, perm := range commonMembers.Permissions {
		typedRelations = append(typedRelations, perm.(*intermediate.Relation))
	}
	for _, perm := range reporterMembers.Permissions {
		typedRelations = append(typedRelations, perm.(*intermediate.Relation))
	}

	ns := k.namespaces[reporter]
	if extendsResource != nil {
		err := k.constructSubclassExtensionAndAddToNamespace(ns, typeName, typedRelations, extendsResource)
		if err != nil {
			return err
		}
	} else {
		k.constructTypeAndAddToNamespace(ns, typeName, typedRelations)
	}
	return nil
}

func (k *KSILVisitor) constructTypeAndAddToNamespace(ns *intermediate.Namespace, typeName string, relations []*intermediate.Relation) {
	typeObj := &intermediate.Type{
		Name:      typeName,
		Relations: relations,
	}

	// Add to the appropriate namespace
	ns.Types = append(ns.Types, typeObj)
}

func (k *KSILVisitor) constructSubclassExtensionAndAddToNamespace(ns *intermediate.Namespace, typeName string, relations []*intermediate.Relation, parent *output.ResourceTypeReference) error {
	ownsMember := map[string]bool{}
	for _, r := range relations {
		ownsMember[r.Name] = true
	}

	subclass := &intermediate.DynamicType{
		Name:       literalValueToDynamicName(parent.Name),
		Namespace:  &parent.Reporter,
		Visibility: "public", //The default value when not specified in KSL
		Relations:  []*intermediate.DynamicRelation{},
	}

	relation_prefix := ns.Name + "_" + typeName + "_"
	for _, relation := range relations {
		body, err := literalToDynamicBody(relation.Body, ownsMember, relation_prefix)
		if err != nil {
			return err
		}

		dynamicRelation := &intermediate.DynamicRelation{
			Name:             literalValueToDynamicName(relation_prefix + relation.Name),
			Visibility:       relation.Visibility,
			IgnoreDuplicates: false,
			Body:             *body,
		}

		subclass.Relations = append(subclass.Relations, dynamicRelation)
	}

	extension := &intermediate.ExtensionDefinition{
		Name:       typeName,
		Visibility: "internal",
		Params:     []string{},
		Types:      []*intermediate.DynamicType{subclass},
	}

	call := &intermediate.ExtensionReference{
		Namespace: ns.Name,
		Name:      typeName,
		Params:    map[string]string{},
	}

	ns.ExtensionDefinitions = append(ns.ExtensionDefinitions, extension)
	ns.ExtensionReferences = append(ns.ExtensionReferences, call)
	return nil
}

func literalToDynamicBody(body *intermediate.RelationBody, ownsMember map[string]bool, prefix string) (*intermediate.DynamicRelationBody, error) {
	dynamicBody := &intermediate.DynamicRelationBody{
		Kind:        body.Kind,
		Types:       body.Types,
		Cardinality: body.Cardinality,
	}
	switch body.Kind {
	case "self":
		break
	case "reference":
		relation := body.Relation
		if ownsMember[relation] {
			relation = prefix + relation
		}
		dynamicBody.Relation = literalValueToDynamicName(relation)
	case "nested_reference":
		relation := body.Relation
		subrelation := body.SubRelation
		if ownsMember[relation] {
			relation = prefix + relation
		} else {
			if ownsMember[subrelation] {
				subrelation = prefix + subrelation //TODO: this is only correct if the relation portion is self-type
			}
		}

		dynamicBody.Relation = literalValueToDynamicName(relation)
		dynamicBody.SubRelation = literalValueToDynamicName(subrelation)
	case "union", "intersect", "except":
		left, err := literalToDynamicBody(body.Left, ownsMember, prefix)
		if err != nil {
			return nil, err
		}
		dynamicBody.Left = left

		right, err := literalToDynamicBody(body.Right, ownsMember, prefix)
		if err != nil {
			return nil, err
		}
		dynamicBody.Right = right
	default:
		return nil, fmt.Errorf("unrecognized body kind: %s", body.Kind)
	}

	return dynamicBody, nil
}

func literalValueToDynamicName(name string) *intermediate.DynamicName {
	return &intermediate.DynamicName{
		Kind:  "literal",
		Value: name,
	}
}

func (k *KSILVisitor) VisitReferenceExpression(name string) any {
	return &intermediate.RelationBody{
		Kind:     "reference",
		Relation: name,
	}
}

func (k *KSILVisitor) VisitSubReferenceExpression(name string, sub string) any {
	return &intermediate.RelationBody{
		Kind:        "nested_reference",
		Relation:    name,
		SubRelation: sub,
	}
}

func (k *KSILVisitor) BeginPermission(name string) {

}

func (k *KSILVisitor) VisitPermission(name string, body any) any {
	return &intermediate.Relation{
		Name: name,
		Body: body.(*intermediate.RelationBody),
	}
}

func (k *KSILVisitor) Results() ([]output.OutputEntry, error) {
	var outputs []output.OutputEntry

	for nsName, namespace := range k.namespaces {
		var buf bytes.Buffer

		if err := intermediate.Store(namespace, &buf); err != nil {
			return nil, fmt.Errorf("error serializing namespace %s: %w", nsName, err)
		}

		outputs = append(outputs, output.OutputEntry{
			Path:     nsName + ".json",
			Contents: buf.Bytes(),
		})
	}

	return outputs, nil
}

func (k *KSILVisitor) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any {
	isWildcard := cardinality == "All"
	if cardinality == "Many" || isWildcard { // Convert to legacy cardinality.
		cardinality = "Any"
	}

	body := &intermediate.RelationBody{
		Kind:        "self",
		Types:       []*intermediate.TypeReference{{Namespace: reporter, Name: typeName, All: isWildcard}},
		Cardinality: cardinality,
	}

	return &intermediate.Relation{
		Name: name,
		Body: body,
	}
}

func (k *KSILVisitor) VisitDataField(name string, required bool, description *string, dataType any) any {
	return nil
}

func (k *KSILVisitor) VisitTextDataType(minLength *int, maxLength *int, regex *string) any {
	return nil
}

func (k *KSILVisitor) VisitUUIDDataType() any {
	return nil
}

func (k *KSILVisitor) VisitNumericIDDataType(min *int, max *int) any {
	return nil
}

func (k *KSILVisitor) VisitBooleanDataType() any {
	return nil
}

func (k *KSILVisitor) VisitDateTimeDataType() any {
	return nil
}

func (k *KSILVisitor) VisitEnumDataType(values []string) any {
	return nil
}

func (k *KSILVisitor) VisitNullableDataType(inner any) any {
	return nil
}

func (k *KSILVisitor) VisitCompositeDataType(dataTypes []any) any {
	return nil
}

func (k *KSILVisitor) VisitArrayDataType(items any) any {
	return nil
}

func (k *KSILVisitor) VisitObjectDataType(properties []any, required []string) any {
	return nil
}
