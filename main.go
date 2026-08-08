package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mark3labs/mcp-filesystem-server/filesystemserver"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	rootDir, ignoredDirs, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Serves a single directory (default: the current working directory).")
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintln(os.Stderr, "  --ignore-dir <patterns> Deny access to matching directories; separate patterns with | (repeatable).")
		os.Exit(1)
	}

	if rootDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to resolve the current working directory: %v", err)
		}
		rootDir = cwd
	}

	// Create and start the server
	fss, err := filesystemserver.NewFilesystemServer(rootDir, ignoredDirs...)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Serve requests
	if err := server.ServeStdio(fss); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// parseArgs accepts the workspace directory as a positional argument and
// allows ignore flags before or after it. Keeping this small parser instead of
// using flag.FlagSet preserves the existing `server /path/to/workspace`
// invocation while also supporting `server /path --ignore-dir .git`.
func parseArgs(args []string) (string, []string, error) {
	var rootDir string
	var ignoredDirs []string
	optionsEnabled := true

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if optionsEnabled && arg == "--" {
			optionsEnabled = false
			continue
		}

		if optionsEnabled && (arg == "--ignore-dir" || arg == "--ignore-directory") {
			if i+1 >= len(args) || args[i+1] == "" {
				return "", nil, fmt.Errorf("missing value for %s", arg)
			}
			ignoredDirs = append(ignoredDirs, args[i+1])
			i++
			continue
		}

		if optionsEnabled && (strings.HasPrefix(arg, "--ignore-dir=") || strings.HasPrefix(arg, "--ignore-directory=")) {
			value := arg[strings.IndexByte(arg, '=')+1:]
			if value == "" {
				return "", nil, fmt.Errorf("missing value for %s", strings.SplitN(arg, "=", 2)[0])
			}
			ignoredDirs = append(ignoredDirs, value)
			continue
		}

		if optionsEnabled && strings.HasPrefix(arg, "-") {
			return "", nil, fmt.Errorf("unknown option: %s", arg)
		}

		if rootDir != "" {
			return "", nil, fmt.Errorf("only one workspace directory may be specified")
		}
		rootDir = arg
	}

	return rootDir, ignoredDirs, nil
}
