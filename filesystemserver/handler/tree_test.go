package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTree extracts the JSON tree from a tree tool response.
func parseTree(t *testing.T, text string) *FileNode {
	t.Helper()

	start := strings.Index(text, "{")
	require.GreaterOrEqual(t, start, 0, "response contains no JSON")

	var tree FileNode
	require.NoError(t, json.Unmarshal([]byte(text[start:]), &tree))
	return &tree
}

func childNames(node *FileNode) []string {
	names := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		names = append(names, child.Name)
	}
	return names
}

func TestHandleTree(t *testing.T) {
	// Setup a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a handler rooted at the temp dir
	fsHandler, err := NewFilesystemHandler(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Create test directory structure
	// /tmpDir/
	//   ├── file1.txt
	//   ├── subdir1/
	//   │   ├── file2.txt
	//   │   └── subdir2/
	//   │       └── file3.txt
	//   └── emptydir/

	file1Path := filepath.Join(tmpDir, "file1.txt")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)

	subdir1Path := filepath.Join(tmpDir, "subdir1")
	err = os.Mkdir(subdir1Path, 0755)
	require.NoError(t, err)

	file2Path := filepath.Join(subdir1Path, "file2.txt")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)

	subdir2Path := filepath.Join(subdir1Path, "subdir2")
	err = os.Mkdir(subdir2Path, 0755)
	require.NoError(t, err)

	file3Path := filepath.Join(subdir2Path, "file3.txt")
	err = os.WriteFile(file3Path, []byte("content3"), 0644)
	require.NoError(t, err)

	emptydirPath := filepath.Join(tmpDir, "emptydir")
	err = os.Mkdir(emptydirPath, 0755)
	require.NoError(t, err)

	t.Run("tree with default depth", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": tmpDir,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		// Verify the response contains tree structure
		require.Len(t, res.Content, 2)
		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "Directory tree for")
		// The default depth is 2, so the tree is a directory skeleton
		assert.Contains(t, textContent.Text, "max depth: 2")

		lines := textContent.Text
		assert.Contains(t, lines, "subdir1")
		assert.Contains(t, lines, "subdir2")
		assert.Contains(t, lines, "emptydir")
		assert.NotContains(t, lines, "file1.txt")
		assert.NotContains(t, lines, "file2.txt")
		assert.NotContains(t, lines, "file3.txt")

		// Verify embedded resource
		embeddedResource := res.Content[1].(mcp.EmbeddedResource)
		assert.Equal(t, "resource", embeddedResource.Type)
		assert.Equal(t, "application/json", embeddedResource.Resource.(mcp.TextResourceContents).MIMEType)
	})

	t.Run("tree with depth 1 lists files", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path":  tmpDir,
					"depth": 1.0, // Only show immediate children
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "max depth: 1")

		tree := parseTree(t, textContent.Text)
		assert.Nil(t, tree.FileCount, "file_count should be absent when files are listed")
		assert.ElementsMatch(t,
			[]string{"file1.txt", "subdir1", "emptydir"},
			childNames(tree),
		)

		// The subdirectories sit at the max depth, so their own files are not
		// listed and show up as counts instead.
		for _, child := range tree.Children {
			switch child.Name {
			case "subdir1":
				require.NotNil(t, child.FileCount)
				assert.Equal(t, 1, *child.FileCount, "subdir1 holds file2.txt")
				assert.Empty(t, child.Children)
			case "emptydir":
				require.NotNil(t, child.FileCount)
				assert.Equal(t, 0, *child.FileCount)
			case "file1.txt":
				assert.Equal(t, "file", child.Type)
				assert.Nil(t, child.FileCount)
			}
		}
	})

	t.Run("tree deeper than 1 omits files and reports counts", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path":  tmpDir,
					"depth": 2.0, // Only go 2 levels deep
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "max depth: 2")
		assert.Contains(t, textContent.Text, "files omitted")

		// No file is listed anywhere in the tree
		assert.NotContains(t, textContent.Text, "file1.txt")
		assert.NotContains(t, textContent.Text, "file2.txt")
		assert.NotContains(t, textContent.Text, "file3.txt")

		tree := parseTree(t, textContent.Text)
		require.NotNil(t, tree.FileCount)
		assert.Equal(t, 1, *tree.FileCount, "root holds file1.txt")
		assert.ElementsMatch(t, []string{"subdir1", "emptydir"}, childNames(tree))

		for _, child := range tree.Children {
			require.NotNil(t, child.FileCount, "child %s should carry a file count", child.Name)
			switch child.Name {
			case "subdir1":
				assert.Equal(t, 1, *child.FileCount, "subdir1 holds file2.txt")
				require.Len(t, child.Children, 1)

				// subdir2 sits at the max depth: its subdirectories are not
				// explored, but its files are still counted.
				subdir2 := child.Children[0]
				assert.Equal(t, "subdir2", subdir2.Name)
				assert.Empty(t, subdir2.Children)
				require.NotNil(t, subdir2.FileCount)
				assert.Equal(t, 1, *subdir2.FileCount, "subdir2 holds file3.txt")
			case "emptydir":
				assert.Equal(t, 0, *child.FileCount)
				assert.Empty(t, child.Children)
			}
		}
	})

	t.Run("deepest expanded directory still reports its file count", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path":  tmpDir,
					"depth": 3.0,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		tree := parseTree(t, res.Content[0].(mcp.TextContent).Text)

		var subdir1 *FileNode
		for _, child := range tree.Children {
			if child.Name == "subdir1" {
				subdir1 = child
			}
		}
		require.NotNil(t, subdir1)
		require.Len(t, subdir1.Children, 1)

		// At depth 3 subdir2 is still expanded, so file3.txt shows up as a
		// count rather than being dropped silently.
		subdir2 := subdir1.Children[0]
		assert.Equal(t, "subdir2", subdir2.Name)
		assert.Empty(t, subdir2.Children)
		require.NotNil(t, subdir2.FileCount)
		assert.Equal(t, 1, *subdir2.FileCount)
	})

	t.Run("depth below 1 is clamped to 1", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path":  tmpDir,
					"depth": 0.0,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "max depth: 1")
		assert.Contains(t, textContent.Text, "file1.txt")
	})

	t.Run("tree of empty directory", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": emptydirPath,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.False(t, res.IsError)

		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "Directory tree for")

		// Parse JSON to verify it's a directory with no children
		tree := parseTree(t, textContent.Text)
		assert.Equal(t, "directory", tree.Type)
		assert.Equal(t, "emptydir", tree.Name)
		assert.Nil(t, tree.Children)
	})

	t.Run("try to tree a file instead of directory", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": file1Path,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.True(t, res.IsError)

		require.Len(t, res.Content, 1)
		textContent := res.Content[0].(mcp.TextContent)
		assert.Contains(t, textContent.Text, "not a directory")
	})

	t.Run("try to tree non-existent directory", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "non_existent_directory")

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": nonExistentPath,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.True(t, res.IsError)
	})

	t.Run("path is in a non-allowed directory", func(t *testing.T) {
		otherDir := t.TempDir()

		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{
					"path": otherDir,
				},
			},
		}

		res, err := fsHandler.HandleTree(ctx, req)
		require.NoError(t, err)
		require.True(t, res.IsError)
	})
}
