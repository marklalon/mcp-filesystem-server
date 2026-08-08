package handler

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
)

// lineMatcher reports the byte range of the first match within a single line.
type lineMatcher interface {
	match(line string) (start, end int, ok bool)
}

// substringMatcher matches a literal, case-sensitive substring.
type substringMatcher struct {
	needle string
}

func (m substringMatcher) match(line string) (int, int, bool) {
	index := strings.Index(line, m.needle)
	if index < 0 {
		return 0, 0, false
	}
	return index, index + len(m.needle), true
}

// regexMatcher matches a compiled regular expression.
type regexMatcher struct {
	re *regexp.Regexp
}

func (m regexMatcher) match(line string) (int, int, bool) {
	loc := m.re.FindStringIndex(line)
	if loc == nil {
		return 0, 0, false
	}
	return loc[0], loc[1], true
}

// newLineMatcher builds a matcher for the given pattern. Case-insensitive literal
// searches are handled by quoting the pattern and running it through the regexp
// engine, which keeps match offsets aligned with the original line.
func newLineMatcher(pattern string, useRegex, ignoreCase bool) (lineMatcher, error) {
	if !useRegex && !ignoreCase {
		return substringMatcher{needle: pattern}, nil
	}

	expr := pattern
	if !useRegex {
		expr = regexp.QuoteMeta(pattern)
	}
	if ignoreCase {
		expr = "(?i)" + expr
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	return regexMatcher{re: re}, nil
}

func (fs *FilesystemHandler) HandleSearchWithinFiles(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	// Extract and validate parameters
	path, err := request.RequireString("path")
	if err != nil {
		return nil, err
	}
	substring, err := request.RequireString("substring")
	if err != nil {
		return nil, err
	}
	if substring == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Error: substring cannot be empty",
				},
			},
			IsError: true,
		}, nil
	}

	// Extract optional depth parameter
	maxDepth := 0 // 0 means unlimited
	if depthArg, err := request.RequireFloat("depth"); err == nil {
		maxDepth = int(depthArg)
		if maxDepth < 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Error: depth cannot be negative",
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Extract optional max_results parameter
	maxResults := MAX_SEARCH_RESULTS // default limit
	if maxResultsArg, err := request.RequireFloat("max_results"); err == nil {
		maxResults = int(maxResultsArg)
		if maxResults <= 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "Error: max_results must be positive",
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Extract optional regex parameter
	useRegex := false
	if val, err := request.RequireBool("regex"); err == nil {
		useRegex = val
	}

	// Extract optional ignore_case parameter
	ignoreCase := false
	if val, err := request.RequireBool("ignore_case"); err == nil {
		ignoreCase = val
	}

	// Compile the pattern once, before touching the file system
	matcher, err := newLineMatcher(substring, useRegex, ignoreCase)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error: Invalid regular expression: %v", err),
				},
			},
			IsError: true,
		}, nil
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

	// Check if the path is a directory
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
					Text: "Error: search path must be a directory",
				},
			},
			IsError: true,
		}, nil
	}

	// Perform the search
	results, err := searchWithinFiles(validPath, matcher, maxDepth, maxResults, fs)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error searching within files: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Describe what was searched for, since the pattern may be a regex
	matchTerm := "occurrences of"
	if useRegex {
		matchTerm = "matches for"
	}

	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("No %s '%s' found in files under %s", matchTerm, substring, displayPath),
				},
			},
		}, nil
	}

	// Format search results
	var formattedResults strings.Builder
	formattedResults.WriteString(fmt.Sprintf("Found %d %s '%s':\n\n", len(results), matchTerm, substring))

	// Group results by file for easier readability
	fileResultsMap := make(map[string][]SearchResult)
	for _, result := range results {
		fileResultsMap[result.FilePath] = append(fileResultsMap[result.FilePath], result)
	}

	// Display results grouped by file
	for filePath, fileResults := range fileResultsMap {
		resourceURI := fs.pathToResourceURI(filePath)
		formattedResults.WriteString(fmt.Sprintf("File: %s (%s)\n", fs.relPath(filePath), resourceURI))

		for _, result := range fileResults {
			formattedResults.WriteString(fmt.Sprintf("  Line %d: %s\n", result.LineNumber, truncateAroundMatch(result)))
		}
		formattedResults.WriteString("\n")
	}

	// If results were limited, note this in the output
	if len(results) >= maxResults {
		formattedResults.WriteString(fmt.Sprintf("\nNote: Results limited to %d matches. There may be more occurrences.", maxResults))
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

// truncateAroundMatch shortens long lines, keeping context around the match.
func truncateAroundMatch(result SearchResult) string {
	line := result.LineContent
	if len(line) <= 100 {
		return line
	}

	// Keep the cut points on rune boundaries so multi-byte characters stay intact
	contextStart := snapToRuneStart(line, max(0, result.MatchStart-30))
	contextEnd := snapToRuneEnd(line, min(len(line), result.MatchEnd+30))

	truncated := line[contextStart:contextEnd]
	if contextStart > 0 {
		truncated = "..." + truncated
	}
	if contextEnd < len(line) {
		truncated += "..."
	}
	return truncated
}

// snapToRuneStart moves index back to the start of the rune it falls inside.
func snapToRuneStart(s string, index int) int {
	for index > 0 && !utf8.RuneStart(s[index]) {
		index--
	}
	return index
}

// snapToRuneEnd moves index forward to the next rune boundary.
func snapToRuneEnd(s string, index int) int {
	for index < len(s) && !utf8.RuneStart(s[index]) {
		index++
	}
	return index
}

// clipLineAroundMatch bounds how much of a matching line is kept in memory,
// returning the clipped line and the match offsets within it. The window kept
// here is much wider than the one used when formatting results, so clipping
// never changes the reported output.
func clipLineAroundMatch(line string, matchStart, matchEnd int) (string, int, int) {
	start := snapToRuneStart(line, max(0, matchStart-MAX_STORED_LINE_CONTEXT))
	end := snapToRuneEnd(line, min(len(line), matchEnd+MAX_STORED_LINE_CONTEXT))

	if start == 0 && end == len(line) {
		return line, matchStart, matchEnd
	}

	// Clone, since slicing alone would keep the whole line alive
	return strings.Clone(line[start:end]), matchStart - start, matchEnd - start
}

// searchWithinFiles searches file contents for lines matching the given matcher
func searchWithinFiles(
	rootPath string, matcher lineMatcher, maxDepth int, maxResults int, fs *FilesystemHandler,
) ([]SearchResult, error) {
	var results []SearchResult
	resultCount := 0
	currentDepth := 0

	// Walk the directory tree
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
			validPath, err := fs.validatePath(path)
			if err != nil {
				return nil // Skip invalid paths
			}

			// Skip directories, only search files
			if info.IsDir() {
				// Calculate depth for this directory
				relPath, err := filepath.Rel(rootPath, path)
				if err != nil {
					return nil // Skip on error
				}

				// Count separators to determine depth (empty or "." means we're at rootPath)
				if relPath == "" || relPath == "." {
					currentDepth = 0
				} else {
					currentDepth = strings.Count(relPath, string(filepath.Separator)) + 1
				}

				// Skip directories beyond max depth if specified
				if maxDepth > 0 && currentDepth >= maxDepth {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip files that are too large
			if info.Size() > MAX_SEARCHABLE_SIZE {
				return nil
			}

			// Determine MIME type and skip non-text files
			mimeType := detectMimeType(validPath)
			if !isTextFile(mimeType) {
				return nil
			}

			// Open the file and search for the substring
			file, err := os.Open(validPath)
			if err != nil {
				return nil // Skip files that can't be opened
			}
			defer file.Close()

			// Create a scanner to read the file line by line. The default 64KB line
			// limit makes the scanner fail on files such as minified sources or
			// single-line JSON, silently skipping the rest of the file, so raise it
			// to a bounded size. The buffer starts small and only grows as needed.
			scanner := bufio.NewScanner(file)
			scanner.Buffer(nil, MAX_SEARCHABLE_LINE_SIZE)
			lineNum := 0

			// Scan each line
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()

				// Check if the line matches the pattern
				if matchStart, matchEnd, ok := matcher.match(line); ok {
					// Keep only a bounded window of the line, so that very long
					// lines are not retained in full for every result
					line, matchStart, matchEnd = clipLineAroundMatch(line, matchStart, matchEnd)

					// Add to results
					results = append(results, SearchResult{
						FilePath:    validPath,
						LineNumber:  lineNum,
						LineContent: line,
						ResourceURI: fs.pathToResourceURI(validPath),
						MatchStart:  matchStart,
						MatchEnd:    matchEnd,
					})
					resultCount++

					// Check if we've reached the maximum results
					if resultCount >= maxResults {
						return filepath.SkipAll
					}
				}
			}

			// Check for scanner errors
			if err := scanner.Err(); err != nil {
				return nil // Skip files with scanning errors
			}

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	return results, nil
}

// Helper function since Go < 1.21 doesn't have min/max functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function since Go < 1.21 doesn't have min/max functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
