package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadfile_Valid(t *testing.T) {
	// prepare temp directory
	dir := t.TempDir()
	content := "test-content"
	err := os.WriteFile(filepath.Join(dir, "test"), []byte(content), 0644)
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)
	request := mcp.CallToolRequest{}
	request.Params.Name = "read_file"
	request.Params.Arguments = map[string]any{
		"path": filepath.Join(dir, "test"),
	}

	result, err := handler.HandleReadFile(context.Background(), request)
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)
	assert.Equal(t, content, result.Content[0].(mcp.TextContent).Text)
}

func TestReadfile_Invalid(t *testing.T) {
	dir := t.TempDir()
	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Name = "read_file"
	request.Params.Arguments = map[string]any{
		"path": filepath.Join(dir, "test"),
	}

	result, err := handler.HandleReadFile(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)

	// The error wording differs across platforms: "no such file or directory"
	// on Unix vs "cannot find the file specified" on Windows.
	text := fmt.Sprint(result.Content[0])
	lower := strings.ToLower(text)
	assert.True(t,
		strings.Contains(lower, "no such file") ||
			strings.Contains(lower, "cannot find the file"),
		"unexpected error message: %s", text,
	)
}

func TestReadfile_LineRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0644))

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	tests := []struct {
		name     string
		args     map[string]any
		expected string
	}{
		{
			name:     "start and end",
			args:     map[string]any{"start_line": 2, "end_line": 4},
			expected: "two\nthree\nfour\n",
		},
		{
			name:     "start only reads to end of file",
			args:     map[string]any{"start_line": 4},
			expected: "four\nfive\n",
		},
		{
			name:     "end only reads from first line",
			args:     map[string]any{"end_line": 2},
			expected: "one\ntwo\n",
		},
		{
			name:     "single line",
			args:     map[string]any{"start_line": 3, "end_line": 3},
			expected: "three\n",
		},
		{
			name:     "end beyond file is clamped",
			args:     map[string]any{"start_line": 5, "end_line": 100},
			expected: "five\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Name = "read_file"
			request.Params.Arguments = map[string]any{"path": path}
			for k, v := range tt.args {
				request.Params.Arguments.(map[string]any)[k] = v
			}

			result, err := handler.HandleReadFile(context.Background(), request)
			require.NoError(t, err)
			require.False(t, result.IsError, "unexpected error: %v", result.Content)
			require.Len(t, result.Content, 1)
			assert.Equal(t, tt.expected, result.Content[0].(mcp.TextContent).Text)
		})
	}
}

func TestReadfile_LineRange_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("one\ntwo\nthree"), 0644))

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Name = "read_file"
	request.Params.Arguments = map[string]any{
		"path":       path,
		"start_line": 2,
	}

	result, err := handler.HandleReadFile(context.Background(), request)
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, "two\nthree", result.Content[0].(mcp.TextContent).Text)
}

func TestReadfile_LineRange_LongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	longLine := strings.Repeat("x", 128*1024)
	require.NoError(t, os.WriteFile(path, []byte("first\n"+longLine+"\nlast\n"), 0644))

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Name = "read_file"
	request.Params.Arguments = map[string]any{
		"path":       path,
		"start_line": 3,
		"end_line":   3,
	}

	// A line longer than the usual scanner buffer must not cut the read short
	result, err := handler.HandleReadFile(context.Background(), request)
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, "last\n", result.Content[0].(mcp.TextContent).Text)
}

func TestReadfile_LineRange_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("one\ntwo\n"), 0644))

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir))
	require.NoError(t, err)

	tests := []struct {
		name     string
		args     map[string]any
		contains string
	}{
		{
			name:     "start_line below one",
			args:     map[string]any{"start_line": 0},
			contains: "start_line must be 1 or greater",
		},
		{
			name:     "end_line before start_line",
			args:     map[string]any{"start_line": 3, "end_line": 2},
			contains: "must be greater than or equal to start_line",
		},
		{
			name:     "start_line past end of file",
			args:     map[string]any{"start_line": 10},
			contains: "is past the end of the file, which has 2 lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Name = "read_file"
			request.Params.Arguments = map[string]any{"path": path}
			for k, v := range tt.args {
				request.Params.Arguments.(map[string]any)[k] = v
			}

			result, err := handler.HandleReadFile(context.Background(), request)
			require.NoError(t, err)
			require.True(t, result.IsError)
			assert.Contains(t, fmt.Sprint(result.Content[0]), tt.contains)
		})
	}
}

func TestReadfile_NoAccess(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	handler, err := NewFilesystemHandler(resolveAllowedDirs(t, dir1))
	require.NoError(t, err)

	request := mcp.CallToolRequest{}
	request.Params.Name = "read_file"
	request.Params.Arguments = map[string]any{
		"path": filepath.Join(dir2, "test"),
	}

	result, err := handler.HandleReadFile(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, fmt.Sprint(result.Content[0]), "access denied - path outside allowed directories")
}
