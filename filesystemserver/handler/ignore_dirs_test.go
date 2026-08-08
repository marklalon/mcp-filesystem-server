package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestIgnoredDirectoriesAreExcludedAndInaccessible(t *testing.T) {
	root := t.TempDir()
	makeDir := func(path string) {
		t.Helper()
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("create directory %s: %v", path, err)
		}
	}
	makeFile := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("needle"), 0644); err != nil {
			t.Fatalf("create file %s: %v", path, err)
		}
	}

	makeDir(filepath.Join(root, ".git"))
	makeFile(filepath.Join(root, ".git", "config"))
	makeDir(filepath.Join(root, ".cache", "nested"))
	makeFile(filepath.Join(root, ".cache", "nested", "cache.txt"))
	makeDir(filepath.Join(root, "src"))
	makeFile(filepath.Join(root, "src", "main.go"))

	fs, err := NewFilesystemHandler(root, ".*")
	if err != nil {
		t.Fatalf("NewFilesystemHandler returned an error: %v", err)
	}

	request := func(name string, arguments map[string]any) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      name,
				Arguments: arguments,
			},
		}
	}

	listResult, err := fs.HandleListDirectory(context.Background(), request("list_directory", map[string]any{
		"path": root,
	}))
	if err != nil {
		t.Fatalf("list directory returned an error: %v", err)
	}
	listText := listResult.Content[0].(mcp.TextContent).Text
	if strings.Contains(listText, ".git") || strings.Contains(listText, ".cache") {
		t.Fatalf("list included ignored directories: %s", listText)
	}
	if !strings.Contains(listText, "src") {
		t.Fatalf("list omitted non-ignored directory: %s", listText)
	}

	searchResult, err := fs.HandleSearchFiles(context.Background(), request("search_files", map[string]any{
		"path":    root,
		"pattern": "*.txt",
	}))
	if err != nil {
		t.Fatalf("search files returned an error: %v", err)
	}
	searchText := searchResult.Content[0].(mcp.TextContent).Text
	if strings.Contains(searchText, "cache.txt") {
		t.Fatalf("search included an ignored directory: %s", searchText)
	}

	treeResult, err := fs.HandleTree(context.Background(), request("tree", map[string]any{
		"path":  root,
		"depth": 3.0,
	}))
	if err != nil {
		t.Fatalf("tree returned an error: %v", err)
	}
	treeText := treeResult.Content[0].(mcp.TextContent).Text
	if strings.Contains(treeText, ".git") || strings.Contains(treeText, ".cache") {
		t.Fatalf("tree included ignored directories: %s", treeText)
	}

	readResult, err := fs.HandleReadFile(context.Background(), request("read_file", map[string]any{
		"path": filepath.Join(".git", "config"),
	}))
	if err != nil {
		t.Fatalf("read file returned an error instead of a tool result: %v", err)
	}
	if !readResult.IsError {
		t.Fatal("read_file allowed access to an ignored directory")
	}

	writeResult, err := fs.HandleWriteFile(context.Background(), request("write_file", map[string]any{
		"path":    filepath.Join(".git", "new-file"),
		"content": "should not be written",
	}))
	if err != nil {
		t.Fatalf("write file returned an error instead of a tool result: %v", err)
	}
	if !writeResult.IsError {
		t.Fatal("write_file allowed access to an ignored directory")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "new-file")); !os.IsNotExist(err) {
		t.Fatal("write_file created a file inside an ignored directory")
	}
}

func TestCopyAndMoveToIgnoredNameAreBlocked(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("create source: %v", err)
	}

	fs, err := NewFilesystemHandler(root, ".*")
	if err != nil {
		t.Fatalf("NewFilesystemHandler returned an error: %v", err)
	}

	request := func(name string, arguments map[string]any) mcp.CallToolRequest {
		return mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      name,
				Arguments: arguments,
			},
		}
	}

	copyResult, err := fs.HandleCopyFile(context.Background(), request("copy_file", map[string]any{
		"source":      "src.txt",
		"destination": ".git",
	}))
	if err != nil {
		t.Fatalf("copy_file returned an error instead of a tool result: %v", err)
	}
	if !copyResult.IsError {
		t.Fatal("copy_file allowed writing to an ignored directory name")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("copy_file created a path with an ignored directory name")
	}

	moveResult, err := fs.HandleMoveFile(context.Background(), request("move_file", map[string]any{
		"source":      "src.txt",
		"destination": ".cache",
	}))
	if err != nil {
		t.Fatalf("move_file returned an error instead of a tool result: %v", err)
	}
	if !moveResult.IsError {
		t.Fatal("move_file allowed writing to an ignored directory name")
	}
	if _, err := os.Stat(filepath.Join(root, ".cache")); !os.IsNotExist(err) {
		t.Fatal("move_file created a path with an ignored directory name")
	}
	if _, err := os.Stat(filepath.Join(root, "src.txt")); err != nil {
		t.Fatalf("source should remain in place after blocked move: %v", err)
	}
}

func TestInvalidIgnoredDirectoryPattern(t *testing.T) {
	if _, err := NewFilesystemHandler(t.TempDir(), "["); err == nil {
		t.Fatal("invalid ignored directory pattern was accepted")
	}
}

func TestCombinedIgnoredDirectoryPatterns(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".archive", ".chat", "visible"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatalf("create directory %s: %v", name, err)
		}
	}

	fs, err := NewFilesystemHandler(root, ".archive|.chat")
	if err != nil {
		t.Fatalf("NewFilesystemHandler returned an error: %v", err)
	}

	for _, name := range []string{".archive", ".chat"} {
		if _, err := fs.validatePath(name); err == nil {
			t.Fatalf("validatePath allowed combined ignored directory %s", name)
		}
	}
	if _, err := fs.validatePath("visible"); err != nil {
		t.Fatalf("validatePath rejected non-ignored directory: %v", err)
	}
}
