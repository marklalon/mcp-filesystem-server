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

func TestHandleGetFileInfo(t *testing.T) {
	// Setup a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a handler rooted at the temp dir
	fsHandler, err := NewFilesystemHandler(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("get file info for a file", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "test_file.txt")
		fileContent := "Hello, world!"
		err := os.WriteFile(filePath, []byte(fileContent), 0644)
		require.NoError(t, err)

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": filePath,
				},
			},
		}

		res, err := fsHandler.HandleGetFileInfo(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		// Verify the response contains file information
		require.Len(t, res.Content, 2)
		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "File information for: test_file.txt\n")
		assert.NotContains(t, textContent.Text, tmpDir)
		assert.Contains(t, textContent.Text, "IsFile: true")
		assert.Contains(t, textContent.Text, "IsDirectory: false")
		assert.Contains(t, textContent.Text, "Size: 13 bytes") // Length of "Hello, world!"
	})

	t.Run("get file info for a directory by relative path", func(t *testing.T) {
		err := os.Mkdir(filepath.Join(tmpDir, "test_directory"), 0755)
		require.NoError(t, err)

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": "test_directory",
				},
			},
		}

		res, err := fsHandler.HandleGetFileInfo(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		// Verify the response contains directory information
		require.Len(t, res.Content, 2)
		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "File information for: test_directory\n")
		assert.Contains(t, textContent.Text, "Resource URI: file://test_directory")
		assert.Contains(t, textContent.Text, "IsFile: false")
		assert.Contains(t, textContent.Text, "IsDirectory: true")
		assert.Contains(t, textContent.Text, "MIME Type: directory")
	})

	t.Run("file does not exist", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "non_existent_file.txt")

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": nonExistentPath,
				},
			},
		}

		res, err := fsHandler.HandleGetFileInfo(ctx, req)
		require.NoError(t, err)
		require.True(t, res.IsError)
	})

	t.Run("path is in a non-allowed directory", func(t *testing.T) {
		otherDir := t.TempDir()

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": filepath.Join(otherDir, "some_file.txt"),
				},
			},
		}

		res, err := fsHandler.HandleGetFileInfo(ctx, req)
		require.NoError(t, err)
		require.True(t, res.IsError)
	})
}
