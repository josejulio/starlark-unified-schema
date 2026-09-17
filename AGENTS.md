For how to...
- set up, build, compile schemas, and promote artifacts across repos, see [README.md](README.md)
- understand the compiler, outputs, and schema layout, see [ARCHITECTURE.md](ARCHITECTURE.md)
- write and run Go tests, see [interpreter/TESTING.md](interpreter/TESTING.md) (subtree `TESTING.md` if present)
- use the schema DSL, see [schema/kessel.star](schema/kessel.star); follow patterns in [schema/](schema/)

Repo rules:
- commit Starlark and interpreter source only — not `bin/`, `output/`, or generated JSON Schema/KSIL
- `common_representation.star` is a plain dict, not `resource()`
- `references/` is obsolete — do not treat as source of truth
- run `make test` after interpreter changes; run `make build-schema` after `.star` or output changes
- **`interpreter/compile` is a public API** consumed by external tooling (e.g., `project-kessel/schema-graph-tools`); changes to `SchemaVisitor` interface (new DSL nodes, data types, permission expressions) are breaking changes that require coordination with consumers
- update README / ARCHITECTURE / relevant TESTING.md in the same PR when your change makes them wrong — do not copy their content here
- only git commit when the user asks
