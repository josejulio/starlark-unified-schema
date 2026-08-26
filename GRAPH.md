# Graph JSON — canonical schema graph

This document defines the **canonical graph representation** of a Kessel schema: a
single JSON artifact that captures the schema as a graph of resource types
(nodes) connected by relations and inheritance (edges), plus the permission
rewrite logic carried on each node.

## Design goals

1. **Schema-native, never target-native.** The model is expressed only in the
   schema's own vocabulary (`resource`, `reporter`, `relation`, `cardinality`,
   `permission`, `and`/`or`/`unless`/`ref`/`subref`). It contains **zero** SpiceDB
   / KSIL / JSON-Schema concepts. SpiceDB is an implementation detail that may be
   swapped; this artifact must survive that swap.
2. **One artifact, many consumers.** This JSON is the single source of truth that
   downstream tools render or analyze — Mermaid/Graphviz for docs, cytoscape.js
   for an interactive playground, and a linter for validation. None of those
   re-walk Starlark; they all read this file.
3. **Deterministic.** Nodes and edges are sorted by `id` so output is stable
   across runs (clean diffs, golden tests).

It is produced by `GraphVisitor` (`interpreter/internal/output/graph.go`), a
`SchemaVisitor` implementation, and written to `$GRAPH_OUTPUT_DIR/graph.json`.

## Top-level shape

```json
{
  "version": "1",
  "nodes": [ /* graphNode */ ],
  "edges": [ /* graphEdge */ ]
}
```

## Nodes

A **node is one logical resource type**, keyed by its `typeName`. The same
logical type can be reported by several reporters (e.g. `workspace` by both
`rbac` and `features`); those share a single node and appear as separate entries
under `reporters`. This mirrors how the schema is authored and how the existing
processor merges facets by type name.

```json
{
  "id": "workspace",
  "kind": "resource",
  "typeName": "workspace",
  "common": {
    "dataFields": [ /* dataField */ ],
    "permissions": [ /* permission */ ]
  },
  "reporters": {
    "rbac": {
      "dataFields": [ /* dataField */ ],
      "permissions": [ /* permission */ ],
      "extends": { "typeName": "...", "reporter": "..." }
    }
  }
}
```

- `common` — members shared across every reporter of this type. Omitted when
  empty. Relations are **not** listed here; they are edges (see below).
- `reporters` — one entry per reporter facet. `extends` is present only when the
  facet extends another type (single-level inheritance).

> Note: `dataFields` are the JSON-Schema-facing part of the model. They are
> carried here so the artifact is a faithful serialization of the schema, but
> graph renderers can ignore them — they are node-internal metadata, not
> topology.

### Data fields and data types

`dataField`:

```json
{ "name": "satellite_id", "required": false, "description": "...", "type": <dataType> }
```

`dataType` is one of (matching the DSL / existing visitors):

```
{ "kind": "text", "minLength": 0, "maxLength": 255, "regex": "..." }   // attrs optional
{ "kind": "uuid" }
{ "kind": "numeric_id", "min": 0, "max": 100 }                          // attrs optional
{ "kind": "boolean" }
{ "kind": "date_time" }
{ "kind": "enum", "values": ["a", "b"] }
{ "kind": "nullable", "inner": <dataType> }
{ "kind": "composite", "types": [ <dataType>, ... ] }                   // from union()
{ "kind": "array", "items": <dataType> }
{ "kind": "object", "properties": [ <dataField>, ... ], "required": ["..."] }
```

### Permissions

A permission is a named computed relation. Its `body` is the rewrite expression
tree, stored faithfully so renderers can either collapse it to a badge or
explode it into operator nodes for a permission-rewrite view.

```json
{
  "name": "view",
  "body": <expr>
}
```

`expr` is one of:

```
{ "kind": "reference",    "name": "owner" }                 // ref: a relation/permission on this type
{ "kind": "subreference", "name": "parent", "sub": "view" } // subref: traverse `name`, then `sub` on the target
{ "kind": "or",     "left": <expr>, "right": <expr> }
{ "kind": "and",    "left": <expr>, "right": <expr> }
{ "kind": "unless", "left": <expr>, "right": <expr> }
```

## Edges

Edges are the **denormalized topology** — a flat list graph tools and lints
consume directly. Two kinds today:

### Relation edges (`kind: "relation"`)

```json
{
  "id": "workspace#reporter:rbac.parent",
  "kind": "relation",
  "source": "workspace",
  "target": "workspace",
  "name": "parent",
  "cardinality": "AtMostOne",
  "scope": "reporter",
  "sourceReporter": "rbac",
  "targetReporter": "rbac",
  "self": true
}
```

- `source` / `target` — node `id`s (logical type names).
- `scope` — `"common"` (relation defined in the shared common block; applies to
  every reporter, `sourceReporter` omitted) or `"reporter"` (`sourceReporter`
  set).
- `targetReporter` — the reporter facet the relation points at (always known;
  the processor resolves it from metadata).
- `self` — true when the relation targets its own type/reporter (authored via
  `self()` or an explicit same-type reference). Renders as a self-loop.
- `cardinality` — one of `AtMostOne`, `ExactlyOne`, `AtLeastOne`, `Many`, `All`.

### Inheritance edges (`kind: "inherits"`)

```json
{
  "id": "special_folder#reporter:test=>inherits",
  "kind": "inherits",
  "source": "special_folder",
  "target": "folder",
  "sourceReporter": "test",
  "targetReporter": "test"
}
```

Directed child → parent.

## Rendering

`make graph` produces both `graph.json` and a Mermaid structural diagram
(`graph.mmd`) under `$GRAPH_OUTPUT_DIR`. The Mermaid renderer
(`cmd/graph-mermaid`, `internal/render`) reads **only** `graph.json` — it never
touches Starlark — demonstrating the "one artifact, many consumers" design. It
The diagram is grouped like the Starlark schema — **one subgraph per resource
type**. Inside each subgraph is a `common` node (when the type has a shared
common representation) and one node per reporter. A purple dotted edge
`common -.-> reporter` shows the common representation shared into every reporter, so common relations
are drawn once (from the `common` node) rather than duplicated per reporter.
Inheritance is a dotted `extends` edge.

Cardinalities are shown as UML multiplicity (parenthesized) at the relation's
target end, with a legend listing the symbols in use (toggle with `-legend`):

| Kessel       | UML     |
| ------------ | ------- |
| `ExactlyOne` | `(1)`   |
| `AtMostOne`  | `(0..1)`|
| `AtLeastOne` | `(1..*)`|
| `Many`       | `(*)`   |

`ExactlyOne` is the implicit default: it is left off relation labels (and the
legend) entirely, so an unlabeled relation reads as "exactly one". Only the
non-default multiplicities are drawn, keeping the common case uncluttered.

Cardinalities without a specific mapping (e.g. `All`, the wildcard) fall back to
the raw Kessel name.

To embed in GitHub-rendered docs, wrap the `.mmd` contents in a ```` ```mermaid ````
fenced block.

### Interactive web view

The interactive view is the in-browser **playground** (see below). It renders with
[Cytoscape.js](https://js.cytoscape.org) using the same grouping as the Mermaid
view: one compound node per resource type, a `common` child (when the type has a
shared representation) and one child per reporter, with relation and inheritance
edges between them. The layout is swappable (default `dagre`, hierarchical/directed).

**Visual encoding.**

| Element                | Encoding                                             |
| ---------------------- | ---------------------------------------------------- |
| resource type          | compound container box (groups its facets)           |
| common representation  | purple node                                          |
| reporter facet         | blue node                                            |
| relation               | grey solid arrow, labelled `name` + UML multiplicity |
| self-relation          | orange arrow (targets its own facet)                 |
| inheritance (`extends`)| dashed green arrow (child → parent)                  |

Multiplicity follows the Mermaid rule: `(0..1)` at most one, `(1..*)` at least
one, `(*)` many; `ExactlyOne` is the implicit default and left off labels. A
legend of the encoding is shown in the page's side panel.

**Detail panel.** Clicking a node or edge opens an inspector built from data
already in `graph.json` — no second input:

- **Resource type** — its kind, reporters present, and whether it has a common
  representation.
- **Common / reporter facet** — its `extends` target (provenance), its **data
  fields** (each rendered with a recursive summary of its `dataType`, e.g.
  `uuid?`, `text(maxLength=255)`, `(uuid | text(regex=…))`, `array<…>`), and its
  **permissions** (the `and`/`or`/`unless`/`reference`/`subreference` rewrite
  trees rendered as a nested, indented view).
- **Edge** — relation metadata (name, cardinality, scope, source/target,
  reporters, self) or the inheritance source/target.

Selecting an element highlights its neighborhood (dimming the rest); the panel's
search box filters resource types by name.

### In-browser playground (WASM)

`make serve-graph-playground` builds the live playground (`cmd/graph-playground`,
`internal/web`) and serves `output/playground/`: a page that carries the Starlark
schema **source**, compiles it to the canonical graph **in the browser**, and
renders it — no server round-trips after load. The Cytoscape elements come from
the `internal/web` element view model applied to the freshly compiled graph.

The compiler is the interpreter itself, built for WebAssembly (`cmd/graph-wasm`,
`GOOS=js GOARCH=wasm`). It exposes one global function, `kesselCompile`, that runs
the **same** pipeline as the CLI: `lang.CompileGraph` (Processor + `GraphVisitor`)
produces `graph.json`, then `web.Elements` applies the one authoritative
`graph.json → elements` transform. The browser re-implements neither, so a
compile in the page is byte-identical to `cmd/interpreter` — a guarantee pinned by
a golden test (`TestCompileGraphMatchesFilesystem`).

- **Editor.** A [CodeMirror](https://codemirror.net) pane (Python mode — Starlark
  is a Python dialect) with a file switcher over the whole schema tree, including
  the `kessel.star` prelude. Edits are held in an in-memory file map; **Compile**
  (or `Ctrl`/`Cmd`+`Enter`, or a debounce after typing) sends the map to the WASM
  compiler and re-renders.
- **Graph + inspector.** The same rendering core (`render.js`) the static page
  uses, so grouping, the detail panel, search and neighbor-highlighting behave
  identically.
- **Errors.** Starlark/processor failures are returned as structured messages and
  shown inline under the editor; the last good graph stays on screen.

The page loads two sidecars at runtime — the compiled `.wasm` binary and Go's
`wasm_exec.js` (copied from the toolchain's `GOROOT` at build time, so it never
drifts) — both fetched over http, so `make serve-graph-playground` serves the
page rather than opening it as a file. The `.wasm` binary is large and
toolchain-specific; it is written to the gitignored `output/` dir and never
committed.

## Analysis

`make graph-analyze` runs graph-theory analyses over `graph.json` and reports
structural problems. Like the renderer, the analyzer (`cmd/graph-analyze`,
`internal/analyze`) reads **only** `graph.json` — never Starlark — and uses
[gonum](https://gonum.org)'s graph package as its engine, so richer algorithms
(strongly-connected components, cycle detection, shortest paths) can be layered
on later without changing the artifact.

The first analysis finds **islands** — resources disconnected from the rest of
the schema. Connectivity is computed over **relation edges only** (inheritance is
ignored), with self-relations excluded, treating edges as undirected: two types
are in the same weakly-connected component if a chain of relations links them in
either direction. The largest connected component is the reference body of the
graph; every other component is an island:

- **Isolated resources** — size-1 components: a type with no cross-type relation.
- **Disconnected clusters** — size-N components linked to each other but not the
  main graph.

Output is a text report by default (`-format json` for programmatic consumers).

## Known limitations (v1)

- **No resource `idType` or `final` on nodes.** The `SchemaVisitor` interface
  does not currently expose a resource's own id type or its `final` flag to
  `VisitResource` (only relation *target* id types are exposed). These are
  omitted rather than guessed. Add them via an interface change if needed.
- **One target per relation.** The Go processor resolves a relation to a single
  target type (`self` or `resource`); `typeUnion` / `any_of` targets are not yet
  handled in the Go path, so each relation yields exactly one edge.
- **No derived permission-dependency edges yet.** Permission bodies are stored as
  expression trees on nodes. A future layer can derive `permission` edges
  (perm → referenced relation/permission) from those trees for the
  permission-rewrite view and cycle-detection lints, without changing this
  artifact.
