package analyze

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// FormatReachText renders a reachability report as human-readable text.
func FormatReachText(r *ReachReport) string {
	var b strings.Builder

	title := fmt.Sprintf("Reachability: %s#%s", r.Object, r.Relation)
	if r.SubjectFilter != "" {
		title += fmt.Sprintf(" @%s", r.SubjectFilter)
	}
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("=", len(title)) + "\n\n")

	// Headline: is it reachable?
	if r.SubjectFilter != "" {
		if r.Reachable {
			fmt.Fprintf(&b, "✓ Reachable from %s\n\n", r.SubjectFilter)
		} else {
			fmt.Fprintf(&b, "✗ Not reachable from %s\n", r.SubjectFilter)
			fmt.Fprintf(&b, "  Schema does not provide a path from %s to %s#%s.\n\n",
				r.SubjectFilter, r.Object, r.Relation)
		}
	}

	// List of all reachable subject types.
	if len(r.ReachableTypes) > 0 {
		fmt.Fprintf(&b, "Reachable subject types (%d):\n", len(r.ReachableTypes))
		for _, t := range r.ReachableTypes {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("No reachable subject types (schema dead-end).\n\n")
	}

	// Path enumeration.
	if len(r.Paths) == 0 {
		b.WriteString("No paths found.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "Paths (%d):\n\n", len(r.Paths))
	for i, p := range r.Paths {
		marker := ""
		if r.Cheapest != nil && pathsEqual(p, *r.Cheapest) {
			marker = " [CHEAPEST]"
		}
		fmt.Fprintf(&b, "%d. %s%s\n", i+1, formatPathOneLine(p), marker)
		writePathDetail(&b, p, "   ")
	}

	// Cheapest vs worst summary.
	if r.Cheapest != nil && r.Worst != nil && !pathsEqual(*r.Cheapest, *r.Worst) {
		b.WriteString("\nCost range:\n")
		fmt.Fprintf(&b, "  Cheapest: %s (depth %d, fan-out sites %d, recursive %t)\n",
			r.Cheapest.Cost.BigO, r.Cheapest.Cost.DispatchDepth, r.Cheapest.Cost.FanoutSites, r.Cheapest.Cost.Recursive)
		fmt.Fprintf(&b, "  Worst:    %s (depth %d, fan-out sites %d, recursive %t)\n",
			r.Worst.Cost.BigO, r.Worst.Cost.DispatchDepth, r.Worst.Cost.FanoutSites, r.Worst.Cost.Recursive)
	}

	return b.String()
}

// formatPathOneLine renders a path as a one-line hop chain with its terminal
// subject type and cost.
func formatPathOneLine(p Path) string {
	if len(p.Hops) == 0 {
		if p.SubjectType == "" {
			return fmt.Sprintf("(dead-end) → %s", p.Cost.BigO)
		}
		return fmt.Sprintf("→ %s  %s", p.SubjectType, p.Cost.BigO)
	}

	parts := make([]string, len(p.Hops))
	for i, h := range p.Hops {
		if h.Recursive {
			parts[i] = fmt.Sprintf("%s ↺", h.Relation)
		} else {
			parts[i] = h.Relation
		}
	}
	chain := strings.Join(parts, " → ")
	if p.SubjectType == "" {
		return fmt.Sprintf("%s → (unresolved)  %s", chain, p.Cost.BigO)
	}
	return fmt.Sprintf("%s → %s  %s", chain, p.SubjectType, p.Cost.BigO)
}

// writePathDetail writes the detailed breakdown of a path (hops, conjuncts,
// exclusions) under an indentation prefix.
func writePathDetail(b *strings.Builder, p Path, prefix string) {
	// Hops.
	for _, h := range p.Hops {
		fromLabel := h.FromReporter + "/" + h.FromType
		toLabel := h.ToReporter + "/" + h.ToType
		label := fmt.Sprintf("%s --%s (%s)--> %s",
			fromLabel, h.Relation, graphdoc.Multiplicity(h.Cardinality), toLabel)
		if h.Recursive {
			label += " ↺"
		}
		if h.Fanout {
			label += " [FAN-OUT]"
		}
		fmt.Fprintf(b, "%s%s\n", prefix, label)
	}

	// Conjuncts (AND requirements).
	if len(p.Conjuncts) > 0 {
		fmt.Fprintf(b, "%sAND (%d requirement(s)):\n", prefix, len(p.Conjuncts))
		for i, c := range p.Conjuncts {
			fmt.Fprintf(b, "%s  %d. %s\n", prefix, i+1, formatPathOneLine(c))
		}
	}

	// Exclusions (UNLESS conditions).
	if len(p.Exclusions) > 0 {
		fmt.Fprintf(b, "%sUNLESS (%d exclusion(s)):\n", prefix, len(p.Exclusions))
		for i, e := range p.Exclusions {
			fmt.Fprintf(b, "%s  %d. %s\n", prefix, i+1, formatPathOneLine(e))
		}
	}

	fmt.Fprintf(b, "%sCost: %s (depth %d, fan-out sites %d, recursive %t)\n",
		prefix, p.Cost.BigO, p.Cost.DispatchDepth, p.Cost.FanoutSites, p.Cost.Recursive)
	b.WriteString("\n")
}

// pathsEqual is a shallow equality check for marking the cheapest path.
func pathsEqual(a, b Path) bool {
	if len(a.Hops) != len(b.Hops) {
		return false
	}
	for i := range a.Hops {
		if a.Hops[i] != b.Hops[i] {
			return false
		}
	}
	return a.SubjectType == b.SubjectType && a.SubjectReporter == b.SubjectReporter
}

// FormatReachJSON renders a reachability report as JSON.
func FormatReachJSON(r *ReachReport) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}
