.PHONY: build-interpreter build-interpreter-debug build-schema build-shipped-schema package-release clean test build-graph-renderer graph build-graph-analyzer graph-analyze build-graph-playground build-graph-wasm graph-playground serve-graph-playground

GRAPH_WEB_PORT ?= 8000

# Go ships wasm_exec.js in lib/wasm (>= 1.24) or misc/wasm (older). Resolve once.
WASM_EXEC := $(shell if [ -f "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ]; then echo "$$(go env GOROOT)/lib/wasm/wasm_exec.js"; else echo "$$(go env GOROOT)/misc/wasm/wasm_exec.js"; fi)

# Features-only delivery allowlist. HBI/RBAC stay in schema/ for full
# build-schema / build-test coverage, but are not shipped to consumers yet —
# those namespaces remain hand-authored downstream. Drop this list (full walk)
# once those services onboard.
SHIPPED_SCHEMA_FILES ?= \
	service/reporters/features/service.star \
	billing_account/reporters/features/billing_account.star \
	workspace/reporters/features/workspace.star

# KSIL files included in ksl.tar.gz. Do not ship hbi.json / rbac.json until
# those namespaces migrate off hand-authored .ksl in rbac-config.
KSL_RELEASE_FILES ?= features.json

JSONSCHEMA_OUTPUT_DIR ?= output/jsonschema
KSL_OUTPUT_DIR ?= output/ksl
GRAPH_OUTPUT_DIR ?= output/graph
GRAPH_PLAYGROUND_DIR ?= output/playground

build-interpreter:
	go build -C ./interpreter/ -o ../bin/interpreter ./cmd/interpreter

build-interpreter-debug:
	go build -C ./interpreter/ -o ../bin/interpreter -gcflags="all=-N -l" ./cmd/interpreter

test:
	go test -C ./interpreter/ -count=1 ./...

build-schema: build-interpreter
	dotenv -f .env run ./bin/interpreter

# Build only the artifacts that are delivered to consumers.
build-shipped-schema: build-interpreter
	rm -rf "$(JSONSCHEMA_OUTPUT_DIR)" "$(KSL_OUTPUT_DIR)"
	JSONSCHEMA_OUTPUT_DIR="$(JSONSCHEMA_OUTPUT_DIR)" KSL_OUTPUT_DIR="$(KSL_OUTPUT_DIR)" \
		./bin/interpreter $(SHIPPED_SCHEMA_FILES)

# Package release tarballs in the repo root (gitignored).
package-release: build-shipped-schema
	@test -s "$(KSL_OUTPUT_DIR)/features.json" || { \
		echo "error: missing or empty $(KSL_OUTPUT_DIR)/features.json" >&2; \
		exit 1; \
	}
	tar czf jsonschema.tar.gz -C "$(JSONSCHEMA_OUTPUT_DIR)" .
	tar czf ksl.tar.gz -C "$(KSL_OUTPUT_DIR)" $(KSL_RELEASE_FILES)

build-graph-renderer:
	go build -C ./interpreter/ -o ../bin/graph-mermaid ./cmd/graph-mermaid

build-graph-analyzer:
	go build -C ./interpreter/ -o ../bin/graph-analyze ./cmd/graph-analyze

# Produce the canonical graph.json and render it to Mermaid (output/graph/).
graph: build-interpreter build-graph-renderer
	GRAPH_OUTPUT_DIR="$(GRAPH_OUTPUT_DIR)" ./bin/interpreter
	./bin/graph-mermaid -in "$(GRAPH_OUTPUT_DIR)/graph.json" -out "$(GRAPH_OUTPUT_DIR)/graph.mmd"

# Analyze graph.json for structural problems (islands / isolated resources).
graph-analyze: build-interpreter build-graph-analyzer
	GRAPH_OUTPUT_DIR="$(GRAPH_OUTPUT_DIR)" ./bin/interpreter
	./bin/graph-analyze -in "$(GRAPH_OUTPUT_DIR)/graph.json"

build-graph-playground:
	go build -C ./interpreter/ -o ../bin/graph-playground ./cmd/graph-playground

# Build the in-browser schema compiler (Go -> WASM). The binary is large and
# toolchain-specific, so it lands in the gitignored output dir, never in git.
build-graph-wasm:
	mkdir -p "$(GRAPH_PLAYGROUND_DIR)"
	GOOS=js GOARCH=wasm go build -C ./interpreter/ -o "../$(GRAPH_PLAYGROUND_DIR)/graph-playground.wasm" ./cmd/graph-wasm

# Assemble the live playground site (schema source + WASM compiler +
# wasm_exec.js) into $(GRAPH_PLAYGROUND_DIR). This is the deployable artifact —
# the CI Pages workflow uploads exactly this directory. Building it is separate
# from serving so both the workflow and serve-graph-playground share one recipe.
graph-playground: build-graph-playground build-graph-wasm
	cp "$(WASM_EXEC)" "$(GRAPH_PLAYGROUND_DIR)/wasm_exec.js"
	./bin/graph-playground -src schema -out "$(GRAPH_PLAYGROUND_DIR)/index.html"

# Serve the built playground over http. The page compiles Starlark to the graph
# entirely in the browser; the .wasm sidecar must be fetched (blocked over
# file://), so this must be served over http rather than opened as a file.
serve-graph-playground: graph-playground
	@echo "Serving $(GRAPH_PLAYGROUND_DIR) at http://localhost:$(GRAPH_WEB_PORT)/ (Ctrl-C to stop)"
	cd "$(GRAPH_PLAYGROUND_DIR)" && python3 -m http.server $(GRAPH_WEB_PORT)

clean:
	rm -rf bin/
	rm -rf output/
	rm -f jsonschema.tar.gz ksl.tar.gz
