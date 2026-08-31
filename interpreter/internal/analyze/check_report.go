package analyze

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// FormatCheckText renders a check proof tree as an indented, human-readable report
// with the headline cost, the per-node rewrite walk, and the cost variables.
func FormatCheckText(object FacetRef, relation string, root *CheckNode) string {
	var b strings.Builder

	title := fmt.Sprintf("Check cost: %s#%s", object, relation)
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("=", len(title)) + "\n\n")

	fmt.Fprintf(&b, "Cost:      %s\n", root.Cost.BigO)
	fmt.Fprintf(&b, "Depth:     %d sequential hop(s)   Fan-out sites: %d   Recursive: %t\n\n",
		root.Cost.DispatchDepth, root.Cost.FanoutSites, root.Cost.Recursive)

	b.WriteString("Proof tree (how the check is evaluated):\n")
	fmt.Fprintf(&b, "%s    %s\n", nodeLabel(root), root.Cost.BigO)
	kids := childrenOf(root)
	for i, kid := range kids {
		writeCheckNode(&b, kid, "", i == len(kids)-1)
	}

	if vars := collectVars(root); len(vars) > 0 {
		b.WriteString("\nCost variables (fixed only by real data):\n")
		for _, v := range vars {
			fmt.Fprintf(&b, "  %-16s %s [%s]\n", v.Name, v.Meaning, v.Source)
		}
	}
	return b.String()
}

// writeCheckNode prints one non-root node and recurses, drawing an ASCII tree.
func writeCheckNode(b *strings.Builder, n *CheckNode, prefix string, last bool) {
	branch, next := "├─ ", prefix+"│  "
	if last {
		branch, next = "└─ ", prefix+"   "
	}
	fmt.Fprintf(b, "%s%s%s    %s\n", prefix, branch, nodeLabel(n), n.Cost.BigO)

	kids := childrenOf(n)
	for i, kid := range kids {
		writeCheckNode(b, kid, next, i == len(kids)-1)
	}
}

// childrenOf returns a node's structural children in display order.
func childrenOf(n *CheckNode) []*CheckNode {
	if n.Body != nil {
		return []*CheckNode{n.Body}
	}
	return n.Children
}

// nodeLabel renders the kind-specific one-line description of a node.
func nodeLabel(n *CheckNode) string {
	switch n.Kind {
	case "permission":
		return fmt.Sprintf("permission %q on %s/%s", n.Name, n.Reporter, n.TypeName)
	case "relation":
		return fmt.Sprintf("relation %q (%s)", n.Name, graphdoc.Multiplicity(n.Cardinality))
	case "op":
		return strings.ToUpper(n.Op)
	case "arrow":
		if len(n.Children) == 0 {
			return fmt.Sprintf("%s → %s [%s]", n.Name, n.Sub, n.Note)
		}
		return fmt.Sprintf("%s (%s) → %s/%s ⇒ %s",
			n.Name, graphdoc.Multiplicity(n.Cardinality), n.TargetReporter, n.TargetType, n.Sub)
	case "recursive":
		return fmt.Sprintf("↺ %s on %s/%s (recursion)", n.Name, n.Reporter, n.TypeName)
	case "unresolved":
		return fmt.Sprintf("%s (unresolved: %s)", n.Name, n.Note)
	default:
		return n.Kind
	}
}

// collectVars gathers the unique cost variables across the whole tree, sorted.
func collectVars(root *CheckNode) []CostVar {
	seen := map[string]CostVar{}
	var walk func(*CheckNode)
	walk = func(n *CheckNode) {
		for _, v := range n.Cost.Vars {
			seen[v.Name] = v
		}
		for _, kid := range childrenOf(n) {
			walk(kid)
		}
	}
	walk(root)
	out := make([]CostVar, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FormatCheckJSON renders the proof tree as indented JSON for programmatic use.
func FormatCheckJSON(root *CheckNode) (string, error) {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling check: %w", err)
	}
	return string(data) + "\n", nil
}
