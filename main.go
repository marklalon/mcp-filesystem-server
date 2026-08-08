package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-filesystem-server/filesystemserver"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// The server serves exactly one directory. It defaults to the working
	// directory the server was started in, which is the workspace directory
	// when an MCP client launches it, and can be overridden by a single
	// argument.
	if len(os.Args) > 2 {
		fmt.Fprintf(
			os.Stderr,
			"Usage: %s [directory]\n\nServes a single directory (default: the current working directory).\n",
			os.Args[0],
		)
		os.Exit(1)
	}

	rootDir := ""
	if len(os.Args) == 2 {
		rootDir = os.Args[1]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to resolve the current working directory: %v", err)
		}
		rootDir = cwd
	}

	// Create and start the server
	fss, err := filesystemserver.NewFilesystemServer(rootDir)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Serve requests
	if err := server.ServeStdio(fss); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
