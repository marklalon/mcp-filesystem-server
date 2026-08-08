package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	root, ignored, err := parseArgs([]string{
		"/workspace",
		"--ignore-dir",
		".git",
		"--ignore-directory=.cache",
	})
	if err != nil {
		t.Fatalf("parseArgs returned an error: %v", err)
	}
	if root != "/workspace" {
		t.Fatalf("root = %q, want %q", root, "/workspace")
	}
	if want := []string{".git", ".cache"}; !reflect.DeepEqual(ignored, want) {
		t.Fatalf("ignored = %#v, want %#v", ignored, want)
	}
}

func TestParseArgsRejectsExtraDirectory(t *testing.T) {
	_, _, err := parseArgs([]string{"/one", "/two"})
	if err == nil {
		t.Fatal("parseArgs accepted more than one workspace directory")
	}
}
