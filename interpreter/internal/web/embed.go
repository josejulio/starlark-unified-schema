package web

import _ "embed"

// Vendored, self-contained assets embedded into the generated page. Keeping the
// HTML/CSS/JS and the Cytoscape libraries as real files (assembled in Go) is far
// more maintainable than Go string literals and keeps the page offline-capable.
//
// renderJS is the Cytoscape rendering core used by the live playground
// (playground.js).
//
//go:embed render.js
var renderJS string

//go:embed vendor/cytoscape.min.js
var cytoscapeJS string

//go:embed vendor/dagre.min.js
var dagreJS string

//go:embed vendor/cytoscape-dagre.js
var cytoscapeDagreJS string

// Playground assets: the live in-browser WASM producer page (see playground.go).
//
//go:embed playground.html
var playgroundHTML string

//go:embed playground.js
var playgroundJS string

//go:embed vendor/codemirror.min.js
var codemirrorJS string

//go:embed vendor/codemirror.min.css
var codemirrorCSS string

//go:embed vendor/codemirror-python.min.js
var codemirrorPythonJS string

//go:embed vendor/codemirror-dracula.min.css
var codemirrorThemeCSS string

// defaultLayout is the Cytoscape layout used when none is requested. fcose is
// compound-aware — it keeps each type's facets tight inside their box, which
// reads best for this grouped graph. It is a lazy-loaded sidecar (see
// LayoutAssets); the page preloads it at boot and falls back to the inlined dagre
// if the sidecar can't be fetched. The layout is swappable via the
// graph-playground -layout flag.
const defaultLayout = "fcose"

// Placeholder tokens replaced during playground assembly. Each is a JS/JSON
// comment so the raw playground.html remains valid HTML and is editable on its own.
const (
	placeholderCytoscape        = "/*__CYTOSCAPE_JS__*/"
	placeholderDagre            = "/*__DAGRE_JS__*/"
	placeholderCytoscapeDagre   = "/*__CYTOSCAPE_DAGRE_JS__*/"
	placeholderLayout           = "__LAYOUT__"
	placeholderRender           = "/*__RENDER_JS__*/"
	placeholderCodemirrorJS     = "/*__CODEMIRROR_JS__*/"
	placeholderCodemirrorCSS    = "/*__CODEMIRROR_CSS__*/"
	placeholderCodemirrorPython = "/*__CODEMIRROR_PYTHON_JS__*/"
	placeholderCodemirrorTheme  = "/*__CODEMIRROR_THEME_CSS__*/"
	placeholderPlaygroundApp    = "/*__PLAYGROUND_JS__*/"
	placeholderFiles            = "/*__FILES__*/"
	placeholderWASMURL          = "__WASM_URL__"
)
