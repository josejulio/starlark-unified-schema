// Command graph-analyze runs graph-theory analyses over a graph.json artifact
// (see GRAPH.md) and reports structural problems — currently islands: resources
// disconnected from the rest of the schema. It reads the JSON contract only — it
// does not touch Starlark.
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
	flag.Parse()

	// The full report is always emitted first; a non-zero exit signals that the
	// analysis found structural problems, for CI gating.
	found, err := run(*in, *out, *format)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if found {
		os.Exit(1)
	}
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
