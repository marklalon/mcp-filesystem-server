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

// Resource URIs reported by the tools carry a path relative to the root
// directory, so they have to be readable back through the resource handler.
func TestReadResource_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("hello"), 0644))

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	// The URI a tool would hand back for that file
	uri := handler.pathToResourceURI(filepath.Join(handler.rootDir, "sub", "file.txt"))
	assert.Equal(t, "file://sub/file.txt", uri)

	request := mcp.ReadResourceRequest{}
	request.Params.URI = uri

	contents, err := handler.HandleReadResource(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, contents, 1)
	assert.Equal(t, "hello", contents[0].(mcp.TextResourceContents).Text)
}

func TestReadResource_RootDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644))

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	// The root directory is addressed as "."
	assert.Equal(t, "file://.", handler.pathToResourceURI(handler.rootDir))

	request := mcp.ReadResourceRequest{}
	request.Params.URI = "file://."

	contents, err := handler.HandleReadResource(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, contents, 1)

	text := contents[0].(mcp.TextResourceContents).Text
	assert.Contains(t, text, "Directory listing for: .\n")
	assert.Contains(t, text, "[FILE] file.txt (file://file.txt)")
	assert.NotContains(t, text, dir)
}

func TestReadResource_OutsideRoot(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	request := mcp.ReadResourceRequest{}
	request.Params.URI = "file://" + filepath.ToSlash(filepath.Join(other, "file.txt"))

	_, err = handler.HandleReadResource(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}
