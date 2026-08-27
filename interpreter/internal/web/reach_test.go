package web

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReach verifies the browser wrapper resolves a target against a compiled
// graph and returns the ReachReport as JSON — the same result the CLI produces,
// since both call analyze.Reach.
func TestReach(t *testing.T) {
	// Use a simple fixture with a clear reachability path.
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "or",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "viewer"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "viewer", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)

	out, err := Reach(graph, "r/doc#view")
	require.NoError(t, err)

	var report struct {
		Reachable      bool     `json:"reachable"`
		ReachableTypes []string `json:"reachableTypes"`
		Paths          []struct {
			Hops []struct {
				Relation string `json:"Relation"`
			} `json:"Hops"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(out, &report))

	// view = owner OR viewer, both targeting user.
	require.True(t, report.Reachable)
	require.Equal(t, []string{"user"}, report.ReachableTypes)
	require.Len(t, report.Paths, 2, "OR should produce two paths")

	// Verify both paths reach user via different relations.
	relations := []string{report.Paths[0].Hops[0].Relation, report.Paths[1].Hops[0].Relation}
	require.ElementsMatch(t, []string{"owner", "viewer"}, relations)
}

// TestReachWithFilter verifies subject type filtering works in the browser wrapper.
func TestReachWithFilter(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"r": {}}},
			{"id": "team", "typeName": "team", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [
					{"name": "view", "body": {"kind": "or",
						"left": {"kind": "reference", "name": "owner"},
						"right": {"kind": "reference", "name": "team_viewer"}}}
				]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "doc", "target": "team", "name": "team_viewer", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"}
		]
	}`)

	// Filter to user: should only get owner path.
	out, err := Reach(graph, "r/doc#view@user")
	require.NoError(t, err)

	var report struct {
		SubjectFilter  string `json:"subjectFilter"`
		Reachable      bool   `json:"reachable"`
		ReachableTypes []string `json:"reachableTypes"`
		Paths          []struct {
			SubjectType string `json:"SubjectType"`
			Hops        []struct {
				Relation string `json:"Relation"`
			} `json:"Hops"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(out, &report))

	require.Equal(t, "user", report.SubjectFilter)
	require.True(t, report.Reachable)
	require.ElementsMatch(t, []string{"team", "user"}, report.ReachableTypes, "All reachable types should be listed")
	require.Len(t, report.Paths, 1, "Filter should reduce paths to user only")
	require.Equal(t, "owner", report.Paths[0].Hops[0].Relation)
}

// TestReachErrors surfaces both target-syntax and resolution failures to the
// caller (the browser) rather than panicking.
func TestReachErrors(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [{"id": "doc", "typeName": "doc", "reporters": {"r": {}}}],
		"edges": []
	}`)

	_, err := Reach(graph, "r/doc")
	require.ErrorContains(t, err, "REPORTER/TYPE#RELATION")

	_, err = Reach(graph, "r/doc#does_not_exist")
	require.ErrorContains(t, err, "neither a permission nor a relation")
}
