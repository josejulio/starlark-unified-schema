package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/analyze"
	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/project-kessel/starlark-unified-schema/internal/lang"
	"github.com/stretchr/testify/require"
)

// TestCheckReachable verifies the browser wrapper resolves a reach target against
// a compiled graph and returns the verdict as JSON — the same result the CLI
// produces, since both call analyze.CheckReachable.
func TestCheckReachable(t *testing.T) {
	// features/workspace#direct_service_preferences is a relation that targets features/service
	out, err := CheckReachable(fixture, "features/workspace#direct_service_preferences@features/service")
	require.NoError(t, err)

	var verdict struct {
		Object struct {
			TypeName string `json:"TypeName"`
			Reporter string `json:"Reporter"`
		} `json:"object"`
		Relation string `json:"relation"`
		Subject  struct {
			TypeName string `json:"TypeName"`
			Reporter string `json:"Reporter"`
		} `json:"subject"`
		Verdict string `json:"verdict"`
		Paths   []any  `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(out, &verdict))

	require.Equal(t, "workspace", verdict.Object.TypeName)
	require.Equal(t, "features", verdict.Object.Reporter)
	require.Equal(t, "direct_service_preferences", verdict.Relation)
	require.Equal(t, "service", verdict.Subject.TypeName)
	require.Equal(t, "features", verdict.Subject.Reporter)
	require.Equal(t, "reachable", verdict.Verdict)
	require.NotEmpty(t, verdict.Paths)
}

// TestCheckReachableErrors surfaces both target-syntax and resolution failures to
// the caller (the browser) rather than panicking.
func TestCheckReachableErrors(t *testing.T) {
	_, err := CheckReachable(fixture, "features/workspace#parent")
	require.ErrorContains(t, err, "subject required")

	_, err = CheckReachable(fixture, "features/workspace#does_not_exist@features/service")
	require.ErrorContains(t, err, "neither a permission nor a relation")

	_, err = CheckReachable(fixture, "features/workspace#parent@workspace")
	require.ErrorContains(t, err, "must be a full facet")
}

// TestReachMatchesFilesystem is the parity golden: compile the committed schema
// via lang.CompileGraph (the same path the CLI and the playground use), run the
// analysis via the CLI path (analyze.CheckReachable) and via the web wrapper
// (web.CheckReachable), and assert byte-identical. This pins the "browser matches
// native" guarantee.
func TestReachMatchesFilesystem(t *testing.T) {
	const schemaDir = "../../../schema"

	files := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(schemaDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".star" {
			return nil
		}
		rel, err := filepath.Rel(schemaDir, path)
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

	// Compile the schema once
	graphJSON, err := lang.CompileGraph(files)
	require.NoError(t, err)

	// Parse it for the native path
	doc, err := graphdoc.Parse(graphJSON)
	require.NoError(t, err)

	// Test cases: reachable, unreachable
	testCases := []struct {
		name   string
		target string
	}{
		{"reachable", "features/workspace#direct_billing_account@features/billing_account"},
		{"unreachable", "rbac/workspace#parent@features/billing_account"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Native path: analyze.CheckReachable
			object, relation, subject, err := analyze.ParseReachTarget(tc.target)
			require.NoError(t, err)
			verdict, err := analyze.CheckReachable(doc, object, relation, subject)
			require.NoError(t, err)
			nativeJSON, err := analyze.FormatReachJSON(verdict)
			require.NoError(t, err)

			// Web path: web.CheckReachable
			webJSON, err := CheckReachable(graphJSON, tc.target)
			require.NoError(t, err)

			// Byte-identical
			require.JSONEq(t, nativeJSON, string(webJSON), "native and web paths must produce identical JSON")
		})
	}
}
