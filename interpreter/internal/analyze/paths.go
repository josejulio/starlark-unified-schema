package analyze

import (
	"sort"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// This file implements path enumeration for reachability analysis. It walks a
// CheckNode proof tree (produced by ExplainCheck) and expands it into the
// alternative grant paths that can satisfy a permission, each terminating at a
// reachable subject type. See REACHABILITY_PLAN.md Phase 1.

// Hop is one relation traversal along a grant path.
type Hop struct {
	FromType, FromReporter string
	Relation, Cardinality  string
	ToType, ToReporter     string
	Fanout                 bool // many-cardinality relation (fan-out site)
	Recursive              bool // this hop re-enters a permission (cycle marker)
}

// Path is one alternative way object#relation can be granted, ending at a
// reachable subject type. AND requirements and UNLESS exclusions are recorded as
// annotations (nested Paths) rather than multiplied out, so the number of paths
// stays proportional to the OR-branches, not combinatorial.
type Path struct {
	Hops            []Hop
	SubjectType     string // terminal reachable subject type ("" if dead-ends unresolved)
	SubjectReporter string
	Conjuncts       []Path // AND: requirements that must ALSO hold
	Exclusions      []Path // UNLESS: conditions that would deny
	Cost            Cost
	Recursive       bool
}

// EnumeratePaths expands a proof tree into alternative grant paths:
//   or(A,B)     -> the paths of A plus the paths of B
//   and(A,B)    -> paths of A, each carrying B's paths under Conjuncts
//   unless(A,B) -> paths of A, each carrying B's paths under Exclusions
//   arrow       -> prepend a Hop, recurse into the target sub-evaluation (Children[0])
//   relation    -> a terminal Hop; SubjectType = node.TargetType
//   recursive   -> a terminal Hop flagged Recursive (cycle, do not expand further)
//   unresolved  -> a terminal Path with SubjectType == "" (surfaces broken schema)
func EnumeratePaths(root *CheckNode) []Path {
	return walkNode(root, []Hop{})
}

// walkNode recursively builds paths from the proof tree node, prepending hops as
// we descend arrows.
func walkNode(n *CheckNode, hops []Hop) []Path {
	switch n.Kind {
	case "permission":
		// A permission expands to its body; the name is cosmetic, not structural.
		return walkNode(n.Body, hops)

	case "relation":
		// Terminal leaf: a direct relation check. Create a hop to the target.
		hop := Hop{
			FromType:     n.TypeName,
			FromReporter: n.Reporter,
			Relation:     n.Name,
			Cardinality:  n.Cardinality,
			ToType:       n.TargetType,
			ToReporter:   n.TargetReporter,
			Fanout:       isManyCardinality(n.Cardinality),
			Recursive:    false,
		}
		allHops := append(append([]Hop{}, hops...), hop)
		return []Path{{
			Hops:            allHops,
			SubjectType:     n.TargetType,
			SubjectReporter: n.TargetReporter,
			Cost:            n.Cost,
			Recursive:       n.Cost.Recursive,
		}}

	case "recursive":
		// A recursive node is a cycle marker: the path stops here.
		hop := Hop{
			FromType:     n.TypeName,
			FromReporter: n.Reporter,
			Relation:     n.Name,
			Cardinality:  "",
			ToType:       n.TypeName,
			ToReporter:   n.Reporter,
			Fanout:       false,
			Recursive:    true,
		}
		allHops := append(append([]Hop{}, hops...), hop)
		return []Path{{
			Hops:            allHops,
			SubjectType:     n.TypeName,
			SubjectReporter: n.Reporter,
			Cost:            n.Cost,
			Recursive:       true,
		}}

	case "arrow":
		// A subreference: traverse the relation, then evaluate the sub on the target.
		// The relation itself becomes a hop; the child's paths are extended with it.
		// If the child is a recursive node, mark this arrow's hop as recursive.
		isRecursive := len(n.Children) > 0 && n.Children[0].Kind == "recursive"
		hop := Hop{
			FromType:     n.TypeName,
			FromReporter: n.Reporter,
			Relation:     n.Name,
			Cardinality:  n.Cardinality,
			ToType:       n.TargetType,
			ToReporter:   n.TargetReporter,
			Fanout:       isManyCardinality(n.Cardinality),
			Recursive:    isRecursive,
		}
		newHops := append(append([]Hop{}, hops...), hop)
		if len(n.Children) == 0 {
			// Broken arrow (unresolved target); return a dead-end path.
			return []Path{{
				Hops:            newHops,
				SubjectType:     "",
				SubjectReporter: "",
				Cost:            n.Cost,
				Recursive:       n.Cost.Recursive,
			}}
		}
		return walkNode(n.Children[0], newHops)

	case "op":
		left := walkNode(n.Children[0], hops)
		right := walkNode(n.Children[1], hops)

		switch n.Op {
		case "or":
			// OR: union of both operands' paths.
			return append(append([]Path{}, left...), right...)

		case "and":
			// AND: left's paths are the spine; right's paths become conjuncts.
			// Set the cost to the node's combined cost.
			for i := range left {
				left[i].Conjuncts = append(left[i].Conjuncts, right...)
				left[i].Cost = n.Cost
				if n.Cost.Recursive {
					left[i].Recursive = true
				}
			}
			return left

		case "unless":
			// UNLESS: left's paths are the spine; right's paths become exclusions.
			for i := range left {
				left[i].Exclusions = append(left[i].Exclusions, right...)
				left[i].Cost = n.Cost
				if n.Cost.Recursive {
					left[i].Recursive = true
				}
			}
			return left

		default:
			// Unknown operator; treat as unresolved.
			return []Path{{
				Hops:            hops,
				SubjectType:     "",
				SubjectReporter: "",
				Cost:            n.Cost,
				Recursive:       n.Cost.Recursive,
			}}
		}

	case "unresolved":
		// An unresolved name: dead-end path with no subject type.
		return []Path{{
			Hops:            hops,
			SubjectType:     "",
			SubjectReporter: "",
			Cost:            n.Cost,
			Recursive:       n.Cost.Recursive,
		}}

	default:
		// Unknown node kind; treat as unresolved.
		return []Path{{
			Hops:            hops,
			SubjectType:     "",
			SubjectReporter: "",
			Cost:            n.Cost,
			Recursive:       n.Cost.Recursive,
		}}
	}
}

// ReachReport is the result of a reachability analysis.
type ReachReport struct {
	Object        FacetRef `json:"object"`
	Relation      string   `json:"relation"`
	SubjectFilter string   `json:"subjectFilter,omitempty"` // "" = enumerate all reachable types
	Reachable     bool     `json:"reachable"`               // meaningful only when SubjectFilter != ""
	ReachableTypes []string `json:"reachableTypes"`         // sorted, deduped terminal subject types
	Paths         []Path   `json:"paths"`                   // all granting paths (filtered to SubjectFilter if set)
	Cheapest      *Path    `json:"cheapest,omitempty"`      // by the static-scalar ordering
	Worst         *Path    `json:"worst,omitempty"`         // by the static-scalar ordering
	Proof         *CheckNode `json:"proof"`                 // underlying ExplainCheck tree, for the tree view
}

// Reach runs a reachability analysis: given object#relation, enumerate the paths
// by which the permission can be granted and the subject types each path reaches.
// If subjectType is non-empty, filter paths to only those reaching that type.
func Reach(doc graphdoc.Document, object FacetRef, relation, subjectType string) (*ReachReport, error) {
	root, err := ExplainCheck(doc, object, relation)
	if err != nil {
		return nil, err
	}

	allPaths := EnumeratePaths(root)

	// Collect reachable subject types.
	typeSet := map[string]bool{}
	for _, p := range allPaths {
		if p.SubjectType != "" {
			typeSet[p.SubjectType] = true
		}
	}
	reachableTypes := make([]string, 0, len(typeSet))
	for t := range typeSet {
		reachableTypes = append(reachableTypes, t)
	}
	sort.Strings(reachableTypes)

	// Filter paths if a subject type is specified.
	filteredPaths := allPaths
	if subjectType != "" {
		filteredPaths = make([]Path, 0, len(allPaths))
		for _, p := range allPaths {
			if p.SubjectType == subjectType {
				filteredPaths = append(filteredPaths, p)
			}
		}
	}

	// Find cheapest and worst paths by static scalars.
	var cheapest, worst *Path
	if len(filteredPaths) > 0 {
		sorted := make([]Path, len(filteredPaths))
		copy(sorted, filteredPaths)
		sort.Slice(sorted, func(i, j int) bool {
			return compareCost(sorted[i].Cost, sorted[j].Cost) < 0
		})
		cheapest = &sorted[0]
		worst = &sorted[len(sorted)-1]
	}

	return &ReachReport{
		Object:         object,
		Relation:       relation,
		SubjectFilter:  subjectType,
		Reachable:      len(filteredPaths) > 0,
		ReachableTypes: reachableTypes,
		Paths:          filteredPaths,
		Cheapest:       cheapest,
		Worst:          worst,
		Proof:          root,
	}, nil
}

// compareCost compares two costs by the static scalar tuple
// (FanoutSites, Recursive, DispatchDepth), ascending. Returns -1 if a < b, 0 if
// equal, 1 if a > b.
func compareCost(a, b Cost) int {
	if a.FanoutSites < b.FanoutSites {
		return -1
	}
	if a.FanoutSites > b.FanoutSites {
		return 1
	}
	// FanoutSites equal; compare Recursive (false < true).
	if !a.Recursive && b.Recursive {
		return -1
	}
	if a.Recursive && !b.Recursive {
		return 1
	}
	// Both equal; compare DispatchDepth.
	if a.DispatchDepth < b.DispatchDepth {
		return -1
	}
	if a.DispatchDepth > b.DispatchDepth {
		return 1
	}
	return 0
}
