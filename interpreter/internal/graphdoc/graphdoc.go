// Package graphdoc is the shared read-model for the canonical graph.json artifact
// (see GRAPH.md). The renderer (internal/render), analyzer (internal/analyze) and
// web view (internal/web) are all pure consumers of the documented JSON contract;
// they parse it into these types and derive topology from them here, rather than
// each re-declaring the structs and re-implementing the shared rules (the
// "common" grouping, the UML multiplicity mapping). This keeps those rules — which
// must agree across every consumer — in one place.
//
// It deliberately depends only on the JSON contract, not on the Go types that
// produce it (internal/output): a separate consumer could read the same file the
// same way.
package graphdoc

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Document mirrors the graph.json top-level shape. Consumers model the whole
// contract here and ignore the fields they do not need.
type Document struct {
	Version string `json:"version"`
	Nodes   []Node `json:"nodes"`
	Edges   []Edge `json:"edges"`
}

// Node is one logical resource type, keyed by TypeName. Common is present when the
// type declares a shared representation; Reporters holds one facet per reporter.
type Node struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	TypeName  string           `json:"typeName"`
	Common    *Members         `json:"common"`
	Reporters map[string]Facet `json:"reporters"`
}

// Members carries the JSON-Schema-facing data fields and the permission rewrite
// trees of a facet. The bodies are kept as raw JSON: consumers that render them
// (the web detail panel) interpret them; the rest pass them through untouched.
type Members struct {
	DataFields  []json.RawMessage `json:"dataFields"`
	Permissions []json.RawMessage `json:"permissions"`
}

// Facet is one reporter's representation of a type. Extends is set only when the
// facet inherits another type/reporter.
type Facet struct {
	DataFields  []json.RawMessage `json:"dataFields"`
	Permissions []json.RawMessage `json:"permissions"`
	Extends     *TypeRef          `json:"extends"`
}

// TypeRef names a type/reporter facet (an inheritance target).
type TypeRef struct {
	TypeName string `json:"typeName"`
	Reporter string `json:"reporter"`
}

// Edge is one relation or inheritance edge in the denormalized topology.
type Edge struct {
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	Name           string `json:"name"`
	Cardinality    string `json:"cardinality"`
	Scope          string `json:"scope"`
	SourceReporter string `json:"sourceReporter"`
	TargetReporter string `json:"targetReporter"`
	Self           bool   `json:"self"`
}

// Parse decodes a graph.json document.
func Parse(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("parsing graph json: %w", err)
	}
	return doc, nil
}

// HasCommon reports, per type name, whether the type has a shared "common"
// representation: it declares a common block or carries any common-scoped
// relation. Every consumer groups facets under a common node on this rule.
func HasCommon(doc Document) map[string]bool {
	has := make(map[string]bool)
	for _, n := range doc.Nodes {
		if n.Common != nil {
			has[n.TypeName] = true
		}
	}
	for _, e := range doc.Edges {
		if e.Kind == "relation" && e.Scope == "common" {
			has[e.Source] = true
		}
	}
	return has
}

// FacetID is the element id for a type's reporter facet (e.g. "workspace__rbac").
func FacetID(typeName, reporter string) string {
	return typeName + "__" + reporter
}

// CommonID is the element id for a type's common node (e.g. "workspace__common").
func CommonID(typeName string) string {
	return typeName + "__common"
}

// SortedKeys returns a map's keys in sorted order, for deterministic output.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cardinalityInfo maps a Kessel cardinality to its UML multiplicity symbol and a
// human description (for legends). rank fixes a stable, readable legend order.
// isDefault marks the cardinality assumed when none is shown (ExactlyOne): it is
// omitted from relation labels (and legends) to reduce noise, since it is the
// common case.
type cardinalityInfo struct {
	symbol    string
	desc      string
	rank      int
	isDefault bool
}

var cardinalities = map[string]cardinalityInfo{
	"ExactlyOne": {"1", "exactly one", 0, true},
	"AtMostOne":  {"0..1", "at most one", 1, false},
	"AtLeastOne": {"1..*", "at least one", 2, false},
	"Many":       {"*", "many", 3, false},
	"All":        {"All", "wildcard (all instances)", 4, false},
}

// Multiplicity returns the UML symbol for a cardinality, falling back to the raw
// Kessel name when there is no specific mapping.
func Multiplicity(cardinality string) string {
	if info, ok := cardinalities[cardinality]; ok {
		return info.symbol
	}
	return cardinality
}

// IsDefaultCardinality reports whether a cardinality is the implicit default,
// which is left off relation labels rather than shown explicitly.
func IsDefaultCardinality(cardinality string) bool {
	info, ok := cardinalities[cardinality]
	return ok && info.isDefault
}

// CardinalityDesc returns the human description for a cardinality (for legends),
// falling back to the raw name.
func CardinalityDesc(cardinality string) string {
	if info, ok := cardinalities[cardinality]; ok {
		return info.desc
	}
	return cardinality
}

// LegendRank orders known cardinalities as declared; unknown ones sort last.
func LegendRank(cardinality string) int {
	if info, ok := cardinalities[cardinality]; ok {
		return info.rank
	}
	return len(cardinalities)
}
