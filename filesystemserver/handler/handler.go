package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
)

// FilesystemHandler serves a single root directory. Every path handled by the
// server is expressed relative to that root: tool arguments are resolved
// against it, and paths reported back are rendered relative to it.
type FilesystemHandler struct {
	// rootDir is an absolute, cleaned, symlink-resolved path without a
	// trailing separator.
	rootDir string

	// ignoredDirPatterns are matched against directory names. Matching
	// directories and all paths below them are inaccessible.
	ignoredDirPatterns []glob.Glob
}

func NewFilesystemHandler(rootDir string, ignoredDirPatterns ...string) (*FilesystemHandler, error) {
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

	compiledPatterns := make([]glob.Glob, 0, len(ignoredDirPatterns))
	for _, patternGroup := range ignoredDirPatterns {
		patterns := strings.Split(patternGroup, "|")
		for _, pattern := range patterns {
			if strings.TrimSpace(pattern) == "" {
				return nil, fmt.Errorf("ignored directory pattern cannot be empty")
			}

			compiled, err := glob.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid ignored directory pattern %q: %w", pattern, err)
			}
			compiledPatterns = append(compiledPatterns, compiled)
		}
	}

	return &FilesystemHandler{
		rootDir:            filepath.Clean(resolved),
		ignoredDirPatterns: compiledPatterns,
	}, nil
}

func (fs *FilesystemHandler) shouldIgnoreDirName(name string) bool {
	for _, pattern := range fs.ignoredDirPatterns {
		if pattern.Match(name) {
			return true
		}
	}
	return false
}

// pathContainsIgnoredDir reports whether path is inside an existing directory
// whose name matches one of the configured ignore patterns. The root itself is
// never considered ignored, even when its own name matches a pattern.
func (fs *FilesystemHandler) pathContainsIgnoredDir(path string) bool {
	cleaned := filepath.Clean(path)
	if !fs.isPathInRoot(cleaned) || cleaned == fs.rootDir {
		return false
	}

	rel, err := filepath.Rel(fs.rootDir, cleaned)
	if err != nil || rel == "." {
		return false
	}

	current := fs.rootDir
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if !fs.shouldIgnoreDirName(component) {
			continue
		}

		info, err := os.Stat(current)
		if err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

func (fs *FilesystemHandler) ignoredPathError(requestedPath string) error {
	return fmt.Errorf("access denied - path is inside an ignored directory: %s", requestedPath)
}
