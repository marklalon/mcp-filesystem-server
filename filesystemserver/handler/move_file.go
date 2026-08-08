package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
)

func (fs *FilesystemHandler) HandleMoveFile(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	source, err := request.RequireString("source")
	if err != nil {
		return nil, err
	}
	destination, err := request.RequireString("destination")
	if err != nil {
		return nil, err
	}

	validSource, err := fs.validatePath(source)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error with source path: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	displaySource := fs.relPath(validSource)

	// Check if source exists
	if _, err := os.Stat(validSource); os.IsNotExist(err) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: Source does not exist: %s", displaySource),
				},
			},
			IsError: true,
		}, nil
	} else if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error accessing source: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	if fs.shouldIgnoreDirName(filepath.Base(fs.resolvePath(destination))) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error with destination path: %v", fs.ignoredPathError(destination)),
				},
			},
			IsError: true,
		}, nil
	}

	// For destination path, validate the parent directory first and create it if
	// needed. The destination is resolved before taking its parent, so that a
	// relative destination is anchored to the root directory.
	destDir := filepath.Dir(fs.resolvePath(destination))
	validDestDir, err := fs.validatePath(destDir)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error with destination directory path: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Create parent directory for destination if it doesn't exist
	if err := os.MkdirAll(validDestDir, 0755); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error creating destination directory: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Now validate the full destination path
	validDest, err := fs.validatePath(destination)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error with destination path: %v", err),
				},
			},
			IsError: true,
		}, nil
	}
	displayDest := fs.relPath(validDest)

	if err := os.Rename(validSource, validDest); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error moving file: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	resourceURI := fs.pathToResourceURI(validDest)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(
					"Successfully moved %s to %s",
					displaySource,
					displayDest,
				),
			},
			mcp.EmbeddedResource{
				Type: "resource",
				Resource: mcp.TextResourceContents{
					URI:      resourceURI,
					MIMEType: "text/plain",
					Text:     fmt.Sprintf("Moved file: %s", displayDest),
				},
			},
		},
	}, nil
}
