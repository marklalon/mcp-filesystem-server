package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFiles_Pattern(t *testing.T) {

	// setting up test folder
	// tmpDir/
	// - foo/
	//   - bar.h
	//   - test.c
	// - test.h
	// - test.c

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "test.h"), []byte("foo"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "test.c"), []byte("foo"), 0644)
	require.NoError(t, err)

	fooDir := filepath.Join(dir, "foo")
	err = os.MkdirAll(fooDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(fooDir, "bar.h"), []byte("foo"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(fooDir, "test.c"), []byte("foo"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	// Matches are reported relative to the root directory, with forward slashes
	tests := []struct {
		info    string
		pattern string
		matches []string
	}{
		{info: "use placeholder with extension", pattern: "*.h", matches: []string{"test.h", "foo/bar.h"}},
		{info: "use placeholder with name", pattern: "test.*", matches: []string{"test.h", "test.c"}},
		{info: "same filename", pattern: "test.c", matches: []string{"test.c", "foo/test.c"}},
	}

	// The search path is given relative to the root directory
	for _, searchPath := range []string{".", dir} {
		for _, test := range tests {
			t.Run(test.info, func(t *testing.T) {
				request := mcp.CallToolRequest{}
				request.Params.Name = "search_files"
				request.Params.Arguments = map[string]any{
					"path":    searchPath,
					"pattern": test.pattern,
				}

				result, err := handler.HandleSearchFiles(context.Background(), request)
				require.NoError(t, err)
				assert.False(t, result.IsError)
				assert.Len(t, result.Content, 1)

				text := result.Content[0].(mcp.TextContent).Text
				assert.NotContains(t, text, dir, "absolute paths must not leak into results")
				for _, match := range test.matches {
					assert.Contains(t, text, match)
				}
			})
		}
	}
}
