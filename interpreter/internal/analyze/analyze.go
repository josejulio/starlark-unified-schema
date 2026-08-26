// Package analyze runs graph-theory analyses over the canonical graph.json
// artifact (see GRAPH.md) to surface structural problems — starting with
// "islands": resources that are disconnected from the rest of the schema. Like
// the render package it depends only on the documented JSON contract, not on the
// Go types that produce it, so a separate consumer could read the same file.
//
// The graph engine is gonum, so richer algorithms (strongly-connected
// components, cycle detection, shortest paths) can be layered on later without
// changing this package's contract.
package analyze

import (
	"sort"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// Component is one weakly-connected group of resource types, its members sorted.
type Component struct {
	Members []string `json:"members"`
}

// Size is the number of resource types in the component.
func (c Component) Size() int { return len(c.Members) }

// HasFindings reports whether the analysis found any structural problem (any
// island). Callers use it to exit non-zero for CI gating while still reporting
// every finding.
func (r IslandReport) HasFindings() bool { return len(r.Islands) > 0 }

// IslandReport describes the connectivity of the schema graph. Connectivity is
// computed over relation edges only (inheritance is ignored) with self-relations
// excluded, treating edges as undirected: two types are in the same component if
// a chain of relations links them regardless of direction.
//
// The largest connected component is the reference body of the graph; every
// other component is an island. Singletons (isolated resources with no cross-type
// relation) are also listed on their own for convenience.
type IslandReport struct {
	NodeCount        int         `json:"nodeCount"`
	RelationEdge     int         `json:"relationEdgeCount"` // cross-type relation edges considered
	LargestComponent Component   `json:"largestComponent"`
	Islands          []Component `json:"islands"`
	Isolated         []string    `json:"isolated"` // island members that are alone (size-1 components)
}

// Islands computes the weakly-connected components of the schema graph and
// classifies them into the mainland and islands (see IslandReport).
func Islands(doc graphdoc.Document) IslandReport {
	g := simple.NewUndirectedGraph()

	ids := make(map[string]int64, len(doc.Nodes))
	names := make(map[int64]string, len(doc.Nodes))
	idFor := func(name string) int64 {
		if id, ok := ids[name]; ok {
			return id
		}
		id := int64(len(ids))
		ids[name] = id
		names[id] = name
		g.AddNode(simple.Node(id))
		return id
	}

	for _, n := range doc.Nodes {
		idFor(n.TypeName)
	}

	edgeCount := 0
	for _, e := range doc.Edges {
		if e.Kind != "relation" {
			continue // inheritance does not count toward relational connectivity
		}
		if e.Source == e.Target {
			continue // self-relation: connects a type to itself, not to others
		}
		s, t := idFor(e.Source), idFor(e.Target)
		g.SetEdge(simple.Edge{F: simple.Node(s), T: simple.Node(t)})
		edgeCount++
	}

	components := topo.ConnectedComponents(g)

	comps := make([]Component, 0, len(components))
	for _, nodes := range components {
		members := make([]string, 0, len(nodes))
		for _, n := range nodes {
			members = append(members, names[n.ID()])
		}
		sort.Strings(members)
		comps = append(comps, Component{Members: members})
	}
	// Largest first; ties broken by first member for deterministic output.
	sort.Slice(comps, func(i, j int) bool {
		if comps[i].Size() != comps[j].Size() {
			return comps[i].Size() > comps[j].Size()
		}
		return comps[i].Members[0] < comps[j].Members[0]
	})

	report := IslandReport{
		NodeCount:    len(ids),
		RelationEdge: edgeCount,
		Islands:      []Component{},
		Isolated:     []string{},
	}
	if len(comps) > 0 {
		report.LargestComponent = comps[0]
		report.Islands = comps[1:]
		for _, c := range report.Islands {
			if c.Size() == 1 {
				report.Isolated = append(report.Isolated, c.Members[0])
			}
		}
	}
	return report
}
