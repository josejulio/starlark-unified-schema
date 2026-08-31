package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// This file implements CheckReachable: a structural reachability verifier modelled
// on inventory-api's CheckRequest. Given object#relation@subject, it walks the
// permission rewrite (via ExplainCheck) and extracts every witness path that
// terminates at a relation leaf targeting the subject facet. It uses no instance
// data — this is a purely static property of the schema graph.

// WitnessHop is one relation traversal on a path from object to subject.
type WitnessHop struct {
	FromType     string `json:"fromType"`
	FromReporter string `json:"fromReporter"`
	Relation     string `json:"relation"`
	Cardinality  string `json:"cardinality"`
	ToType       string `json:"toType"`
	ToReporter   string `json:"toReporter"`
	Fanout       bool   `json:"fanout"` // many-cardinality relation
}

// WitnessPath is one structural path object#relation ... -> subject.
type WitnessPath struct {
	Hops     []WitnessHop `json:"hops"`
	Excluded bool         `json:"excluded"` // path descends through the right operand of an `unless`
}

// ReachVerdict is the result of a CheckRequest reachability verification.
type ReachVerdict struct {
	Object   FacetRef      `json:"object"`   // TYPE.REPORTER
	Relation string        `json:"relation"`
	Subject  FacetRef      `json:"subject"`  // TYPE.REPORTER (required)
	Verdict  string        `json:"verdict"`  // "reachable" | "exclusion-only" | "unreachable"
	Paths    []WitnessPath `json:"paths"`    // all witnesses (grant + exclusion), stable-sorted
	Proof    *CheckNode    `json:"proof"`    // the underlying ExplainCheck tree (for a tree view)
}

// CheckReachable runs ExplainCheck, then extracts every witness path whose
// terminal relation leaf targets `subject` (matched on BOTH type and reporter).
func CheckReachable(doc graphdoc.Document, object FacetRef, relation string, subject FacetRef) (*ReachVerdict, error) {
	// Validate subject exists in the schema
	r, err := newCheckResolver(doc)
	if err != nil {
		return nil, err
	}
	reps, ok := r.reportersByType[subject.TypeName]
	if !ok {
		return nil, fmt.Errorf("unknown subject type %q", subject.TypeName)
	}
	if !contains(reps, subject.Reporter) {
		return nil, fmt.Errorf("subject type %q has no reporter %q (have: %s)", subject.TypeName, subject.Reporter, strings.Join(reps, ", "))
	}

	// Get the proof tree from ExplainCheck
	proof, err := ExplainCheck(doc, object, relation)
	if err != nil {
		return nil, err
	}

	// Extract witness paths
	paths := extractWitnesses(proof, subject, []WitnessHop{}, false)

	// Deterministic sort: non-excluded first, then by hop count, then by path signature
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Excluded != paths[j].Excluded {
			return !paths[i].Excluded // false < true (non-excluded first)
		}
		if len(paths[i].Hops) != len(paths[j].Hops) {
			return len(paths[i].Hops) < len(paths[j].Hops)
		}
		return pathSignature(paths[i]) < pathSignature(paths[j])
	})

	// Determine verdict
	verdict := "unreachable"
	hasGrant := false
	hasExclusion := false
	for _, p := range paths {
		if p.Excluded {
			hasExclusion = true
		} else {
			hasGrant = true
		}
	}
	if hasGrant {
		verdict = "reachable"
	} else if hasExclusion {
		verdict = "exclusion-only"
	}

	return &ReachVerdict{
		Object:   object,
		Relation: relation,
		Subject:  subject,
		Verdict:  verdict,
		Paths:    paths,
		Proof:    proof,
	}, nil
}

// extractWitnesses performs DFS over the proof tree, carrying accumulated hops
// and a flag indicating whether we are under the right operand of an `unless`.
func extractWitnesses(n *CheckNode, subject FacetRef, hops []WitnessHop, underUnlessRight bool) []WitnessPath {
	if n == nil {
		return nil
	}

	switch n.Kind {
	case "permission":
		// Permission expands to its body
		return extractWitnesses(n.Body, subject, hops, underUnlessRight)

	case "relation":
		// Relation leaf: check if it matches the subject
		if n.TargetType == subject.TypeName && n.TargetReporter == subject.Reporter {
			// This leaf targets the subject — emit a witness path
			hop := WitnessHop{
				FromType:     n.TypeName,
				FromReporter: n.Reporter,
				Relation:     n.Name,
				Cardinality:  n.Cardinality,
				ToType:       n.TargetType,
				ToReporter:   n.TargetReporter,
				Fanout:       isManyCardinality(n.Cardinality),
			}
			allHops := append(append([]WitnessHop{}, hops...), hop)
			return []WitnessPath{{Hops: allHops, Excluded: underUnlessRight}}
		}
		// Does not match subject — dead end
		return nil

	case "op":
		// Operators: recurse into both children
		var paths []WitnessPath
		if n.Op == "unless" {
			// Left operand: keep underUnlessRight unchanged
			paths = append(paths, extractWitnesses(n.Children[0], subject, hops, underUnlessRight)...)
			// Right operand: set underUnlessRight = true
			paths = append(paths, extractWitnesses(n.Children[1], subject, hops, true)...)
		} else {
			// "and" or "or": recurse into both with same underUnlessRight
			for _, child := range n.Children {
				paths = append(paths, extractWitnesses(child, subject, hops, underUnlessRight)...)
			}
		}
		return paths

	case "arrow":
		// Subreference: append a hop, then recurse into the target evaluation
		hop := WitnessHop{
			FromType:     n.TypeName,
			FromReporter: n.Reporter,
			Relation:     n.Name,
			Cardinality:  n.Cardinality,
			ToType:       n.TargetType,
			ToReporter:   n.TargetReporter,
			Fanout:       isManyCardinality(n.Cardinality),
		}
		newHops := append(append([]WitnessHop{}, hops...), hop)
		if len(n.Children) > 0 {
			return extractWitnesses(n.Children[0], subject, newHops, underUnlessRight)
		}
		return nil

	case "recursive":
		// Recursion sentinel: dead-end for witnesses (see soundness note in plan)
		// A recursive node re-enters a permission already expanded higher on the path,
		// introducing no new subject facet.
		return nil

	case "unresolved":
		// Unresolved name: dead-end
		return nil

	default:
		return nil
	}
}

// pathSignature returns a stable string key for sorting paths deterministically.
func pathSignature(p WitnessPath) string {
	parts := make([]string, len(p.Hops))
	for i, h := range p.Hops {
		parts[i] = fmt.Sprintf("%s.%s/%s/%s.%s", h.FromType, h.FromReporter, h.Relation, h.ToType, h.ToReporter)
	}
	return strings.Join(parts, "->")
}
