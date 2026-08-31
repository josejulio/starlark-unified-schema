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

## Notation convention: REPORTER/TYPE

Throughout this document and the CLI tools, **facets** (a reporter's view of a
resource type) are written as `REPORTER/TYPE` — for example, `features/workspace`
or `rbac/workspace`. This mirrors how the schema is organized: the **reporter**
(service provider like `rbac`, `features`, or `hbi`) owns the type definition
(resource like `workspace`, `host`, or `billing_account`).

- **Reporter** (left of `/`) — the service that provides this facet, e.g.,
  `features`, `rbac`, `hbi`. This maps to the Starlark `reporter="..."` attribute.
- **Type** (right of `/`) — the resource type, e.g., `workspace`, `host`,
  `billing_account`. This maps to the Starlark `resource(...)` call.

**Examples:**
- `features/workspace` — the `features` reporter's facet of the `workspace` type
- `rbac/workspace` — the `rbac` reporter's facet of the `workspace` type
- `hbi/host` — the `hbi` reporter's facet of the `host` type

**When a type has only one reporter**, the reporter can be omitted in commands
(e.g., `workspace#view` if `workspace` has only one reporter), but the full
`REPORTER/TYPE` form is always accepted and recommended for clarity.

**In check and reach commands**, targets use this convention:
- Check cost: `REPORTER/TYPE#RELATION` (e.g., `features/workspace#enabled_services`)
- Reachability: `REPORTER/TYPE#RELATION@REPORTER/TYPE` (e.g.,
  `features/workspace#direct_billing_account@features/billing_account`)

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

| Element                              | Encoding                                             |
| ------------------------------------ | ---------------------------------------------------- |
| resource type                        | compound container box (groups its facets)           |
| common representation                | purple node                                          |
| reporter facet                       | blue node                                            |
| reporter facet (playground only)     | bordered by worst permission cost: green (O(1)), amber (O(D) recursion), red (O(N·…) fan-out) |
| relation                             | grey solid arrow, labelled `name` + UML multiplicity |
| self-relation                        | orange arrow (targets its own facet)                 |
| inheritance (`extends`)              | dashed green arrow (child → parent)                  |
| permission overlay (playground only) | when a permission is clicked, affected edges are coloured: amber-dashed (recursion hop), red-thick (fan-out hop) |

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
  trees rendered as a nested, indented view). On a **reporter** facet each
  permission name also carries a **read-cost chip** — the same symbolic `bigO`
  the analyzer computes (see *Check cost* below), colour-coded by its dominant
  driver (constant-time, hierarchy/recursion walk, or fan-out over a
  many-relation) — so the cost of a rewrite is glanceable without leaving the
  graph. The chip is present only in the playground, where a compiler is loaded;
  the static page renders permissions without it. Clicking a permission in the
  playground also highlights the relation edges its rewrite traverses,
  cost-coloured by their role (amber-dashed for recursion hops, red-thick for
  fan-out hops).
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

### Check cost

`graph-analyze -check REPORTER/TYPE#RELATION` (or `TYPE#RELATION` when the type
has only one reporter) explains the **read cost of a single check** — the
`object#relation@subject` query that inventory-api's
[`CheckRequest`](https://buf.build/project-kessel/inventory-api/docs/main:kessel.inventory.v1beta2#kessel.inventory.v1beta2.CheckRequest)
carries (`object` = a reporter facet, `relation` = one named permission-or-relation,
`subject` = a subject or subject-set). It walks the permission rewrite from the
object down the relation topology and returns a **proof tree** annotated with cost:

```
$ graph-analyze -in graph.json -check features/workspace#enabled_services
Cost:      O(D_workspace)
Depth:     1 sequential hop(s)   Fan-out sites: 0   Recursive: true

permission "enabled_services" on features/workspace    O(D_workspace)
└─ AND    O(D_workspace)
   ├─ permission "_paid_services" on features/workspace    O(D_workspace)
   │  └─ OR    O(D_workspace)
   │     ├─ direct_billing_account (0..1) → features/billing_account ⇒ services    O(1)
   │     │  └─ relation "services" (*)    O(1)
   │     └─ parent (0..1) → features/workspace ⇒ _paid_services    O(D_workspace)
   │        └─ ↺ _paid_services on features/workspace (recursion)    O(1)
   └─ permission "_desired_services" on features/workspace    O(D_workspace)
      └─ …
```

Why cost, not just correctness: the load-bearing hard part of schema design is
coming up with computed-permission rules that **read** well — e.g. weighing a
bidirectional relation against inverting a relation's direction. That is largely a
**static property of the graph**: a check dispatches sub-checks along the arrows of
the rewrite, so its shape and asymptotics follow from topology + `cardinality` +
the rewrite trees alone, with **no instance data**.

**The cost model.** Each rewrite construct maps to a cost, composed bottom-up:

| Construct                                   | Cost                                             |
| ------------------------------------------- | ------------------------------------------------ |
| direct relation (`reference`)               | `O(1)` — one indexed membership check            |
| `or` / `and` / `unless`                     | sum of both operands (worst case)                |
| `subreference` over a single-target edge    | one hop: `O(1 + sub)`                            |
| `subreference` over a `many`/`≥1`/`All` edge | fan-out: `O(N_edge · sub)`                       |
| `subreference` that re-enters a permission  | recursion: `O(D_type)` (tree), `O(reach(type))` if it fans out |

Because a check's true cost is **data-dependent**, the headline `bigO` is symbolic
in named variables (`D_workspace` = hierarchy depth, `N_parent` = per-relation
fan-out) that only real tuple counts can fix to a number. Two fully-static scalars
accompany it on every node: **`dispatchDepth`** (sequential arrow hops → a latency
proxy; recursion counted once, flagged by `recursive` + the depth variable) and
**`fanoutSites`** (arrows over many-cardinality relations → a work proxy). Together
they make design alternatives **sortable at a glance** while staying honest that
absolute latency needs a real resolver.

**Resolution.** Names resolve in a facet's scope — its own members, its type's
`common` members, and everything inherited from facets it `extends` (own wins on
clashes), mirroring the web highlighter. A `subreference`'s downstream name is
resolved on the *target type*, preferring the edge's target reporter but falling
back to the type's other facets — so `parent` (which targets `workspace.rbac`) still
finds `_paid_services` on `workspace.features`, which is what makes the recursion
visible. This is a pure `graph.json` consumer in `internal/analyze` (see
`ExplainCheck`); a golden test compiles the committed schema and pins the model.

The **same** analysis runs in the browser: `cmd/graph-wasm` exports a
`kesselExplainCheck(graph, "TYPE[.REPORTER]#RELATION")` global that calls
`web.ExplainCheck` → `analyze.ExplainCheck`, so a check explained in the
playground is byte-identical to `graph-analyze -check -format json`. The
playground installs it as a cost provider after each compile and the inspector
uses it to draw the per-permission cost chips described above.

What it is **not**: it ranks structural alternatives and flags red flags
(fan-out, hierarchy walks); it does **not** predict production latency (that needs
real tuple counts, cache behavior, and the request's `Consistency` mode), and it
does not yet author a cheaper rewrite for you.

The playground **visualises** this same cost model on the graph itself: each
reporter facet is bordered by the worst permission cost it carries (green/amber/red
heatmap), and clicking a permission overlays its proof-tree edges with cost-coloured
roles (recursion/fan-out), so the shape and expense of a rewrite are immediately
visible.

### Check reachability

`graph-analyze -reach REPORTER/TYPE#RELATION@REPORTER/TYPE` verifies **structural
reachability** — whether the schema wiring needed to satisfy
`object#relation@subject` exists. It walks the permission rewrite from
`object#relation` down the relation topology (via the same proof tree
`ExplainCheck` builds) and reports whether at least one path terminates at a
relation whose target is `subject`:

```
$ graph-analyze -in graph.json -reach features/workspace#direct_billing_account@features/billing_account
✓ reachable

Grant path(s): 1
  Path 1:
    features/workspace
      --direct_billing_account 0..1--> features/billing_account
```

This is a **purely static property** of the schema graph + rewrite trees — it uses
no instance data and no IDs. We are not replicating the resolver's evaluation; we
are verifying that the structure required to complete the query successfully is
present. **"No path exists" is a first-class answer** — that is how a schema
author learns the schema does not support an intended check.

**Subject granularity = full facet.** A leaf matches only when **both** the target
type **and** the target reporter equal the subject facet. Relation-edge targets
always carry a resolved `TargetReporter` in `graph.json`, so even subject-only
types like `user` appear as `<reporter>/user` facets and must be matched exactly.

**Traverse all branches; tag exclusions.** The analysis walks both operands of
`and`, `or`, **and `unless`** — we are verifying that wiring *exists*, not
evaluating truth. A witness path that reaches `subject` only by descending into
the **right operand of an `unless`** (the exclusion/deny side) is recorded but
**flagged as an exclusion path**. The wiring is present, but reaching the subject
there means it would be *denied*, not granted.

**Verdict tiers** derived from the witnesses:

| Verdict          | Condition                                                      |
| ---------------- | ------------------------------------------------------------- |
| `reachable`      | ≥1 witness path with `Excluded == false` (a real grant path) |
| `exclusion-only` | no grant witness, but ≥1 witness with `Excluded == true`      |
| `unreachable`    | no witness path at all                                        |

**Recursion is sound to treat as a dead-end for witnesses.** `ExplainCheck` cuts a
rewrite cycle with a `recursive` sentinel node keyed by `facet#name`. A
`recursive` node re-enters a permission **already expanded higher on the path**,
so it introduces **no new subject facet** — every subject facet reachable through
the cycle was already discovered before the cut (the guard only fires on a true
`facet#name` repeat; distinct facets in a hierarchy are expanded until they
repeat). Therefore, for witness extraction, a `recursive` node emits **no
terminal**.

**CLI and WASM parity.** The **same** analysis runs in the browser:
`cmd/graph-wasm` exports a `kesselCheckReach(graph, "REPORTER/TYPE#RELATION@REPORTER/TYPE")`
global that calls `web.CheckReachable` → `analyze.CheckReachable`, so a reach
check in the playground is byte-identical to `graph-analyze -reach -format json`.
The playground installs it as a reach provider after each compile and the
**Check reachability** panel uses it to verify and highlight paths on the graph.

**Graph encoding.** Witness paths are highlighted on the graph:

- **`.reach-path`** (green, solid) — an edge on at least one grant (non-excluded)
  witness.
- **`.reach-exclusion`** (amber, dashed) — an edge appearing **only** on exclusion
  witnesses.

The rest of the graph is dimmed. Inheritance edges from the object facet remain
visible (as with permission highlighting) so borrowed relations read as connected.

**Limitations inherited from the cost model:**

- **One-target-per-relation**: a `typeUnion` target is not expanded, so paths
  through it are missed.
- **Subject-set expansion treated as a leaf match**: the subject is matched against
  the target of a relation leaf, not expanded further (this is a structural
  property; subject-set expansion is instance-dependent).

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
- **Check cost is structural, not measured.** `-check` computes worst-case *shape*
  from the graph (fan-out, hierarchy depth, hops) with symbolic variables for
  data-dependent quantities. It does not consume tuple counts, cache behavior, or
  the request's `Consistency` mode, so it ranks alternatives rather than predicting
  latency. It also inherits the one-target-per-relation limit above (a `typeUnion`
  target would multiply fan-out) and treats the subject side as a leaf match (no
  subject-set expansion), since neither changes the walk's structure.
