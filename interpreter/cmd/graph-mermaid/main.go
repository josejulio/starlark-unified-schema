// Command graph-mermaid renders a Mermaid flowchart from a graph.json artifact
// (see GRAPH.md). It reads the JSON contract only — it does not touch Starlark.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/project-kessel/starlark-unified-schema/internal/cmdio"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/project-kessel/starlark-unified-schema/internal/render"
)

func main() {
	in := flag.String("in", "", "path to graph.json (default: stdin)")
	out := flag.String("out", "", "path to write the Mermaid diagram (default: stdout)")
	direction := flag.String("direction", "LR", "Mermaid flowchart direction (LR, TD, ...)")
	legend := flag.Bool("legend", true, "append a legend of the UML multiplicity symbols in use")
	flag.Parse()

	if err := run(*in, *out, *direction, *legend); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(in, out, direction string, legend bool) error {
	data, err := cmdio.Read(in)
	if err != nil {
		return err
	}

	doc, err := graphdoc.Parse(data)
	if err != nil {
		return err
	}

	return cmdio.Write(out, []byte(render.Mermaid(doc, direction, legend)))
}
