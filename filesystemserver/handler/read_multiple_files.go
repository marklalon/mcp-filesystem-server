package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
)

func (fs *FilesystemHandler) HandleReadMultipleFiles(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	pathsSlice, err := request.RequireStringSlice("paths")
	if err != nil {
		return nil, err
	}

	if len(pathsSlice) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "No files specified to read",
				},
			},
			IsError: true,
		}, nil
	}

	// Maximum number of files to read in a single request
	const maxFiles = 50
	if len(pathsSlice) > maxFiles {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Too many files requested. Maximum is %d files per request.", maxFiles),
				},
			},
			IsError: true,
		}, nil
	}

	// Process each file
	var results []mcp.Content
	for _, path := range pathsSlice {
		validPath, err := fs.validatePath(path)
		if err != nil {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error with path '%s': %v", path, err),
			})
			continue
		}
		displayPath := fs.relPath(validPath)

		// Check if it's a directory
		info, err := os.Stat(validPath)
		if err != nil {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error accessing '%s': %v", displayPath, err),
			})
			continue
		}

		if info.IsDir() {
			// For directories, return a resource reference instead
			resourceURI := fs.pathToResourceURI(validPath)
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("'%s' is a directory. Use list_directory tool or resource URI: %s", displayPath, resourceURI),
			})
			continue
		}

		// Determine MIME type
		mimeType := detectMimeType(validPath)

		// Check file size
		if info.Size() > MAX_INLINE_SIZE {
			// File is too large to inline, return a resource reference
			resourceURI := fs.pathToResourceURI(validPath)
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("File '%s' is too large to display inline (%d bytes). Access it via resource URI: %s",
					displayPath, info.Size(), resourceURI),
			})
			continue
		}

		// Read file content
		content, err := os.ReadFile(validPath)
		if err != nil {
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Error reading file '%s': %v", displayPath, err),
			})
			continue
		}

		// Add file header
		results = append(results, mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("--- File: %s ---", displayPath),
		})

		// Check if it's a text file
		if isTextFile(mimeType) {
			// It's a text file, return as text
			results = append(results, mcp.TextContent{
				Type: "text",
				Text: string(content),
			})
		} else if isImageFile(mimeType) {
			// It's an image file, return as image content
			if info.Size() <= MAX_BASE64_SIZE {
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Image file: %s (%s, %d bytes)", displayPath, mimeType, info.Size()),
				})
				results = append(results, mcp.ImageContent{
					Type:     "image",
					Data:     base64.StdEncoding.EncodeToString(content),
					MIMEType: mimeType,
				})
			} else {
				// Too large for base64, return a reference
				resourceURI := fs.pathToResourceURI(validPath)
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Image file '%s' is too large to display inline (%d bytes). Access it via resource URI: %s",
						displayPath, info.Size(), resourceURI),
				})
			}
		} else {
			// It's another type of binary file
			resourceURI := fs.pathToResourceURI(validPath)

			if info.Size() <= MAX_BASE64_SIZE {
				// Small enough for base64 encoding
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Binary file: %s (%s, %d bytes)", displayPath, mimeType, info.Size()),
				})
				results = append(results, mcp.EmbeddedResource{
					Type: "resource",
					Resource: mcp.BlobResourceContents{
						URI:      resourceURI,
						MIMEType: mimeType,
						Blob:     base64.StdEncoding.EncodeToString(content),
					},
				})
			} else {
				// Too large for base64, return a reference
				results = append(results, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Binary file '%s' (%s, %d bytes). Access it via resource URI: %s",
						displayPath, mimeType, info.Size(), resourceURI),
				})
			}
		}
	}

	return &mcp.CallToolResult{
		Content: results,
	}, nil
}