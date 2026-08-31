package analyze

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// This file holds the formatters for ReachVerdict (text and JSON), mirroring
// check_report.go.

// FormatReachText renders a ReachVerdict as a human-readable text report.
func FormatReachText(v *ReachVerdict) string {
	var sb strings.Builder

	// Headline verdict
	switch v.Verdict {
	case "reachable":
		sb.WriteString("✓ reachable\n")
	case "exclusion-only":
		sb.WriteString("⚠ reachable only through an exclusion (unless) branch\n")
	case "unreachable":
		sb.WriteString("✗ no path — schema does not support this check\n")
	}

	if len(v.Paths) == 0 {
		return sb.String()
	}

	// List witness paths
	sb.WriteString("\n")
	grantPaths := 0
	exclusionPaths := 0
	for _, p := range v.Paths {
		if p.Excluded {
			exclusionPaths++
		} else {
			grantPaths++
		}
	}

	if grantPaths > 0 {
		sb.WriteString(fmt.Sprintf("Grant path(s): %d\n", grantPaths))
		for i, p := range v.Paths {
			if !p.Excluded {
				sb.WriteString(formatPath(i+1, p))
			}
		}
	}

	if exclusionPaths > 0 {
		if grantPaths > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Exclusion path(s): %d\n", exclusionPaths))
		for i, p := range v.Paths {
			if p.Excluded {
				sb.WriteString(formatPath(i+1, p))
			}
		}
	}

	return sb.String()
}

// formatPath renders a single witness path as an indented hop chain.
func formatPath(num int, p WitnessPath) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Path %d:\n", num))

	if len(p.Hops) == 0 {
		sb.WriteString("    (empty path)\n")
		return sb.String()
	}

	// Start with the first hop's source
	firstHop := p.Hops[0]
	sb.WriteString(fmt.Sprintf("    %s/%s\n", firstHop.FromReporter, firstHop.FromType))

	// Chain each hop
	for _, h := range p.Hops {
		mult := graphdoc.Multiplicity(h.Cardinality)
		sb.WriteString(fmt.Sprintf("      --%s %s--> %s/%s\n", h.Relation, mult, h.ToReporter, h.ToType))
	}

	return sb.String()
}

// FormatReachJSON renders a ReachVerdict as indented JSON.
func FormatReachJSON(v *ReachVerdict) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}
