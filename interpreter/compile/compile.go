package compile

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/project-kessel/starlark-unified-schema/internal/lang"
)

// Compile parses the given in-memory schema files (path -> source) and drives
// the supplied visitor with the parsed schema definitions.
//
// The files map must include the kessel.star prelude, as schema modules load it
// via load("kessel.star", ...). Modules are processed in sorted path order for
// deterministic output.
//
// This function touches no filesystem and runs unchanged under GOOS=js/wasm,
// making it suitable for browser-based schema processing.
//
// Returns an error if parsing fails or if the visitor returns an error during
// processing.
func Compile(files map[string][]byte, visitor SchemaVisitor) error {
	// No real schema path: every source file is supplied by the caller.
	reader := lang.NewInMemorySourceFileReader("schema", "")

	names := make([]string, 0, len(files))
	for name, contents := range files {
		if err := reader.AddFile(name, contents); err != nil {
			return fmt.Errorf("adding %s: %w", name, err)
		}
		if filepath.Ext(name) == ".star" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	loader := lang.NewLoaderForReader("schema", reader)
	processor := lang.NewProcessor(loader)

	// Adapt the public visitor to the internal interface.
	return processor.Process(&visitorAdapter{visitor: visitor}, names...)
}
