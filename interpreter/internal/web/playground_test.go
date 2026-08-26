package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var playgroundFiles = map[string][]byte{
	"kessel.star":    []byte("# prelude\n"),
	"host/host.star": []byte(`load("kessel.star", "resource")` + "\n"),
}

func TestBuildPlayground(t *testing.T) {
	page, err := BuildPlayground(playgroundFiles, PlaygroundOptions{})
	require.NoError(t, err)

	// The schema sources are inlined for the editor + compiler, keyed by path.
	require.Contains(t, page, `<script id="schema-files" type="application/json">`)
	require.Contains(t, page, `"host/host.star"`)
	require.Contains(t, page, `"kessel.star"`)

	// Defaults injected; the wasm + wasm_exec.js are sidecars.
	require.Contains(t, page, `window.GRAPH_LAYOUT = "fcose";`)
	require.Contains(t, page, `window.WASM_URL = "graph-playground.wasm";`)
	require.Contains(t, page, `<script src="wasm_exec.js"></script>`)

	// The editor, rendering core and vendored libraries are inlined; every
	// placeholder was substituted.
	require.Contains(t, page, "CodeMirror")               // from vendor/codemirror.min.js
	require.Contains(t, page, "The Cytoscape Consortium") // from vendor/cytoscape.min.js
	require.Contains(t, page, "window.KesselRender")      // from render.js
	for _, ph := range []string{
		placeholderCodemirrorCSS, placeholderCodemirrorTheme, placeholderCytoscape,
		placeholderDagre, placeholderCytoscapeDagre, placeholderCodemirrorJS,
		placeholderCodemirrorPython, placeholderFiles, placeholderLayout,
		placeholderWASMURL, placeholderRender, placeholderPlaygroundApp,
	} {
		require.NotContains(t, page, ph, "unsubstituted placeholder: %s", ph)
	}
}

func TestBuildPlaygroundOptions(t *testing.T) {
	page, err := BuildPlayground(playgroundFiles, PlaygroundOptions{Layout: "fcose", WASMURL: "dist/compiler.wasm"})
	require.NoError(t, err)
	require.Contains(t, page, `window.GRAPH_LAYOUT = "fcose";`)
	require.Contains(t, page, `window.WASM_URL = "dist/compiler.wasm";`)
}

func TestBuildPlaygroundEmpty(t *testing.T) {
	_, err := BuildPlayground(map[string][]byte{}, PlaygroundOptions{})
	require.Error(t, err)
}

// TestBuildPlaygroundEscapesScriptClose ensures a "</script>" in the sources
// cannot break out of the inlined data block.
func TestBuildPlaygroundEscapesScriptClose(t *testing.T) {
	page, err := BuildPlayground(map[string][]byte{
		"kessel.star": []byte("x = '</script>'\n"),
	}, PlaygroundOptions{})
	require.NoError(t, err)
	require.NotContains(t, page, "</script>'")
	require.Contains(t, page, `<\/script>`)
}
