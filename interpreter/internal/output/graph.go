package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// GraphVisitor builds the canonical graph representation of a schema: resource
// types as nodes, relations and inheritance as edges, with permission rewrite
// trees carried on each node. The model is schema-native and contains no
// SpiceDB / KSIL / JSON-Schema concepts. See GRAPH.md for the artifact spec.
type GraphVisitor struct {
	// nodes keyed by logical type name; facets are merged as they are visited.
	nodes map[string]*graphNode
	// edges keyed by edge id so relations shared across reporter facets (common
	// members are re-visited per facet) are emitted exactly once.
	edges map[string]*graphEdge
}

func NewGraphVisitor() *GraphVisitor {
	return &GraphVisitor{
		nodes: make(map[string]*graphNode),
		edges: make(map[string]*graphEdge),
	}
}

var _ SchemaVisitor = (*GraphVisitor)(nil)

type graphOutput struct {
	Version string       `json:"version"`
	Nodes   []*graphNode `json:"nodes"`
	Edges   []*graphEdge `json:"edges"`
}

type graphNode struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	TypeName  string                 `json:"typeName"`
	Common    *graphMembers          `json:"common,omitempty"`
	Reporters map[string]*graphFacet `json:"reporters"`
}

type graphMembers struct {
	DataFields  []any `json:"dataFields,omitempty"`
	Permissions []any `json:"permissions,omitempty"`
}

type graphFacet struct {
	DataFields  []any         `json:"dataFields,omitempty"`
	Permissions []any         `json:"permissions,omitempty"`
	Extends     *graphTypeRef `json:"extends,omitempty"`
}

type graphTypeRef struct {
	TypeName string `json:"typeName"`
	Reporter string `json:"reporter"`
}

type graphEdge struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	Name           string `json:"name,omitempty"`
	Cardinality    string `json:"cardinality,omitempty"`
	Scope          string `json:"scope,omitempty"`
	SourceReporter string `json:"sourceReporter,omitempty"`
	TargetReporter string `json:"targetReporter,omitempty"`
	Self           bool   `json:"self,omitempty"`
}

// relationInfo is the value returned by VisitRelation. Edges are built later, in
// VisitResource, where the source type/reporter and the member scope (common vs
// reporter) are known.
type relationInfo struct {
	name           string
	cardinality    string
	targetType     string
	targetReporter string
}

func (v *GraphVisitor) BeginType(name string) {}

func (v *GraphVisitor) VisitResource(typeName string, reporter string, commonMembers *Members, reporterMembers *Members, extendsResource *ResourceTypeReference) error {
	node, ok := v.nodes[typeName]
	if !ok {
		node = &graphNode{
			ID:        typeName,
			Kind:      "resource",
			TypeName:  typeName,
			Reporters: map[string]*graphFacet{},
		}
		v.nodes[typeName] = node
	}

	// Common members are shared across facets and re-visited per facet; record
	// them (and their relation edges) once.
	if node.Common == nil {
		if members := toMembers(commonMembers); members != nil {
			node.Common = members
		}
		v.addRelationEdges(typeName, "", commonMembers)
	}

	if reporter != "" {
		if _, dup := node.Reporters[reporter]; dup {
			return fmt.Errorf("resource %s: reporter '%s' registered more than once", typeName, reporter)
		}

		facet := &graphFacet{}
		if reporterMembers != nil {
			facet.DataFields = reporterMembers.DataFields
			facet.Permissions = reporterMembers.Permissions
		}
		if extendsResource != nil {
			facet.Extends = &graphTypeRef{TypeName: extendsResource.Name, Reporter: extendsResource.Reporter}
			v.addInheritsEdge(typeName, reporter, extendsResource)
		}
		node.Reporters[reporter] = facet

		v.addRelationEdges(typeName, reporter, reporterMembers)
	}

	return nil
}

// addRelationEdges emits one relation edge per relation member. A reporter of ""
// denotes the shared common scope.
func (v *GraphVisitor) addRelationEdges(sourceType string, reporter string, members *Members) {
	if members == nil {
		return
	}
	for _, r := range members.RelationFields {
		rel, ok := r.(*relationInfo)
		if !ok {
			continue
		}

		var id, scope string
		if reporter == "" {
			scope = "common"
			id = fmt.Sprintf("%s#common.%s", sourceType, rel.name)
		} else {
			scope = "reporter"
			id = fmt.Sprintf("%s#reporter:%s.%s", sourceType, reporter, rel.name)
		}
		if _, exists := v.edges[id]; exists {
			continue
		}

		v.edges[id] = &graphEdge{
			ID:             id,
			Kind:           "relation",
			Source:         sourceType,
			Target:         rel.targetType,
			Name:           rel.name,
			Cardinality:    rel.cardinality,
			Scope:          scope,
			SourceReporter: reporter,
			TargetReporter: rel.targetReporter,
			Self:           rel.targetType == sourceType,
		}
	}
}

func (v *GraphVisitor) addInheritsEdge(sourceType string, reporter string, parent *ResourceTypeReference) {
	id := fmt.Sprintf("%s#reporter:%s=>inherits", sourceType, reporter)
	if _, exists := v.edges[id]; exists {
		return
	}
	v.edges[id] = &graphEdge{
		ID:             id,
		Kind:           "inherits",
		Source:         sourceType,
		Target:         parent.Name,
		SourceReporter: reporter,
		TargetReporter: parent.Reporter,
	}
}

func toMembers(m *Members) *graphMembers {
	if m == nil {
		return nil
	}
	if len(m.DataFields) == 0 && len(m.Permissions) == 0 {
		return nil
	}
	return &graphMembers{DataFields: m.DataFields, Permissions: m.Permissions}
}

func (v *GraphVisitor) VisitRelation(name string, reporter string, typeName string, cardinality string, idType any) any {
	return &relationInfo{
		name:           name,
		cardinality:    cardinality,
		targetType:     typeName,
		targetReporter: reporter,
	}
}

func (v *GraphVisitor) VisitDataField(name string, required bool, description *string, dataType any) any {
	field := map[string]any{"name": name, "type": dataType, "required": required}
	if description != nil {
		field["description"] = *description
	}
	return field
}

func (v *GraphVisitor) VisitTextDataType(minLength *int, maxLength *int, regex *string) any {
	dt := map[string]any{"kind": "text"}
	if minLength != nil {
		dt["minLength"] = *minLength
	}
	if maxLength != nil {
		dt["maxLength"] = *maxLength
	}
	if regex != nil {
		dt["regex"] = *regex
	}
	return dt
}

func (v *GraphVisitor) VisitUUIDDataType() any {
	return map[string]any{"kind": "uuid"}
}

func (v *GraphVisitor) VisitNumericIDDataType(min *int, max *int) any {
	dt := map[string]any{"kind": "numeric_id"}
	if min != nil {
		dt["min"] = *min
	}
	if max != nil {
		dt["max"] = *max
	}
	return dt
}

func (v *GraphVisitor) VisitBooleanDataType() any {
	return map[string]any{"kind": "boolean"}
}

func (v *GraphVisitor) VisitDateTimeDataType() any {
	return map[string]any{"kind": "date_time"}
}

func (v *GraphVisitor) VisitEnumDataType(values []string) any {
	return map[string]any{"kind": "enum", "values": values}
}

func (v *GraphVisitor) VisitNullableDataType(inner any) any {
	return map[string]any{"kind": "nullable", "inner": inner}
}

func (v *GraphVisitor) VisitCompositeDataType(dataTypes []any) any {
	return map[string]any{"kind": "composite", "types": dataTypes}
}

func (v *GraphVisitor) VisitArrayDataType(items any) any {
	return map[string]any{"kind": "array", "items": items}
}

func (v *GraphVisitor) VisitObjectDataType(properties []any, required []string) any {
	dt := map[string]any{"kind": "object", "properties": properties}
	if len(required) > 0 {
		dt["required"] = required
	}
	return dt
}

func (v *GraphVisitor) VisitAnd(left any, right any) any {
	return map[string]any{"kind": "and", "left": left, "right": right}
}

func (v *GraphVisitor) VisitOr(left any, right any) any {
	return map[string]any{"kind": "or", "left": left, "right": right}
}

func (v *GraphVisitor) VisitUnless(left any, right any) any {
	return map[string]any{"kind": "unless", "left": left, "right": right}
}

func (v *GraphVisitor) VisitReferenceExpression(name string) any {
	return map[string]any{"kind": "reference", "name": name}
}

func (v *GraphVisitor) VisitSubReferenceExpression(name string, sub string) any {
	return map[string]any{"kind": "subreference", "name": name, "sub": sub}
}

func (v *GraphVisitor) BeginPermission(name string) {}

func (v *GraphVisitor) VisitPermission(name string, body any) any {
	return map[string]any{"name": name, "body": body}
}

func (v *GraphVisitor) Results() ([]OutputEntry, error) {
	nodes := make([]*graphNode, 0, len(v.nodes))
	for _, n := range v.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edges := make([]*graphEdge, 0, len(v.edges))
	for _, e := range v.edges {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	out := graphOutput{Version: "1", Nodes: nodes, Edges: edges}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("error marshaling graph: %w", err)
	}

	return []OutputEntry{{Path: "graph.json", Contents: buf.Bytes()}}, nil
}
