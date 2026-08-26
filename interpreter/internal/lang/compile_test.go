package lang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/output"
	"github.com/stretchr/testify/require"
)

// TestCompileGraphMatchesFilesystem is the Phase 4 acceptance guarantee: the
// in-memory compile path used by the browser (CompileGraph) produces graph.json
// byte-identical to the native filesystem path cmd/interpreter runs, over the
// real schema. A compile in the WASM playground therefore equals the CLI.
func TestCompileGraphMatchesFilesystem(t *testing.T) {
	const schemaDir = "../../../schema"

	// Native path: filesystem loader + GraphVisitor (what cmd/interpreter runs
	// with no file arguments — it walks every .star under the schema dir).
	loader := NewLoader(schemaDir)
	processor := NewProcessor(loader)
	graph := output.NewGraphVisitor()
	require.NoError(t, processor.Process(graph))
	results, err := graph.Results()
	require.NoError(t, err)
	require.Len(t, results, 1)
	native := results[0].Contents

	// In-memory path: read the same files into a map and compile.
	files := readStarFiles(t, schemaDir)
	fromMemory, err := CompileGraph(files)
	require.NoError(t, err)

	require.Equal(t, string(native), string(fromMemory))
}

// TestCompileGraphReportsErrors surfaces Starlark errors as a returned error
// (which the WASM entry turns into a structured message for the editor).
func TestCompileGraphReportsErrors(t *testing.T) {
	files := readStarFiles(t, "../../../schema")
	files["broken.star"] = []byte(`load("kessel.star", "resource")` + "\nthis is not valid starlark(")

	_, err := CompileGraph(files)
	require.Error(t, err)
}

func readStarFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".star" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = contents
		return nil
	}))
	return files
}
