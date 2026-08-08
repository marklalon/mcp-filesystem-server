package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callSearchWithinFiles invokes the tool and returns the result along with its text content.
func callSearchWithinFiles(
	t *testing.T, handler *FilesystemHandler, args map[string]any,
) (*mcp.CallToolResult, string) {
	t.Helper()

	request := mcp.CallToolRequest{}
	request.Params.Name = "search_within_files"
	request.Params.Arguments = args

	result, err := handler.HandleSearchWithinFiles(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	return result, result.Content[0].(mcp.TextContent).Text
}

// resolvedPath returns the path as the handler reports it, with symlinks resolved.
// On Windows this also expands the 8.3 short names that t.TempDir may hand back.
func resolvedPath(t *testing.T, dir string, name string) string {
	t.Helper()

	resolvedDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	return filepath.Join(resolvedDir, name)
}

func TestSearchWithinFiles_Pattern(t *testing.T) {

	// setting up test folder
	// tmpDir/
	// - main.go   ("package main", "func Alpha() {}", "func beta() {}")
	// - notes.txt ("ALPHA in caps", "alpha lower")

	dir := t.TempDir()

	mainGo := filepath.Join(dir, "main.go")
	err := os.WriteFile(mainGo, []byte("package main\nfunc Alpha() {}\nfunc beta() {}\n"), 0644)
	require.NoError(t, err)

	notesTxt := filepath.Join(dir, "notes.txt")
	err = os.WriteFile(notesTxt, []byte("ALPHA in caps\nalpha lower\n"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	// Paths as they appear in the tool output
	mainGoOut := resolvedPath(t, dir, "main.go")
	notesTxtOut := resolvedPath(t, dir, "notes.txt")

	tests := []struct {
		info       string
		substring  string
		regex      bool
		ignoreCase bool
		wantCount  int
		wantFiles  []string
	}{
		{
			info:      "literal substring is case sensitive",
			substring: "func Alpha",
			wantCount: 1,
			wantFiles: []string{mainGoOut},
		},
		{
			info:      "literal substring is not a regex",
			substring: `func\s+Alpha`,
			wantCount: 0,
		},
		{
			info:      "regex with character class",
			substring: `func\s+\w+\(\)`,
			regex:     true,
			wantCount: 2,
			wantFiles: []string{mainGoOut},
		},
		{
			info:      "regex anchors to line boundaries",
			substring: `^func`,
			regex:     true,
			wantCount: 2,
			wantFiles: []string{mainGoOut},
		},
		{
			info:      "regex cannot span lines",
			substring: "main\nfunc",
			regex:     true,
			wantCount: 0,
		},
		{
			info:      "regex without a match",
			substring: `^gamma`,
			regex:     true,
			wantCount: 0,
		},
		{
			info:       "ignore_case with a literal substring",
			substring:  "alpha",
			ignoreCase: true,
			wantCount:  3,
			wantFiles:  []string{mainGoOut, notesTxtOut},
		},
		{
			info:       "ignore_case with a regex",
			substring:  `^alpha`,
			regex:      true,
			ignoreCase: true,
			wantCount:  2,
			wantFiles:  []string{notesTxtOut},
		},
		{
			info:      "inline case-insensitive flag",
			substring: `(?i)^alpha`,
			regex:     true,
			wantCount: 2,
			wantFiles: []string{notesTxtOut},
		},
	}

	for _, test := range tests {
		t.Run(test.info, func(t *testing.T) {
			result, text := callSearchWithinFiles(t, handler, map[string]any{
				"path":        dir,
				"substring":   test.substring,
				"regex":       test.regex,
				"ignore_case": test.ignoreCase,
			})
			assert.False(t, result.IsError)

			if test.wantCount == 0 {
				assert.Contains(t, text, "No ")
				assert.Contains(t, text, "found in files under")
				return
			}

			assert.Contains(t, text, fmt.Sprintf("Found %d ", test.wantCount))
			for _, file := range test.wantFiles {
				assert.Contains(t, text, file)
			}
		})
	}
}

func TestSearchWithinFiles_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("func Alpha() {}\n"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	// The same pattern is a valid literal, so it must only fail in regex mode
	result, text := callSearchWithinFiles(t, handler, map[string]any{
		"path":      dir,
		"substring": "func Alpha(",
		"regex":     true,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, text, "Invalid regular expression")

	result, text = callSearchWithinFiles(t, handler, map[string]any{
		"path":      dir,
		"substring": "func Alpha(",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, text, "Found 1 ")
}

func TestSearchWithinFiles_EmptySubstring(t *testing.T) {
	dir := t.TempDir()
	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	result, text := callSearchWithinFiles(t, handler, map[string]any{
		"path":      dir,
		"substring": "",
		"regex":     true,
	})
	assert.True(t, result.IsError)
	assert.Contains(t, text, "substring cannot be empty")
}

func TestSearchWithinFiles_TruncatesAroundMatch(t *testing.T) {
	dir := t.TempDir()

	line := strings.Repeat("x", 200) + "NEEDLE" + strings.Repeat("y", 200)
	err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte(line+"\n"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	// The window must be centred on the match in regex mode too, where the match
	// offset cannot be recovered from the pattern itself
	for _, useRegex := range []bool{false, true} {
		t.Run(fmt.Sprintf("regex=%v", useRegex), func(t *testing.T) {
			result, text := callSearchWithinFiles(t, handler, map[string]any{
				"path":      dir,
				"substring": "NEEDLE",
				"regex":     useRegex,
			})
			assert.False(t, result.IsError)

			want := "..." + strings.Repeat("x", 30) + "NEEDLE" + strings.Repeat("y", 30) + "..."
			assert.Contains(t, text, want)
			assert.NotContains(t, text, strings.Repeat("x", 31))
		})
	}
}

func TestSearchWithinFiles_TruncatesOnRuneBoundaries(t *testing.T) {
	dir := t.TempDir()

	// Four-byte runes, so a raw 30-byte context window would cut one in half
	line := strings.Repeat("😀", 30) + "TARGET" + strings.Repeat("🎉", 30)
	err := os.WriteFile(filepath.Join(dir, "emoji.txt"), []byte(line+"\n"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	result, text := callSearchWithinFiles(t, handler, map[string]any{
		"path":      dir,
		"substring": "TARGET",
	})
	assert.False(t, result.IsError)
	assert.True(t, utf8.ValidString(text))

	// 30 bytes of context expands outwards to the enclosing rune boundaries: 8 runes
	want := "..." + strings.Repeat("😀", 8) + "TARGET" + strings.Repeat("🎉", 8) + "..."
	assert.Contains(t, text, want)
}

func TestSearchWithinFiles_RegexRespectsLimits(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(
		filepath.Join(dir, "top.txt"),
		[]byte("hit 1\nhit 2\nhit 3\nhit 4\nhit 5\n"),
		0644,
	)
	require.NoError(t, err)

	nested := filepath.Join(dir, "nested")
	err = os.MkdirAll(nested, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(nested, "nested.txt"), []byte("hit 6\n"), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	nestedTxtOut := resolvedPath(t, nested, "nested.txt")

	t.Run("max_results", func(t *testing.T) {
		result, text := callSearchWithinFiles(t, handler, map[string]any{
			"path":        dir,
			"substring":   `^hit\s\d$`,
			"regex":       true,
			"max_results": 2,
		})
		assert.False(t, result.IsError)
		assert.Contains(t, text, "Found 2 ")
		assert.Contains(t, text, "Results limited to 2 matches")
	})

	t.Run("depth", func(t *testing.T) {
		result, text := callSearchWithinFiles(t, handler, map[string]any{
			"path":      dir,
			"substring": `^hit\s\d$`,
			"regex":     true,
			"depth":     1,
		})
		assert.False(t, result.IsError)
		assert.Contains(t, text, "Found 5 ")
		assert.NotContains(t, text, nestedTxtOut)
	})
}
