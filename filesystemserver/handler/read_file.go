package handler

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// errLineRangeTooLarge is returned when the requested line range holds more
// content than can be returned inline.
var errLineRangeTooLarge = errors.New("line range too large")

// readLineRange returns the raw contents of lines [startLine, endLine] (1-based,
// inclusive) of the file at path, together with the number of lines seen while
// reading. Line endings are preserved exactly as stored, and reading stops as
// soon as endLine is reached, so a range may be taken from a very large file.
// A line is counted as terminated by "\n"; trailing content without a final
// newline counts as one more line.
func readLineRange(path string, startLine, endLine int, maxBytes int) (string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	// Read with a bufio.Reader rather than a Scanner so that arbitrarily long
	// lines are handled instead of aborting the read
	reader := bufio.NewReader(file)
	var content strings.Builder
	lineNum := 0

	for lineNum < endLine {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			lineNum++
			if lineNum >= startLine {
				if content.Len()+len(line) > maxBytes {
					return "", lineNum, errLineRangeTooLarge
				}
				content.WriteString(line)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", lineNum, err
		}
	}

	return content.String(), lineNum, nil
}

func (fs *FilesystemHandler) HandleReadFile(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	path, err := request.RequireString("path")
	if err != nil {
		return nil, err
	}

	// Extract the optional line range. Both bounds are 1-based and inclusive;
	// start_line defaults to the first line and end_line to the last one.
	startLine := 1
	endLine := 0 // 0 means "until the end of the file"
	hasRange := false

	if val, err := request.RequireFloat("start_line"); err == nil {
		startLine = int(val)
		hasRange = true
		if startLine < 1 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Error: start_line must be 1 or greater",
					},
				},
				IsError: true,
			}, nil
		}
	}

	if val, err := request.RequireFloat("end_line"); err == nil {
		endLine = int(val)
		hasRange = true
		if endLine < startLine {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error: end_line (%d) must be greater than or equal to start_line (%d)", endLine, startLine),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Handle empty or relative paths like "." or "./" by converting to absolute path
	if path == "." || path == "./" {
		// Get current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error resolving current directory: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
		path = cwd
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

	if info.IsDir() {
		// For directories, return a resource reference instead
		resourceURI := pathToResourceURI(validPath)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("This is a directory. Use the resource URI to browse its contents: %s", resourceURI),
				},
				mcp.EmbeddedResource{
					Type: "resource",
					Resource: mcp.TextResourceContents{
						URI:      resourceURI,
						MIMEType: "text/plain",
						Text:     fmt.Sprintf("Directory: %s", validPath),
					},
				},
			},
		}, nil
	}

	// Determine MIME type
	mimeType := detectMimeType(validPath)

	// A line range is served without regard to the total file size, since only
	// the requested lines are read
	if hasRange {
		if !isTextFile(mimeType) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error: start_line/end_line are only supported for text files, but %s was detected as %s", validPath, mimeType),
					},
				},
				IsError: true,
			}, nil
		}

		lastLine := endLine
		if lastLine == 0 {
			lastLine = math.MaxInt
		}

		content, linesSeen, err := readLineRange(validPath, startLine, lastLine, MAX_INLINE_SIZE)
		if err != nil {
			if errors.Is(err, errLineRangeTooLarge) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						mcp.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Requested line range is too large to display inline (over %d bytes). Request fewer lines, or access the file via resource URI: %s", MAX_INLINE_SIZE, pathToResourceURI(validPath)),
						},
					},
					IsError: true,
				}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error reading file: %v", err),
					},
				},
				IsError: true,
			}, nil
		}

		if linesSeen < startLine {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Error: start_line %d is past the end of the file, which has %d lines", startLine, linesSeen),
					},
				},
				IsError: true,
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: content,
				},
			},
		}, nil
	}

	// Check file size
	if info.Size() > MAX_INLINE_SIZE {
		// File is too large to inline, return a resource reference
		resourceURI := pathToResourceURI(validPath)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("File is too large to display inline (%d bytes). Access it via resource URI: %s", info.Size(), resourceURI),
				},
				mcp.EmbeddedResource{
					Type: "resource",
					Resource: mcp.TextResourceContents{
						URI:      resourceURI,
						MIMEType: "text/plain",
						Text:     fmt.Sprintf("Large file: %s (%s, %d bytes)", validPath, mimeType, info.Size()),
					},
				},
			},
		}, nil
	}

	// Read file content
	content, err := os.ReadFile(validPath)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error reading file: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Check if it's a text file
	if isTextFile(mimeType) {
		// It's a text file, return as text
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(content),
				},
			},
		}, nil
	} else if isImageFile(mimeType) {
		// It's an image file, return as image content
		if info.Size() <= MAX_BASE64_SIZE {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Image file: %s (%s, %d bytes)", validPath, mimeType, info.Size()),
					},
					mcp.ImageContent{
						Type:     "image",
						Data:     base64.StdEncoding.EncodeToString(content),
						MIMEType: mimeType,
					},
				},
			}, nil
		} else {
			// Too large for base64, return a reference
			resourceURI := pathToResourceURI(validPath)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Image file is too large to display inline (%d bytes). Access it via resource URI: %s", info.Size(), resourceURI),
					},
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.TextResourceContents{
							URI:      resourceURI,
							MIMEType: "text/plain",
							Text:     fmt.Sprintf("Large image: %s (%s, %d bytes)", validPath, mimeType, info.Size()),
						},
					},
				},
			}, nil
		}
	} else {
		// It's another type of binary file
		resourceURI := pathToResourceURI(validPath)

		if info.Size() <= MAX_BASE64_SIZE {
			// Small enough for base64 encoding
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Binary file: %s (%s, %d bytes)", validPath, mimeType, info.Size()),
					},
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.BlobResourceContents{
							URI:      resourceURI,
							MIMEType: mimeType,
							Blob:     base64.StdEncoding.EncodeToString(content),
						},
					},
				},
			}, nil
		} else {
			// Too large for base64, return a reference
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Binary file: %s (%s, %d bytes). Access it via resource URI: %s", validPath, mimeType, info.Size(), resourceURI),
					},
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.TextResourceContents{
							URI:      resourceURI,
							MIMEType: "text/plain",
							Text:     fmt.Sprintf("Binary file: %s (%s, %d bytes)", validPath, mimeType, info.Size()),
						},
					},
				},
			}, nil
		}
	}
}