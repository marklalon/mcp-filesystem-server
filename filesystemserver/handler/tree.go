package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
)

func (fs *FilesystemHandler) HandleTree(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	path, err := request.RequireString("path")
	if err != nil {
		return nil, err
	}

	// Extract depth parameter (optional, default: 2)
	depth := 2 // Default value
	if depthParam, err := request.RequireFloat("depth"); err == nil {
		depth = int(depthParam)
	}
	if depth < 1 {
		depth = 1
	}

	// Individual files are only listed for a single-level tree. Deeper trees
	// return the directory skeleton with a per-directory file count instead, so
	// that a directory with many files cannot blow up the result.
	includeFiles := depth == 1

	// Extract follow_symlinks parameter (optional, default: false)
	followSymlinks := false // Default value
	if followParam, err := request.RequireBool("follow_symlinks"); err == nil {
		followSymlinks = followParam
	}

	// Validate the path is within allowed directories
	validPath, err := fs.validatePath(path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}
	displayPath := fs.relPath(validPath)

	// Check if it's a directory
	info, err := os.Stat(validPath)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	if !info.IsDir() {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Error: The specified path is not a directory",
				},
			},
			IsError: true,
		}, nil
	}

	// Build the tree structure
	tree, err := fs.buildTree(validPath, depth, 0, followSymlinks, includeFiles)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error building directory tree: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error generating JSON: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Create resource URI for the directory
	resourceURI := fs.pathToResourceURI(validPath)

	note := ""
	if !includeFiles {
		note = ", files omitted and reported as file_count per directory; use depth 1 to list them"
	}

	// Return the result
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Directory tree for %s (max depth: %d%s):\n\n%s", displayPath, depth, note, string(jsonData)),
			},
			mcp.EmbeddedResource{
				Type: "resource",
				Resource: mcp.TextResourceContents{
					URI:      resourceURI,
					MIMEType: "application/json",
					Text:     string(jsonData),
				},
			},
		},
	}, nil
}

// buildTree builds a tree representation of the filesystem starting at the given path.
// Files are only added as children when includeFiles is set and the directory is
// within maxDepth. Every other directory instead carries the number of files
// directly inside it, so a directory whose files are not listed is never mistaken
// for an empty one.
func (fs *FilesystemHandler) buildTree(path string, maxDepth int, currentDepth int, followSymlinks bool, includeFiles bool) (*FileNode, error) {
	// Validate the path
	validPath, err := fs.validatePath(path)
	if err != nil {
		return nil, err
	}

	// Get file info
	info, err := os.Stat(validPath)
	if err != nil {
		return nil, err
	}

	// Create the node
	node := &FileNode{
		Name:     filepath.Base(validPath),
		Path:     fs.relPath(validPath),
		Modified: info.ModTime(),
	}

	// Set type and size
	if info.IsDir() {
		node.Type = "directory"

		// Directories are read even at the max depth, so that their files can
		// still be counted. Only their subdirectories are left unexplored.
		expand := currentDepth < maxDepth
		listFiles := expand && includeFiles

		entries, err := os.ReadDir(validPath)
		if err != nil {
			// The root of the tree is the caller's own request, so report the
			// failure. Deeper down, keep the directory in the tree without
			// children rather than dropping it from its parent's listing.
			if currentDepth == 0 {
				return nil, err
			}
			return node, nil
		}

		fileCount := 0

		// Process each entry
		for _, entry := range entries {
			entryPath, isDir, ok := fs.resolveTreeEntry(validPath, entry, followSymlinks)
			if !ok {
				continue
			}

			if !isDir {
				// Count files instead of listing them when files are excluded
				if !listFiles {
					fileCount++
					continue
				}
			} else if !expand {
				// Past the max depth, subdirectories are not descended into
				continue
			}

			// Recursively build child node
			childNode, err := fs.buildTree(entryPath, maxDepth, currentDepth+1, followSymlinks, includeFiles)
			if err != nil {
				// Skip entries with errors
				continue
			}

			// Add child to the current node
			node.Children = append(node.Children, childNode)
		}

		if !listFiles {
			node.FileCount = &fileCount
		}
	} else {
		node.Type = "file"
		node.Size = info.Size()
	}

	return node, nil
}

// resolveTreeEntry returns the path to walk for a directory entry and whether that
// path is a directory. ok is false when the entry should be skipped entirely.
func (fs *FilesystemHandler) resolveTreeEntry(dir string, entry os.DirEntry, followSymlinks bool) (path string, isDir bool, ok bool) {
	entryPath := filepath.Join(dir, entry.Name())

	if entry.Type()&os.ModeSymlink == 0 {
		return entryPath, entry.IsDir(), true
	}

	// Skip symlinks if not following them
	if !followSymlinks {
		return "", false, false
	}

	// Resolve symlink
	linkDest, err := filepath.EvalSymlinks(entryPath)
	if err != nil {
		// Skip invalid symlinks
		return "", false, false
	}

	// Validate the symlink destination is within the root directory
	if !fs.isPathInRoot(linkDest) {
		// Skip symlinks pointing outside the root directory
		return "", false, false
	}

	// The entry type describes the link itself, so the destination has to be
	// inspected to know whether it counts as a file or a directory
	linkInfo, err := os.Stat(linkDest)
	if err != nil {
		return "", false, false
	}

	return linkDest, linkInfo.IsDir(), true
}
