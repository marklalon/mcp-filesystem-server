package handler

import "time"

const (
	// Maximum size for inline content (5MB)
	MAX_INLINE_SIZE = 5 * 1024 * 1024
	// Maximum size for base64 encoding (1MB)
	MAX_BASE64_SIZE = 1 * 1024 * 1024
	// Maximum number of search results to return (prevent excessive output)
	MAX_SEARCH_RESULTS = 1000
	// Maximum file size in bytes to search within (10MB)
	MAX_SEARCHABLE_SIZE = 10 * 1024 * 1024
	// Maximum length in bytes of a single line to search within (1MB). Files
	// containing a longer line are skipped from that line on.
	MAX_SEARCHABLE_LINE_SIZE = 1024 * 1024
	// Maximum number of bytes kept on each side of a match when a matching line
	// is stored (comfortably wider than the window used to display a match)
	MAX_STORED_LINE_CONTEXT = 512
)

type FileInfo struct {
	Size        int64     `json:"size"`
	Created     time.Time `json:"created"`
	Modified    time.Time `json:"modified"`
	Accessed    time.Time `json:"accessed"`
	IsDirectory bool      `json:"isDirectory"`
	IsFile      bool      `json:"isFile"`
	Permissions string    `json:"permissions"`
}

// FileNode represents a node in the file tree
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"type"` // "file" or "directory"
	Size     int64       `json:"size,omitempty"`
	Modified time.Time   `json:"modified,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// SearchResult represents a single match in a file
type SearchResult struct {
	FilePath    string
	LineNumber  int
	LineContent string
	ResourceURI string
	// MatchStart and MatchEnd are the byte offsets of the match within LineContent.
	MatchStart int
	MatchEnd   int
}
