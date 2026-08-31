package analyze

import (
	"testing"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
	"github.com/stretchr/testify/require"
)

// TestCheckReachableDirectRelation checks a simple reachable case: a direct relation
// leaf that targets the subject.
func TestCheckReachableDirectRelation(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "owner", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 1)
	require.False(t, v.Paths[0].Excluded)
	require.Len(t, v.Paths[0].Hops, 1)
	require.Equal(t, "owner", v.Paths[0].Hops[0].Relation)
	require.Equal(t, "user", v.Paths[0].Hops[0].ToType)
	require.Equal(t, "u", v.Paths[0].Hops[0].ToReporter)
}

// TestCheckReachableUnreachable verifies that a check with no matching subject
// returns unreachable with no paths.
func TestCheckReachableUnreachable(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "group", "typeName": "group", "reporters": {"g": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// Subject is group.g, but the relation targets user.u
	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "owner", FacetRef{"group", "g"})
	require.NoError(t, err)
	require.Equal(t, "unreachable", v.Verdict)
	require.Len(t, v.Paths, 0)
}

// TestCheckReachableFullFacetMatching verifies that subject matching requires
// BOTH type and reporter to match (full-facet rule).
func TestCheckReachableFullFacetMatching(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}, "v": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// Relation targets user.u, subject is user.v — type matches but reporter differs
	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "owner", FacetRef{"user", "v"})
	require.NoError(t, err)
	require.Equal(t, "unreachable", v.Verdict)
	require.Len(t, v.Paths, 0)

	// Now check with the correct facet
	v, err = CheckReachable(doc, FacetRef{"doc", "r"}, "owner", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 1)
}

// TestCheckReachableOr verifies that an OR produces multiple witness paths when
// both operands reach the subject.
func TestCheckReachableOr(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [{"name": "view", "body": {"kind": "or", "left": {"kind": "reference", "name": "owner"}, "right": {"kind": "reference", "name": "editor"}}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "editor", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "view", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 2) // Two distinct paths via owner and editor
	require.False(t, v.Paths[0].Excluded)
	require.False(t, v.Paths[1].Excluded)
}

// TestCheckReachableAnd verifies that AND searches both operands.
func TestCheckReachableAnd(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [{"name": "admin", "body": {"kind": "and", "left": {"kind": "reference", "name": "owner"}, "right": {"kind": "reference", "name": "can_admin"}}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "can_admin", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "admin", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 2) // Both operands reach user.u
}

// TestCheckReachableUnless verifies that the right operand of UNLESS is flagged
// as an exclusion path.
func TestCheckReachableUnless(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [{"name": "view", "body": {"kind": "unless", "left": {"kind": "reference", "name": "viewer"}, "right": {"kind": "reference", "name": "blocked"}}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "user", "name": "viewer", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "blocked", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "view", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 2)

	// Left operand (viewer) should be a grant path
	grant := v.Paths[0]
	require.False(t, grant.Excluded)
	require.Equal(t, "viewer", grant.Hops[0].Relation)

	// Right operand (blocked) should be an exclusion path
	exclusion := v.Paths[1]
	require.True(t, exclusion.Excluded)
	require.Equal(t, "blocked", exclusion.Hops[0].Relation)
}

// TestCheckReachableExclusionOnly verifies the exclusion-only verdict tier when
// the subject is reachable ONLY through the right operand of UNLESS.
func TestCheckReachableExclusionOnly(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "group", "typeName": "group", "reporters": {"g": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [{"name": "access", "body": {"kind": "unless", "left": {"kind": "reference", "name": "viewers"}, "right": {"kind": "reference", "name": "banned"}}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "group", "name": "viewers", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "g"},
			{"kind": "relation", "source": "doc", "target": "user", "name": "banned", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// Subject is user.u: only reachable via banned (right operand, exclusion)
	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "access", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "exclusion-only", v.Verdict)
	require.Len(t, v.Paths, 1)
	require.True(t, v.Paths[0].Excluded)
}

// TestCheckReachableArrowChain verifies that subreference (arrow) hops are
// prepended in order.
func TestCheckReachableArrowChain(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "team", "typeName": "team", "reporters": {"r": {}}},
			{"id": "doc", "typeName": "doc", "reporters": {"r": {
				"permissions": [{"name": "view", "body": {"kind": "subreference", "name": "team", "sub": "members"}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "doc", "target": "team", "name": "team", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r"},
			{"kind": "relation", "source": "team", "target": "user", "name": "members", "cardinality": "Many", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	v, err := CheckReachable(doc, FacetRef{"doc", "r"}, "view", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.Len(t, v.Paths, 1)
	require.Len(t, v.Paths[0].Hops, 2)

	// First hop: doc.r --team--> team.r
	hop1 := v.Paths[0].Hops[0]
	require.Equal(t, "doc", hop1.FromType)
	require.Equal(t, "r", hop1.FromReporter)
	require.Equal(t, "team", hop1.Relation)
	require.Equal(t, "team", hop1.ToType)
	require.Equal(t, "r", hop1.ToReporter)
	require.False(t, hop1.Fanout)

	// Second hop: team.r --members--> user.u
	hop2 := v.Paths[0].Hops[1]
	require.Equal(t, "team", hop2.FromType)
	require.Equal(t, "members", hop2.Relation)
	require.Equal(t, "user", hop2.ToType)
	require.True(t, hop2.Fanout) // Many cardinality
}

// TestCheckReachableRecursion verifies that a recursive node is a dead-end for
// witnesses (no new subject facet is introduced).
func TestCheckReachableRecursion(t *testing.T) {
	graph := []byte(`{
		"version": "1",
		"nodes": [
			{"id": "user", "typeName": "user", "reporters": {"u": {}}},
			{"id": "folder", "typeName": "folder", "reporters": {"r": {
				"permissions": [{"name": "view", "body": {"kind": "or", "left": {"kind": "reference", "name": "owner"}, "right": {"kind": "subreference", "name": "parent", "sub": "view"}}}]
			}}}
		],
		"edges": [
			{"kind": "relation", "source": "folder", "target": "user", "name": "owner", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "u"},
			{"kind": "relation", "source": "folder", "target": "folder", "name": "parent", "cardinality": "AtMostOne", "scope": "reporter", "sourceReporter": "r", "targetReporter": "r", "self": true}
		]
	}`)
	doc, err := graphdoc.Parse(graph)
	require.NoError(t, err)

	// view = owner OR parent->view. The right operand recurses but does not introduce
	// a new terminal (user.u is already discovered via the left operand).
	v, err := CheckReachable(doc, FacetRef{"folder", "r"}, "view", FacetRef{"user", "u"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)

	// Only one witness path: the non-recursive owner branch
	require.Len(t, v.Paths, 1)
	require.Equal(t, "owner", v.Paths[0].Hops[0].Relation)
}

// TestCheckReachableRealSchema is the golden guarantee against the committed schema.
// It exercises at least one reachable, one unreachable, and (if the schema has an
// unless) one exclusion-only query.
func TestCheckReachableRealSchema(t *testing.T) {
	doc := compileRealSchema(t)

	// Reachable: workspace.features#enabled_services -> should reach some subject
	// Let's test if we can reach billing_account.features via direct_billing_account
	v, err := CheckReachable(doc, FacetRef{"workspace", "features"}, "direct_billing_account", FacetRef{"billing_account", "features"})
	require.NoError(t, err)
	require.Equal(t, "reachable", v.Verdict)
	require.NotEmpty(t, v.Paths)

	// Unreachable: workspace.rbac#parent cannot reach billing_account.features
	// (different types, no connection in the schema)
	v, err = CheckReachable(doc, FacetRef{"workspace", "rbac"}, "parent", FacetRef{"billing_account", "features"})
	require.NoError(t, err)
	require.Equal(t, "unreachable", v.Verdict)
	require.Empty(t, v.Paths)
}

func TestCheckReachableErrors(t *testing.T) {
	doc := compileRealSchema(t)

	// Unknown object type
	_, err := CheckReachable(doc, FacetRef{"nope", "r"}, "view", FacetRef{"workspace", "rbac"})
	require.ErrorContains(t, err, "unknown resource type")

	// Unknown object reporter
	_, err = CheckReachable(doc, FacetRef{"workspace", "nope"}, "view", FacetRef{"workspace", "rbac"})
	require.ErrorContains(t, err, "no reporter")

	// Unknown subject type
	_, err = CheckReachable(doc, FacetRef{"workspace", "rbac"}, "parent", FacetRef{"nope", "r"})
	require.ErrorContains(t, err, "unknown subject type")

	// Unknown subject reporter
	_, err = CheckReachable(doc, FacetRef{"workspace", "rbac"}, "parent", FacetRef{"workspace", "nope"})
	require.ErrorContains(t, err, "has no reporter")

	// Relation does not exist
	_, err = CheckReachable(doc, FacetRef{"workspace", "rbac"}, "does_not_exist", FacetRef{"workspace", "rbac"})
	require.ErrorContains(t, err, "neither a permission nor a relation")
}

func TestParseReachTarget(t *testing.T) {
	// Valid target
	obj, rel, subj, err := ParseReachTarget("rbac/workspace#parent@features/workspace")
	require.NoError(t, err)
	require.Equal(t, "workspace", obj.TypeName)
	require.Equal(t, "rbac", obj.Reporter)
	require.Equal(t, "parent", rel)
	require.Equal(t, "workspace", subj.TypeName)
	require.Equal(t, "features", subj.Reporter)

	// Missing @
	_, _, _, err = ParseReachTarget("rbac/workspace#parent")
	require.ErrorContains(t, err, "subject required")

	// Empty subject
	_, _, _, err = ParseReachTarget("rbac/workspace#parent@")
	require.ErrorContains(t, err, "missing a subject")

	// Subject without reporter (not a full facet)
	_, _, _, err = ParseReachTarget("rbac/workspace#parent@user")
	require.ErrorContains(t, err, "must be a full facet")

	// Invalid object#relation part
	_, _, _, err = ParseReachTarget("workspace@u/user")
	require.ErrorContains(t, err, "RELATION")
}
