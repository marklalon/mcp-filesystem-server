package handler

import (
	"fmt"
	"os"
	"path/filepath"
)

// FilesystemHandler serves a single root directory. Every path handled by the
// server is expressed relative to that root: tool arguments are resolved
// against it, and paths reported back are rendered relative to it.
type FilesystemHandler struct {
	// rootDir is an absolute, cleaned, symlink-resolved path without a
	// trailing separator.
	rootDir string
}

func NewFilesystemHandler(rootDir string) (*FilesystemHandler, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path %s: %w", rootDir, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("failed to access directory %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", abs)
	}

	// Resolve the root once, so that it can be compared against request paths,
	// which are themselves symlink-resolved before being checked
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory %s: %w", abs, err)
	}

	return &FilesystemHandler{
		rootDir: filepath.Clean(resolved),
	}, nil
}
