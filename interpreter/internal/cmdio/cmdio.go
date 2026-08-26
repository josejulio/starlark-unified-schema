// Package cmdio holds the tiny input/output scaffolding shared by the graph
// command-line tools: read from a path or stdin, write to a path or stdout. The
// convention is uniform across the tools — an empty path means "use the standard
// stream" — so it lives in one place rather than being copied per command.
package cmdio

import (
	"fmt"
	"io"
	"os"
)

// Read returns the contents of the file at path, or all of stdin when path is
// empty.
func Read(path string) ([]byte, error) {
	if path == "" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, nil
}

// Write sends data to the file at path, or to stdout when path is empty. When it
// writes a file it notes the destination on stderr, so stdout stays clean for
// piping.
func Write(path string, data []byte) error {
	if path == "" {
		fmt.Print(string(data))
		return nil
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	return nil
}
