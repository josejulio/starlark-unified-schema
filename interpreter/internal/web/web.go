// Package web turns the canonical graph.json artifact (see GRAPH.md) into a
// single self-contained, interactive HTML page. Like the render and analyze
// packages it depends only on the documented JSON contract, not on the Go types
// that produce it — a separate consumer could read the same file the same way.
//
// The page renders the graph with Cytoscape.js: one compound node per resource
// type, a child node for the shared "common" representation (when present) and
// one child per reporter facet, with relation and inheritance edges between
// them, plus a "shared" edge from each common node to its reporters. The
// grouping mirrors the Mermaid renderer (internal/render): a type has a "common"
// node when it declares a common block or carries a common-scoped relation;
// relation edges originate from the common node when common-scoped.
package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// element is one Cytoscape.js graph element (a node or an edge). data carries the
// element's raw properties, surfaced verbatim in the detail panel; classes drive
// styling by kind.
type element struct {
	Data    map[string]any `json:"data"`
	Classes string         `json:"classes,omitempty"`
}

// BuildElements transforms a graph document into the deterministic, sorted list
// of Cytoscape.js elements: a compound parent per resource type, a "common"
// child (when the type has a shared representation) and one child per reporter,
// followed by relation and inheritance edges. Output order is stable so the
// generated HTML diffs cleanly and is golden-testable.
func BuildElements(doc graphdoc.Document) []element {
	hasCommon := graphdoc.HasCommon(doc)

	nodes := append([]graphdoc.Node(nil), doc.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].TypeName < nodes[j].TypeName })

	elements := make([]element, 0)
	for _, n := range nodes {
		reporters := graphdoc.SortedKeys(n.Reporters)

		// Compound parent for the logical resource type.
		elements = append(elements, element{
			Classes: "resource",
			Data: map[string]any{
				"id":        n.TypeName,
				"label":     n.TypeName,
				"group":     "type",
				"kind":      n.Kind,
				"typeName":  n.TypeName,
				"reporters": reporters,
				"hasCommon": hasCommon[n.TypeName],
			},
		})

		if hasCommon[n.TypeName] {
			data := map[string]any{
				"id":       graphdoc.CommonID(n.TypeName),
				"parent":   n.TypeName,
				"label":    "common",
				"group":    "common",
				"typeName": n.TypeName,
			}
			if n.Common != nil {
				addMembers(data, n.Common.DataFields, n.Common.Permissions)
			}
			elements = append(elements, element{Classes: "common", Data: data})
		}

		for _, reporter := range reporters {
			facet := n.Reporters[reporter]
			data := map[string]any{
				"id":       graphdoc.FacetID(n.TypeName, reporter),
				"parent":   n.TypeName,
				"label":    reporter,
				"group":    "reporter",
				"typeName": n.TypeName,
				"reporter": reporter,
			}
			if ext := facet.Extends; ext != nil {
				data["extends"] = ext.TypeName + " (" + ext.Reporter + ")"
			}
			addMembers(data, facet.DataFields, facet.Permissions)
			elements = append(elements, element{Classes: "reporter", Data: data})
		}
	}

	// Edges are keyed by a synthetic id and de-duplicated, then sorted, so the
	// output is stable regardless of graph.json edge ordering.
	edges := map[string]element{}
	for _, e := range doc.Edges {
		switch e.Kind {
		case "relation":
			src := graphdoc.FacetID(e.Source, e.SourceReporter)
			if e.Scope == "common" {
				src = graphdoc.CommonID(e.Source)
			}
			target := graphdoc.FacetID(e.Target, e.TargetReporter)

			label := e.Name
			if e.Cardinality != "" && !graphdoc.IsDefaultCardinality(e.Cardinality) {
				label = fmt.Sprintf("%s (%s)", e.Name, graphdoc.Multiplicity(e.Cardinality))
			}

			id := fmt.Sprintf("%s->%s:%s", src, target, e.Name)
			classes := "relation"
			if e.Self {
				classes += " self"
			}
			data := map[string]any{
				"id":          id,
				"source":      src,
				"target":      target,
				"label":       label,
				"kind":        "relation",
				"name":        e.Name,
				"cardinality": e.Cardinality,
				"scope":       e.Scope,
				"self":        e.Self,
			}
			if e.SourceReporter != "" {
				data["sourceReporter"] = e.SourceReporter
			}
			if e.TargetReporter != "" {
				data["targetReporter"] = e.TargetReporter
			}
			edges[id] = element{Classes: classes, Data: data}
		case "inherits":
			src := graphdoc.FacetID(e.Source, e.SourceReporter)
			target := graphdoc.FacetID(e.Target, e.TargetReporter)
			id := fmt.Sprintf("%s=>inherits:%s", src, target)
			edges[id] = element{
				Classes: "inherits",
				Data: map[string]any{
					"id":     id,
					"source": src,
					"target": target,
					"label":  "extends",
					"kind":   "inherits",
				},
			}
		}
	}

	// A type's common representation is shared into each of its reporters. Mirror
	// the Mermaid renderer by drawing an explicit edge from the common node to
	// every reporter facet, so the portal shows the same topology rather than
	// implying it through compound nesting alone.
	for _, n := range nodes {
		if !hasCommon[n.TypeName] {
			continue
		}
		src := graphdoc.CommonID(n.TypeName)
		for _, reporter := range graphdoc.SortedKeys(n.Reporters) {
			target := graphdoc.FacetID(n.TypeName, reporter)
			id := fmt.Sprintf("%s==>%s", src, target)
			edges[id] = element{
				Classes: "shared",
				Data: map[string]any{
					"id":     id,
					"source": src,
					"target": target,
					"kind":   "shared",
				},
			}
		}
	}

	for _, id := range graphdoc.SortedKeys(edges) {
		elements = append(elements, edges[id])
	}

	return elements
}

// addMembers attaches a facet's data fields and permission rewrite trees to a
// node's element data, omitting each when empty so the embedded JSON stays clean.
func addMembers(data map[string]any, dataFields, permissions []json.RawMessage) {
	if len(dataFields) > 0 {
		data["dataFields"] = dataFields
	}
	if len(permissions) > 0 {
		data["permissions"] = permissions
	}
}

// marshalElements renders the element list as indented JSON for embedding.
func marshalElements(elements []element) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(elements); err != nil {
		return nil, fmt.Errorf("marshaling elements: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Elements transforms a graph.json document into the Cytoscape element list as
// indented JSON. This is the authoritative graph.json -> elements transform: the
// in-browser WASM compiler (cmd/graph-wasm) returns it directly, and the
// playground renders it.
func Elements(data []byte) ([]byte, error) {
	doc, err := graphdoc.Parse(data)
	if err != nil {
		return nil, err
	}
	return marshalElements(BuildElements(doc))
}
