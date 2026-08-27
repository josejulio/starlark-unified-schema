// Command graph-analyze runs graph-theory analyses over a graph.json artifact
// (see GRAPH.md) and reports structural problems. By default it reports islands
// (resources disconnected from the rest of the schema); with -check it explains
// the read cost of a single check (object#relation). It reads the JSON contract
// only — it does not touch Starlark.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/project-kessel/starlark-unified-schema/internal/analyze"
	"github.com/project-kessel/starlark-unified-schema/internal/cmdio"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

func main() {
	in := flag.String("in", "", "path to graph.json (default: stdin)")
	out := flag.String("out", "", "path to write the report (default: stdout)")
	format := flag.String("format", "text", "report format: text or json")
	check := flag.String("check", "", "explain a check's cost: TYPE[.REPORTER]#RELATION (e.g. workspace.features#enabled_services)")
	flag.Parse()

	// With -check we explain a single check (never a CI-gating finding); otherwise
	// the full island report is emitted and a non-zero exit signals a finding.
	if *check != "" {
		if err := runCheck(*in, *out, *format, *check); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	found, err := run(*in, *out, *format)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if found {
		os.Exit(1)
	}
}

// runCheck parses the check target, explains it against the graph, and writes the
// report in the requested format.
func runCheck(in, out, format, target string) error {
	data, err := cmdio.Read(in)
	if err != nil {
		return err
	}
	doc, err := graphdoc.Parse(data)
	if err != nil {
		return err
	}

	object, relation, err := analyze.ParseCheckTarget(target)
	if err != nil {
		return err
	}

	root, err := analyze.ExplainCheck(doc, object, relation)
	if err != nil {
		return err
	}

	var rendered string
	switch format {
	case "text":
		rendered = analyze.FormatCheckText(object, relation, root)
	case "json":
		rendered, err = analyze.FormatCheckJSON(root)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q (want text or json)", format)
	}
	return cmdio.Write(out, []byte(rendered))
}

// run emits the report and returns whether the analysis found any structural
// problem (an island). An error is returned only for operational failures
// (bad input, unknown format, write failure).
func run(in, out, format string) (bool, error) {
	data, err := cmdio.Read(in)
	if err != nil {
		return false, err
	}

	doc, err := graphdoc.Parse(data)
	if err != nil {
		return false, err
	}

	report := analyze.Islands(doc)

	var rendered string
	switch format {
	case "text":
		rendered = analyze.FormatText(report)
	case "json":
		rendered, err = analyze.FormatJSON(report)
		if err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unknown format %q (want text or json)", format)
	}

	if err := cmdio.Write(out, []byte(rendered)); err != nil {
		return false, err
	}
	return report.HasFindings(), nil
}
