// Package render turns the canonical graph.json artifact (see GRAPH.md) into
// human-facing diagrams. It depends only on the documented JSON contract, not on
// the Go types that produce it — a separate JS/Python renderer would consume the
// same file the same way.
package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// multLabel is the parenthesized multiplicity as shown on edges and the legend.
func multLabel(cardinality string) string {
	return "(" + graphdoc.Multiplicity(cardinality) + ")"
}

// Mermaid renders a structural diagram grouped like the Starlark schema:
// one subgraph per resource type, containing a "common" node (when the resource
// has a shared representation) and one node per reporter. A purple dotted edge
// links the common node to each reporter representation; relation edges are
// labeled by name and UML multiplicity; inheritance is a dotted "extends" edge.
// direction is a
// Mermaid flowchart direction such as "LR" or "TD" (defaults to "LR"). When
// legend is true, a legend of the multiplicity symbols in use is appended.
func Mermaid(doc graphdoc.Document, direction string, legend bool) string {
	if direction == "" {
		direction = "LR"
	}

	hasCommon := graphdoc.HasCommon(doc)

	var b strings.Builder
	fmt.Fprintf(&b, "flowchart %s\n", direction)

	nodes := append([]graphdoc.Node(nil), doc.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].TypeName < nodes[j].TypeName })
	for _, n := range nodes {
		fmt.Fprintf(&b, "  subgraph %s\n", sanitize(n.TypeName))
		if hasCommon[n.TypeName] {
			fmt.Fprintf(&b, "    %s[%q]\n", commonID(n.TypeName), "common")
		}
		for _, reporter := range graphdoc.SortedKeys(n.Reporters) {
			fmt.Fprintf(&b, "    %s[%q]\n", facetID(n.TypeName, reporter), reporter)
		}
		b.WriteString("  end\n")
	}

	commonEdges := map[string]struct{}{}
	relationEdges := map[string]struct{}{}
	extendsEdges := map[string]struct{}{}
	usedCards := map[string]struct{}{}

	// The common representation is shared into each reporter of the type.
	for _, n := range nodes {
		if !hasCommon[n.TypeName] {
			continue
		}
		for _, reporter := range graphdoc.SortedKeys(n.Reporters) {
			commonEdges[fmt.Sprintf("  %s -.-> %s", commonID(n.TypeName), facetID(n.TypeName, reporter))] = struct{}{}
		}
	}

	for _, e := range doc.Edges {
		switch e.Kind {
		case "relation":
			label := e.Name
			if e.Cardinality != "" && !graphdoc.IsDefaultCardinality(e.Cardinality) {
				label = fmt.Sprintf("%s %s", e.Name, multLabel(e.Cardinality))
				usedCards[e.Cardinality] = struct{}{}
			}
			target := facetID(e.Target, e.TargetReporter)
			src := facetID(e.Source, e.SourceReporter)
			if e.Scope == "common" {
				src = commonID(e.Source)
			}
			relationEdges[fmt.Sprintf("  %s -->|%q| %s", src, label, target)] = struct{}{}
		case "inherits":
			src := facetID(e.Source, e.SourceReporter)
			target := facetID(e.Target, e.TargetReporter)
			extendsEdges[fmt.Sprintf("  %s -.->|%q| %s", src, "extends", target)] = struct{}{}
		}
	}

	writeBlock(&b, "shared common representation", commonEdges)
	writeBlock(&b, "relations", relationEdges)
	writeBlock(&b, "inheritance", extendsEdges)

	// Style the shared-common links (emitted first, so they own link indices
	// 0..N-1) as a purple dotted line at the same weight as the "extends" edge,
	// distinct from the solid gray relation edges.
	if n := len(commonEdges); n > 0 {
		idxs := make([]string, n)
		for i := range idxs {
			idxs[i] = strconv.Itoa(i)
		}
		fmt.Fprintf(&b, "\n  linkStyle %s stroke:#8a5cf6,stroke-width:2px;\n", strings.Join(idxs, ","))
	}

	if legend && len(usedCards) > 0 {
		cards := make([]string, 0, len(usedCards))
		for c := range usedCards {
			cards = append(cards, c)
		}
		sort.Slice(cards, func(i, j int) bool { return graphdoc.LegendRank(cards[i]) < graphdoc.LegendRank(cards[j]) })

		b.WriteString("\n  subgraph Legend\n    direction LR\n")
		for i, c := range cards {
			fmt.Fprintf(&b, "    legend%d[%q]\n", i, multLabel(c)+" = "+graphdoc.CardinalityDesc(c))
		}
		b.WriteString("  end\n")
	}

	return b.String()
}

// writeBlock emits a comment-titled, sorted block of edge lines (nothing if empty).
func writeBlock(b *strings.Builder, title string, set map[string]struct{}) {
	if len(set) == 0 {
		return
	}
	fmt.Fprintf(b, "\n  %%%% %s\n", title)
	for _, line := range sortedSet(set) {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// facetID / commonID wrap the shared element ids in sanitize so they are valid
// Mermaid node identifiers.
func facetID(typeName, reporter string) string {
	return sanitize(graphdoc.FacetID(typeName, reporter))
}

func commonID(typeName string) string {
	return sanitize(graphdoc.CommonID(typeName))
}

// sanitize maps a name to a Mermaid-safe identifier.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func sortedSet(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
