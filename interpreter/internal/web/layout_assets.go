//go:build !js

package web

import _ "embed"

// Lazy-loaded Cytoscape layout libraries. Unlike dagre (inlined into every page
// as the default), these heavier layouts are written next to the playground HTML
// as sidecar files and fetched only when the user selects them — elk alone is
// ~1.6 MB, so inlining all of them would triple the page. They are excluded from
// the js/wasm build (`//go:build !js`) so the in-browser compiler binary
// (cmd/graph-wasm, which imports this package for web.Elements) does not carry
// layout JS it never uses.

//go:embed vendor/layout-base.js
var layoutBaseJS string

//go:embed vendor/cose-base.js
var coseBaseJS string

//go:embed vendor/cytoscape-fcose.js
var cytoscapeFcoseJS string

//go:embed vendor/cola.min.js
var colaJS string

//go:embed vendor/cytoscape-cola.js
var cytoscapeColaJS string

//go:embed vendor/elk.bundled.js
var elkJS string

//go:embed vendor/cytoscape-elk.js
var cytoscapeElkJS string

// LayoutAssets returns the lazy-loaded layout bundles, keyed by the sidecar
// filename the playground fetches on demand (e.g. "layout-fcose.js"). Each value
// concatenates a Cytoscape layout extension and its dependencies in load order,
// so loading the single file registers the layout with the global cytoscape
// (each extension auto-registers when window.cytoscape is present). Callers write
// these next to the generated page; see cmd/graph-playground.
func LayoutAssets() map[string]string {
	return map[string]string{
		"layout-fcose.js": layoutBaseJS + "\n" + coseBaseJS + "\n" + cytoscapeFcoseJS,
		"layout-cola.js":  colaJS + "\n" + cytoscapeColaJS,
		"layout-elk.js":   elkJS + "\n" + cytoscapeElkJS,
	}
}
