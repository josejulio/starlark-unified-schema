package analyze

import (
	"fmt"
	"sort"
	"strings"
)

// This file holds the symbolic cost algebra used by the check-cost analyzer
// (check.go). A check's read cost is data-dependent — it scales with how many
// tuples sit on each traversed relation and how deep a hierarchy runs — so a
// single scalar would be dishonest. Cost is instead a symbolic big-O in named
// variables (see costExpr), paired with two fully-static scalars computed on the
// proof tree: dispatch depth (sequential arrow hops → latency) and fan-out sites
// (arrows over many-cardinality relations → work). See GRAPH.md "Check cost".

// costExpr is a sum of product-terms representing a worst-case big-O. Each term
// is a product of symbolic factors (an empty term is the constant 1). The whole
// expression is the additive combination of its terms, e.g.
// {{}, {"D_workspace"}} simplifies to O(D_workspace).
type costExpr []costTerm

// costTerm is a product of factors, kept sorted for a stable key. Repeated
// factors are allowed (they render as f^k), so nested many-arrows compound.
type costTerm []string

// constExpr is the cost of a constant-time step (a direct relation check).
func constExpr() costExpr { return costExpr{costTerm{}} }

// varExpr is the cost of a single symbolic factor (e.g. a hierarchy depth).
func varExpr(factor string) costExpr { return costExpr{costTerm{factor}} }

func termKey(t costTerm) string { return strings.Join(t, "\x00") }

// sumExpr adds two costs (the worst case of an or/and/unless: both operands may
// be evaluated). Duplicate terms collapse; a constant term is dropped once any
// variable term is present, since it is dominated.
func sumExpr(a, b costExpr) costExpr {
	seen := map[string]bool{}
	out := costExpr{}
	for _, src := range []costExpr{a, b} {
		for _, t := range src {
			k := termKey(t)
			if !seen[k] {
				seen[k] = true
				out = append(out, t)
			}
		}
	}
	return simplify(out)
}

// mulFactor multiplies every term of an expression by one factor (a many-arrow
// fanning its downstream cost out across N targets).
func mulFactor(e costExpr, factor string) costExpr {
	seen := map[string]bool{}
	out := costExpr{}
	for _, t := range e {
		nt := append(append(costTerm{}, t...), factor)
		sort.Strings(nt)
		k := termKey(nt)
		if !seen[k] {
			seen[k] = true
			out = append(out, nt)
		}
	}
	if len(out) == 0 {
		return varExpr(factor)
	}
	return out
}

// simplify drops constant terms when a variable term dominates, and guarantees a
// non-empty expression (a bare constant when nothing else is present).
func simplify(e costExpr) costExpr {
	hasVar := false
	for _, t := range e {
		if len(t) > 0 {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return constExpr()
	}
	out := costExpr{}
	for _, t := range e {
		if len(t) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// bigO renders the expression as an O(...) string with terms sorted for stable
// output. Repeated factors in a term are shown as f^k.
func (e costExpr) bigO() string {
	e = simplify(e)
	parts := make([]string, 0, len(e))
	for _, t := range e {
		if len(t) == 0 {
			continue
		}
		parts = append(parts, renderTerm(t))
	}
	if len(parts) == 0 {
		return "O(1)"
	}
	sort.Strings(parts)
	return "O(" + strings.Join(parts, " + ") + ")"
}

func renderTerm(t costTerm) string {
	counts := map[string]int{}
	order := make([]string, 0, len(t))
	for _, f := range t {
		if counts[f] == 0 {
			order = append(order, f)
		}
		counts[f]++
	}
	sort.Strings(order)
	parts := make([]string, 0, len(order))
	for _, f := range order {
		if counts[f] > 1 {
			parts = append(parts, fmt.Sprintf("%s^%d", f, counts[f]))
		} else {
			parts = append(parts, f)
		}
	}
	return strings.Join(parts, "·")
}

// isManyCardinality reports whether traversing a relation of this cardinality
// fans out — a subreference across it dispatches one sub-check per target tuple.
// ExactlyOne / AtMostOne are single-target; everything else is treated as fan-out.
func isManyCardinality(cardinality string) bool {
	switch cardinality {
	case "ExactlyOne", "AtMostOne":
		return false
	default: // Many, AtLeastOne, All, and any unknown wide cardinality
		return true
	}
}

// mergeVars unions two CostVar lists, deduping by name and keeping stable order.
func mergeVars(a, b []CostVar) []CostVar {
	seen := map[string]bool{}
	out := make([]CostVar, 0, len(a)+len(b))
	for _, src := range [][]CostVar{a, b} {
		for _, v := range src {
			if !seen[v.Name] {
				seen[v.Name] = true
				out = append(out, v)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
