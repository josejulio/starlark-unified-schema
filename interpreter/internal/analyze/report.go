package analyze

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatText renders an IslandReport as a human-readable report.
func FormatText(r IslandReport) string {
	var b strings.Builder

	b.WriteString("Graph analysis: islands & isolated resources\n")
	b.WriteString("============================================\n\n")
	fmt.Fprintf(&b, "Resources: %d   Cross-type relations: %d\n", r.NodeCount, r.RelationEdge)
	fmt.Fprintf(&b, "Largest component: %d resources   Islands: %d\n\n", r.LargestComponent.Size(), len(r.Islands))

	if len(r.Islands) == 0 {
		b.WriteString("No islands: every resource is connected to the main graph.\n")
		return b.String()
	}

	if len(r.Isolated) > 0 {
		b.WriteString("Isolated resources (no cross-type relation):\n")
		for _, name := range r.Isolated {
			fmt.Fprintf(&b, "  - %s\n", name)
		}
		b.WriteString("\n")
	}

	clusters := make([]Component, 0, len(r.Islands))
	for _, c := range r.Islands {
		if c.Size() > 1 {
			clusters = append(clusters, c)
		}
	}
	if len(clusters) > 0 {
		b.WriteString("Disconnected clusters (linked to each other but not the main graph):\n")
		for _, c := range clusters {
			fmt.Fprintf(&b, "  - %d resources: %s\n", c.Size(), strings.Join(c.Members, ", "))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatJSON renders an IslandReport as indented JSON, for programmatic consumers.
func FormatJSON(r IslandReport) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling report: %w", err)
	}
	return string(data) + "\n", nil
}
