package web

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExplainCheck verifies the browser wrapper resolves a target against a
// compiled graph and returns the annotated proof tree as JSON — the same result
// the CLI produces, since both call analyze.ExplainCheck.
func TestExplainCheck(t *testing.T) {
	out, err := ExplainCheck(fixture, "workspace.features#view")
	require.NoError(t, err)

	var root struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		Cost struct {
			BigO      string `json:"bigO"`
			Recursive bool   `json:"recursive"`
		} `json:"cost"`
	}
	require.NoError(t, json.Unmarshal(out, &root))

	// view = owner OR parent→view, and parent is an AtMostOne self-relation, so the
	// check walks the workspace tree: depth-bound and recursive.
	require.Equal(t, "permission", root.Kind)
	require.Equal(t, "view", root.Name)
	require.Equal(t, "O(D_workspace)", root.Cost.BigO)
	require.True(t, root.Cost.Recursive)
}

// TestExplainCheckErrors surfaces both target-syntax and resolution failures to
// the caller (the browser) rather than panicking.
func TestExplainCheckErrors(t *testing.T) {
	_, err := ExplainCheck(fixture, "workspace.features")
	require.ErrorContains(t, err, "TYPE[.REPORTER]#RELATION")

	_, err = ExplainCheck(fixture, "workspace.features#does_not_exist")
	require.ErrorContains(t, err, "neither a permission nor a relation")
}
