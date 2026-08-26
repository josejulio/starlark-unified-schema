// Command graph-playground assembles the live, in-browser schema playground: a
// page that carries the Starlark schema source, compiles it to the canonical
// graph with a Go WASM binary (cmd/graph-wasm), and renders the result with
// Cytoscape.js — all client-side, no server round-trips after load.
//
// Unlike the other graph consumers (which read a finished graph.json), this reads
// the schema *source* directory. The page fetches two sidecars at runtime: the
// compiled WASM binary and Go's wasm_exec.js, so it must be served over http (see
// the Makefile's serve-graph-playground target).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"flag"

	"github.com/project-kessel/starlark-unified-schema/internal/cmdio"
	"github.com/project-kessel/starlark-unified-schema/internal/web"
)

func main() {
	src := flag.String("src", "schema", "path to the schema source directory")
	out := flag.String("out", "", "path to write the playground HTML (default: stdout)")
	layout := flag.String("layout", "fcose", "Cytoscape layout name (fcose, dagre, cola, elk)")
	wasm := flag.String("wasm", "graph-playground.wasm", "relative URL the page fetches the compiled WASM from")
	flag.Parse()

	if err := run(*src, *out, *layout, *wasm); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(src, out, layout, wasm string) error {
	files, err := readSchemaFiles(src)
	if err != nil {
		return err
	}

	page, err := web.BuildPlayground(files, web.PlaygroundOptions{Layout: layout, WASMURL: wasm})
	if err != nil {
		return err
	}

	if err := cmdio.Write(out, []byte(page)); err != nil {
		return err
	}
	if out == "" {
		return nil
	}

	// Write the lazy-loaded layout libraries as sidecars next to the page. The
	// playground fetches these only when the user picks a non-default layout, so
	// they stay out of the (already large) inlined page.
	dir := filepath.Dir(out)
	for name, contents := range web.LayoutAssets() {
		dst := filepath.Join(dir, name)
		if err := os.WriteFile(dst, []byte(contents), 0644); err != nil {
			return fmt.Errorf("writing layout sidecar %s: %w", dst, err)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", dst)
	}
	return nil
}

// readSchemaFiles walks the schema directory and returns every .star file keyed
// by slash path relative to src — the keys lang.CompileGraph (and the browser
// file switcher) expect, e.g. "kessel.star", "host/reporters/hbi/host.star".
func readSchemaFiles(src string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".star" {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = contents
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading schema files from %s: %w", src, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .star files found under %s", src)
	}
	return files, nil
}
