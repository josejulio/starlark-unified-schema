package analyze

import (
	"fmt"
	"sort"
	"strings"

	"github.com/project-kessel/starlark-unified-schema/internal/graphdoc"
)

// This file implements ExplainCheck: a symbolic evaluation of a Kessel check
// (object#relation@subject, mirroring inventory-api's CheckRequest) against the
// canonical graph.json. It walks the permission rewrite from the object down the
// relation topology and returns a proof tree annotated with cost. Like the rest
// of internal/analyze it reads only the documented JSON contract via graphdoc.
//
// The analysis is *structural*: it computes the shape and asymptotics of a check
// (does it fan out? recurse? how many hops?) from the schema alone, with no
// instance data. The subject side of a check only ever matters at the leaves
// (does the subject match a resolved relation), which does not change the walk,
// so ExplainCheck takes object + relation and is subject-independent. Absolute
// latency needs real tuples and a resolver; this ranks alternatives.

// FacetRef identifies a resource type's reporter facet — the object a check is
// asked against (ResourceReference.resource_type + reporter).
type FacetRef struct {
	TypeName string
	Reporter string
}

func (f FacetRef) String() string { return f.TypeName + "." + f.Reporter }

// CostVar is a symbolic quantity a check's cost depends on: a hierarchy depth or
// a per-relation fan-out that only concrete data can fix to a number.
type CostVar struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
	Source  string `json:"source"`
}

// Cost is the annotation carried by every proof-tree node. BigO is the symbolic
// headline; DispatchDepth (sequential arrow hops, recursion counted once) is a
// static latency proxy; FanoutSites (arrows over many-cardinality relations) is a
// static work proxy; Recursive flags a check that walks a relation cycle (its
// true depth is the corresponding CostVar, not DispatchDepth).
type Cost struct {
	BigO          string    `json:"bigO"`
	DispatchDepth int       `json:"dispatchDepth"`
	FanoutSites   int       `json:"fanoutSites"`
	Recursive     bool      `json:"recursive"`
	Vars          []CostVar `json:"vars,omitempty"`
}

// CheckNode is one node of the proof tree. Kind is one of:
//   - "permission" — a computed permission expanding to its rewrite (Body)
//   - "relation"   — a direct relation leaf (a constant-time membership check)
//   - "op"         — an or/and/unless operator over Children (two operands)
//   - "arrow"      — a subreference: traverse Name to a target, evaluate Sub there
//     (the target evaluation is Children[0]); TargetType/TargetReporter name it
//   - "recursive"  — a subreference re-entered a permission already on the path;
//     expansion stops here and the enclosing arrow supplies the depth variable
//   - "unresolved" — a name with no matching relation or permission in scope
type CheckNode struct {
	Kind string `json:"kind"`

	TypeName string `json:"typeName,omitempty"`
	Reporter string `json:"reporter,omitempty"`

	Name        string `json:"name,omitempty"`
	Cardinality string `json:"cardinality,omitempty"`
	Op          string `json:"op,omitempty"`
	Sub         string `json:"sub,omitempty"`

	TargetType     string `json:"targetType,omitempty"`
	TargetReporter string `json:"targetReporter,omitempty"`

	Body     *CheckNode   `json:"body,omitempty"`
	Children []*CheckNode `json:"children,omitempty"`
	Note     string       `json:"note,omitempty"`

	Cost Cost `json:"cost"`

	// Unexported working state used while composing costs bottom-up; the exported
	// Cost is derived from these by finalize.
	expr      costExpr
	depth     int
	fanout    int
	recursive bool
	vars      []CostVar
}

// finalize copies the composed working state into the serialized Cost.
func (n *CheckNode) finalize() *CheckNode {
	n.Cost = Cost{
		BigO:          n.expr.bigO(),
		DispatchDepth: n.depth,
		FanoutSites:   n.fanout,
		Recursive:     n.recursive,
		Vars:          n.vars,
	}
	return n
}

// checkResolver indexes the graph document for name resolution within a facet's
// scope (its own members, its type's common members, and everything inherited
// from facets it extends), mirroring the rules the web highlighter uses.
type checkResolver struct {
	relEdges        []graphdoc.Edge
	inheritParent   map[FacetRef]FacetRef
	facetPerms      map[FacetRef]map[string]graphdoc.Expr
	commonPerms     map[string]map[string]graphdoc.Expr
	reportersByType map[string][]string
}

func newCheckResolver(doc graphdoc.Document) (*checkResolver, error) {
	r := &checkResolver{
		inheritParent:   map[FacetRef]FacetRef{},
		facetPerms:      map[FacetRef]map[string]graphdoc.Expr{},
		commonPerms:     map[string]map[string]graphdoc.Expr{},
		reportersByType: map[string][]string{},
	}
	for _, e := range doc.Edges {
		switch e.Kind {
		case "relation":
			r.relEdges = append(r.relEdges, e)
		case "inherits":
			r.inheritParent[FacetRef{e.Source, e.SourceReporter}] = FacetRef{e.Target, e.TargetReporter}
		}
	}
	for _, n := range doc.Nodes {
		if n.Common != nil && len(n.Common.Permissions) > 0 {
			perms, err := graphdoc.ParsePermissions(n.Common.Permissions)
			if err != nil {
				return nil, fmt.Errorf("%s common: %w", n.TypeName, err)
			}
			r.commonPerms[n.TypeName] = indexPerms(perms)
		}
		reporters := make([]string, 0, len(n.Reporters))
		for rep, facet := range n.Reporters {
			reporters = append(reporters, rep)
			if len(facet.Permissions) > 0 {
				perms, err := graphdoc.ParsePermissions(facet.Permissions)
				if err != nil {
					return nil, fmt.Errorf("%s.%s: %w", n.TypeName, rep, err)
				}
				r.facetPerms[FacetRef{n.TypeName, rep}] = indexPerms(perms)
			}
		}
		sort.Strings(reporters)
		r.reportersByType[n.TypeName] = reporters
	}
	return r, nil
}

func indexPerms(perms []graphdoc.Permission) map[string]graphdoc.Expr {
	m := make(map[string]graphdoc.Expr, len(perms))
	for _, p := range perms {
		m[p.Name] = p.Body
	}
	return m
}

// chain returns f followed by every facet it extends, transitively (child →
// parent), so a facet's scope includes what it inherits. The facet's own
// definitions come first and win on name clashes.
func (r *checkResolver) chain(f FacetRef) []FacetRef {
	out := []FacetRef{}
	seen := map[FacetRef]bool{}
	for !seen[f] {
		seen[f] = true
		out = append(out, f)
		p, ok := r.inheritParent[f]
		if !ok {
			break
		}
		f = p
	}
	return out
}

// relIn returns the relation edges reachable by name in f's scope: f's own
// reporter relations, its type's common relations, and those inherited from
// facets it extends. First writer (f itself) wins.
func (r *checkResolver) relIn(f FacetRef) map[string]graphdoc.Edge {
	m := map[string]graphdoc.Edge{}
	for _, cf := range r.chain(f) {
		for _, e := range r.relEdges {
			if e.Source != cf.TypeName {
				continue
			}
			if e.SourceReporter == cf.Reporter || e.Scope == "common" {
				if _, ok := m[e.Name]; !ok {
					m[e.Name] = e
				}
			}
		}
	}
	return m
}

// permIn returns the permissions in scope for f (own + common + inherited).
func (r *checkResolver) permIn(f FacetRef) map[string]graphdoc.Expr {
	m := map[string]graphdoc.Expr{}
	for _, cf := range r.chain(f) {
		for name, body := range r.facetPerms[cf] {
			if _, ok := m[name]; !ok {
				m[name] = body
			}
		}
		for name, body := range r.commonPerms[cf.TypeName] {
			if _, ok := m[name]; !ok {
				m[name] = body
			}
		}
	}
	return m
}

// ExplainCheck symbolically evaluates object#relation and returns the annotated
// proof tree. It errors only on invalid input (unknown type/reporter, or a
// relation that resolves to nothing).
func ExplainCheck(doc graphdoc.Document, object FacetRef, relation string) (*CheckNode, error) {
	r, err := newCheckResolver(doc)
	if err != nil {
		return nil, err
	}
	reps, ok := r.reportersByType[object.TypeName]
	if !ok {
		return nil, fmt.Errorf("unknown resource type %q", object.TypeName)
	}
	switch {
	case object.Reporter == "" && len(reps) == 1:
		object.Reporter = reps[0]
	case object.Reporter == "":
		return nil, fmt.Errorf("resource %q has multiple reporters (%s); specify one as TYPE.REPORTER", object.TypeName, strings.Join(reps, ", "))
	default:
		if !contains(reps, object.Reporter) {
			return nil, fmt.Errorf("resource %q has no reporter %q (have: %s)", object.TypeName, object.Reporter, strings.Join(reps, ", "))
		}
	}

	root := r.resolveOn(object, relation, map[string]bool{})
	if root.Kind == "unresolved" {
		return nil, fmt.Errorf("%q is neither a permission nor a relation on %s", relation, object)
	}
	return root, nil
}

// resolveOn resolves a bare name in f's scope: a relation edge is a leaf, a
// permission expands into its rewrite (guarded against re-entry), anything else
// is unresolved.
func (r *checkResolver) resolveOn(f FacetRef, name string, path map[string]bool) *CheckNode {
	if e, ok := r.relIn(f)[name]; ok {
		n := &CheckNode{Kind: "relation", TypeName: f.TypeName, Reporter: f.Reporter, Name: name, Cardinality: e.Cardinality}
		n.expr = constExpr()
		return n.finalize()
	}
	if body, ok := r.permIn(f)[name]; ok {
		key := f.String() + "#" + name
		if path[key] {
			n := &CheckNode{Kind: "recursive", TypeName: f.TypeName, Reporter: f.Reporter, Name: name,
				Note: "re-enters " + name + " on " + f.String()}
			n.expr = constExpr()
			n.recursive = true
			return n.finalize()
		}
		path[key] = true
		bodyNode := r.walkExpr(f, body, path)
		delete(path, key)

		n := &CheckNode{Kind: "permission", TypeName: f.TypeName, Reporter: f.Reporter, Name: name, Body: bodyNode}
		n.expr, n.depth, n.fanout, n.recursive, n.vars =
			bodyNode.expr, bodyNode.depth, bodyNode.fanout, bodyNode.recursive, bodyNode.vars
		return n.finalize()
	}
	n := &CheckNode{Kind: "unresolved", TypeName: f.TypeName, Reporter: f.Reporter, Name: name,
		Note: "no relation or permission named " + name + " in scope"}
	n.expr = constExpr()
	return n.finalize()
}

// walkExpr evaluates a rewrite expression in f's scope.
func (r *checkResolver) walkExpr(f FacetRef, e graphdoc.Expr, path map[string]bool) *CheckNode {
	switch e.Kind {
	case "reference":
		return r.resolveOn(f, e.Name, path)
	case "subreference":
		return r.resolveArrow(f, e.Name, e.Sub, path)
	case "and", "or", "unless":
		left := r.walkExpr(f, *e.Left, path)
		right := r.walkExpr(f, *e.Right, path)
		n := &CheckNode{Kind: "op", Op: e.Kind, Children: []*CheckNode{left, right}}
		n.expr = sumExpr(left.expr, right.expr)
		n.depth = maxInt(left.depth, right.depth)
		n.fanout = left.fanout + right.fanout
		n.recursive = left.recursive || right.recursive
		n.vars = mergeVars(left.vars, right.vars)
		return n.finalize()
	default:
		n := &CheckNode{Kind: "unresolved", Note: "unknown expression kind: " + e.Kind}
		n.expr = constExpr()
		return n.finalize()
	}
}

// resolveArrow handles a subreference (name → sub): traverse the relation `name`
// to its target type, then resolve `sub` there. A single-target relation is one
// hop; a many-target relation fans the downstream cost out by a per-relation
// variable; re-entering a permission already on the path becomes the recursion
// whose depth this arrow multiplies.
func (r *checkResolver) resolveArrow(f FacetRef, name, sub string, path map[string]bool) *CheckNode {
	e, ok := r.relIn(f)[name]
	if !ok {
		n := &CheckNode{Kind: "arrow", TypeName: f.TypeName, Reporter: f.Reporter, Name: name, Sub: sub,
			Note: "no relation named " + name + " in scope"}
		n.expr = constExpr()
		return n.finalize()
	}

	child := r.resolveSubOnType(e.Target, e.TargetReporter, sub, path)
	n := &CheckNode{
		Kind: "arrow", TypeName: f.TypeName, Reporter: f.Reporter,
		Name: name, Cardinality: e.Cardinality, Sub: sub,
		TargetType: child.TypeName, TargetReporter: child.Reporter,
		Children: []*CheckNode{child},
	}
	source := f.TypeName + "." + name
	many := isManyCardinality(e.Cardinality)

	if child.Kind == "recursive" {
		// This arrow drives a relation cycle: its cost is the traversal depth (a
		// tree walk for a single-target parent, the reachable set when it fans out).
		v := r.depthVar(e, source)
		n.expr = varExpr(v.Name)
		n.depth = 1
		n.fanout = boolToInt(many)
		n.recursive = true
		n.vars = mergeVars(child.vars, []CostVar{v})
		return n.finalize()
	}

	n.depth = child.depth + 1
	n.recursive = child.recursive
	if many {
		fv := "N_" + name
		n.expr = mulFactor(child.expr, fv)
		n.fanout = child.fanout + 1
		n.vars = mergeVars(child.vars, []CostVar{{
			Name:    fv,
			Meaning: "number of " + name + " targets on a " + f.TypeName,
			Source:  source,
		}})
	} else {
		n.expr = child.expr
		n.fanout = child.fanout
		n.vars = child.vars
	}
	return n.finalize()
}

// resolveSubOnType resolves a subreference's downstream name on the target type.
// The relation names one reporter facet (preferred), but the sub may live on a
// different facet of the same type — e.g. `parent` targets workspace.rbac while
// the `_paid_services` permission it reaches lives on workspace.features — so on
// a miss it falls back to the type's other facets in sorted order.
func (r *checkResolver) resolveSubOnType(targetType, preferred, sub string, path map[string]bool) *CheckNode {
	order := []string{}
	if preferred != "" {
		order = append(order, preferred)
	}
	for _, rep := range r.reportersByType[targetType] {
		if rep != preferred {
			order = append(order, rep)
		}
	}
	for _, rep := range order {
		if n := r.resolveOn(FacetRef{targetType, rep}, sub, path); n.Kind != "unresolved" {
			return n
		}
	}
	return r.resolveOn(FacetRef{targetType, preferred}, sub, path)
}

// depthVar names the symbolic depth a recursive arrow multiplies: a bounded tree
// walk when the back-relation is single-target, the reachable set when it fans out.
func (r *checkResolver) depthVar(e graphdoc.Edge, source string) CostVar {
	if isManyCardinality(e.Cardinality) {
		return CostVar{
			Name:    "reach(" + e.Target + ")",
			Meaning: "number of " + e.Target + " reachable via " + e.Name + " (fan-out cycle; bounded by caching)",
			Source:  source,
		}
	}
	return CostVar{
		Name:    "D_" + e.Target,
		Meaning: "depth of the " + e.Target + " hierarchy traversed via " + e.Name,
		Source:  source,
	}
}

// ParseCheckTarget splits "TYPE[.REPORTER]#RELATION" into the object facet and
// relation a check is asked against. It is shared by the CLI and the WASM wrapper
// so both accept the same target syntax.
func ParseCheckTarget(s string) (FacetRef, string, error) {
	hash := strings.LastIndex(s, "#")
	if hash < 0 {
		return FacetRef{}, "", fmt.Errorf("check target %q must be TYPE[.REPORTER]#RELATION", s)
	}
	left, relation := s[:hash], s[hash+1:]
	if relation == "" {
		return FacetRef{}, "", fmt.Errorf("check target %q is missing a relation after '#'", s)
	}
	ref := FacetRef{TypeName: left}
	if dot := strings.Index(left, "."); dot >= 0 {
		ref.TypeName, ref.Reporter = left[:dot], left[dot+1:]
	}
	if ref.TypeName == "" {
		return FacetRef{}, "", fmt.Errorf("check target %q is missing a resource type", s)
	}
	return ref, relation, nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
