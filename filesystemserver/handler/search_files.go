package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/mark3labs/mcp-go/mcp"
)

func (fs *FilesystemHandler) HandleSearchFiles(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	path, err := request.RequireString("path")
	if err != nil {
		return nil, err
	}
	pattern, err := request.RequireString("pattern")
	if err != nil {
		return nil, err
	}

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
					Text: "Error: Search path must be a directory",
				},
			},
			IsError: true,
		}, nil
	}

	results, err := searchFiles(validPath, pattern, fs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error searching files: %v",
						err),
				},
			},
			IsError: true,
		}, nil
	}

	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("No files found matching pattern '%s' in %s", pattern, displayPath),
				},
			},
		}, nil
	}

	// Format results with resource URIs
	var formattedResults strings.Builder
	formattedResults.WriteString(fmt.Sprintf("Found %d results:\n\n", len(results)))

	for _, result := range results {
		resourceURI := fs.pathToResourceURI(result)
		displayResult := fs.relPath(result)
		info, err := os.Stat(result)
		if err == nil {
			if info.IsDir() {
				formattedResults.WriteString(fmt.Sprintf("[DIR]  %s (%s)\n", displayResult, resourceURI))
			} else {
				formattedResults.WriteString(fmt.Sprintf("[FILE] %s (%s) - %d bytes\n",
					displayResult, resourceURI, info.Size()))
			}
		} else {
			formattedResults.WriteString(fmt.Sprintf("%s (%s)\n", displayResult, resourceURI))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: formattedResults.String(),
			},
		},
	}, nil
}

func searchFiles(rootPath, pattern string, fs *FilesystemHandler) ([]string, error) {
	var results []string
	globPattern := glob.MustCompile(pattern)

	err := filepath.Walk(
		rootPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors and continue
			}

			if path != rootPath && info.IsDir() && fs.shouldIgnoreDirName(info.Name()) {
				return filepath.SkipDir
			}

			// Try to validate path
			if _, err := fs.validatePath(path); err != nil {
				return nil // Skip invalid paths
			}

			if globPattern.Match(info.Name()) {
				results = append(results, path)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return results, nil
}
