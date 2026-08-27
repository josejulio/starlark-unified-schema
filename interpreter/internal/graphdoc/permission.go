package graphdoc

import (
	"encoding/json"
	"fmt"
)

// Permission is a named computed relation with its rewrite tree, parsed from a
// facet's (or common block's) raw permission list. graphdoc keeps permission
// bodies as raw JSON by default (see Members) so pass-through consumers stay
// cheap; ParsePermissions is the opt-in decode for consumers that walk the
// rewrite — the check-cost analyzer (internal/analyze) is the first.
type Permission struct {
	Name string
	Body Expr
}

// Expr is one node of a permission rewrite tree (see GRAPH.md). Kind is one of
// "reference", "subreference", "or", "and", "unless". For a reference, Name is
// set; for a subreference, Name and Sub; for the boolean operators, Left and
// Right. A zero Expr (Kind == "") denotes an absent/empty body.
type Expr struct {
	Kind  string
	Name  string
	Sub   string
	Left  *Expr
	Right *Expr
}

// ParsePermissions decodes a facet's raw permission list into typed Permissions.
func ParsePermissions(raw []json.RawMessage) ([]Permission, error) {
	perms := make([]Permission, 0, len(raw))
	for _, r := range raw {
		var p struct {
			Name string          `json:"name"`
			Body json.RawMessage `json:"body"`
		}
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("parsing permission: %w", err)
		}
		body, err := parseExpr(p.Body)
		if err != nil {
			return nil, fmt.Errorf("parsing permission %q: %w", p.Name, err)
		}
		perms = append(perms, Permission{Name: p.Name, Body: body})
	}
	return perms, nil
}

// parseExpr recursively decodes a rewrite expression, descending into the operands
// of the boolean operators.
func parseExpr(raw json.RawMessage) (Expr, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Expr{}, nil
	}
	var e struct {
		Kind  string          `json:"kind"`
		Name  string          `json:"name"`
		Sub   string          `json:"sub"`
		Left  json.RawMessage `json:"left"`
		Right json.RawMessage `json:"right"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return Expr{}, err
	}
	expr := Expr{Kind: e.Kind, Name: e.Name, Sub: e.Sub}
	switch e.Kind {
	case "and", "or", "unless":
		left, err := parseExpr(e.Left)
		if err != nil {
			return Expr{}, err
		}
		right, err := parseExpr(e.Right)
		if err != nil {
			return Expr{}, err
		}
		expr.Left = &left
		expr.Right = &right
	}
	return expr, nil
}
