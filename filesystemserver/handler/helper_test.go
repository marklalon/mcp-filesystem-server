package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot(t *testing.T) {
	root, err := filepath.Abs(string(filepath.Separator))
	require.NoError(t, err)

	handler, err := NewFilesystemHandler(root)
	require.NoError(t, err)
	assert.True(t, handler.isPathInRoot(filepath.Join(root, "etc", "hostname")))
}

func TestResolvePath(t *testing.T) {
	dir := t.TempDir()
	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)
	root := handler.rootDir

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty path is the root", path: "", want: root},
		{name: "dot is the root", path: ".", want: root},
		{name: "dot slash is the root", path: "./", want: root},
		{name: "relative file", path: "sub/file.txt", want: filepath.Join(root, "sub", "file.txt")},
		{name: "forward slashes are accepted", path: "sub/nested/file.txt", want: filepath.Join(root, "sub", "nested", "file.txt")},
		{name: "absolute path is kept", path: filepath.Join(root, "file.txt"), want: filepath.Join(root, "file.txt")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, handler.resolvePath(tt.path))
		})
	}
}

func TestRelPath(t *testing.T) {
	dir := t.TempDir()
	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)
	root := handler.rootDir

	assert.Equal(t, ".", handler.relPath(root))
	assert.Equal(t, "file.txt", handler.relPath(filepath.Join(root, "file.txt")))
	// Reported paths always use forward slashes, on every platform
	assert.Equal(t, "sub/file.txt", handler.relPath(filepath.Join(root, "sub", "file.txt")))
}

// A relative path argument is resolved against the root directory, not against
// the process working directory.
func TestRelativePathIgnoresWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "inside.txt"), []byte("in root"), 0644))

	// Point the process at a different directory holding a same-named file
	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "inside.txt"), []byte("in cwd"), 0644))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(other))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	validPath, err := handler.validatePath("inside.txt")
	require.NoError(t, err)

	content, err := os.ReadFile(validPath)
	require.NoError(t, err)
	assert.Equal(t, "in root", string(content))
}

func TestValidatePathOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()

	handler, err := NewFilesystemHandler(dir)
	require.NoError(t, err)

	// An absolute path outside the root is rejected
	_, err = handler.validatePath(filepath.Join(other, "file.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied - path outside the allowed directory")

	// So is a relative path that escapes the root by traversal
	_, err = handler.validatePath("../file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied - path outside the allowed directory")
}
