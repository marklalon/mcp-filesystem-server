package handler

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

// isPathInRoot checks whether an absolute path is the root directory itself or
// lies inside it. The separator guards against a prefix match on a sibling
// directory (e.g. /tmp/foo should not match /tmp/foobar).
func (fs *FilesystemHandler) isPathInRoot(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	if cleaned == fs.rootDir {
		return true
	}

	// The root of a volume ("/" or "C:\") already ends with a separator
	prefix := fs.rootDir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(cleaned, prefix)
}

// resolvePath turns a path given by a tool caller into an absolute path.
// Relative paths - including "", "." and "./" - are resolved against the root
// directory rather than the process working directory. Absolute paths are
// accepted as-is and are rejected later if they fall outside the root.
func (fs *FilesystemHandler) resolvePath(requestedPath string) string {
	if requestedPath == "" {
		return fs.rootDir
	}
	if filepath.IsAbs(requestedPath) {
		return filepath.Clean(requestedPath)
	}
	return filepath.Join(fs.rootDir, requestedPath)
}

// relPath renders an absolute path as a path relative to the root directory,
// using forward slashes so that output is the same on every platform. The root
// directory itself renders as ".".
func (fs *FilesystemHandler) relPath(absPath string) string {
	rel, err := filepath.Rel(fs.rootDir, filepath.Clean(absPath))
	if err != nil {
		// Not expressible relative to the root; fall back to the path as given
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

// pathToResourceURI converts an absolute path to a resource URI holding the
// path relative to the root directory.
func (fs *FilesystemHandler) pathToResourceURI(absPath string) string {
	return "file://" + fs.relPath(absPath)
}

// validatePath turns a path given by a tool caller into an absolute path that
// is guaranteed to lie inside the root directory. The path is resolved before
// it is checked, so that a symlink cannot be used to step outside the root, and
// so that a path spelled differently but pointing at the same file (a Windows
// 8.3 short name, say) is recognised as being inside it.
func (fs *FilesystemHandler) validatePath(requestedPath string) (string, error) {
	abs := fs.resolvePath(requestedPath)

	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}

		// The path does not exist yet, so resolve its parent instead. This is
		// what allows a file to be created or written through.
		parent := filepath.Dir(abs)
		realParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return "", fmt.Errorf("parent directory does not exist: %s", fs.relPath(parent))
		}

		if !fs.isPathInRoot(realParent) {
			return "", fmt.Errorf(
				"access denied - path outside the allowed directory: %s",
				requestedPath,
			)
		}
		return filepath.Join(realParent, filepath.Base(abs)), nil
	}

	// Check if the real path (after resolving symlinks) is within the root
	if !fs.isPathInRoot(realPath) {
		return "", fmt.Errorf(
			"access denied - path outside the allowed directory: %s",
			requestedPath,
		)
	}

	return realPath, nil
}

// detectMimeType tries to determine the MIME type of a file
func detectMimeType(path string) string {
	// Use mimetype library for more accurate detection
	mtype, err := mimetype.DetectFile(path)
	if err != nil {
		// Fallback to extension-based detection if file can't be read
		ext := filepath.Ext(path)
		if ext != "" {
			mimeType := mime.TypeByExtension(ext)
			if mimeType != "" {
				return mimeType
			}
		}
		return "application/octet-stream" // Default
	}

	return mtype.String()
}

// isTextFile determines if a file is likely a text file based on MIME type
func isTextFile(mimeType string) bool {
	// Check for common text MIME types
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}

	// Common application types that are text-based
	textApplicationTypes := []string{
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
		"application/typescript",
		"application/x-typescript",
		"application/x-yaml",
		"application/yaml",
		"application/toml",
		"application/x-sh",
		"application/x-shellscript",
	}

	if slices.Contains(textApplicationTypes, mimeType) {
		return true
	}

	// Check for +format types
	if strings.Contains(mimeType, "+xml") ||
		strings.Contains(mimeType, "+json") ||
		strings.Contains(mimeType, "+yaml") {
		return true
	}

	// Common code file types that might be misidentified
	if strings.HasPrefix(mimeType, "text/x-") {
		return true
	}

	if strings.HasPrefix(mimeType, "application/x-") &&
		(strings.Contains(mimeType, "script") ||
			strings.Contains(mimeType, "source") ||
			strings.Contains(mimeType, "code")) {
		return true
	}

	return false
}

// isImageFile determines if a file is an image based on MIME type
func isImageFile(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/") ||
		(mimeType == "application/xml" && strings.HasSuffix(strings.ToLower(mimeType), ".svg"))
}
