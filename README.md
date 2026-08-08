# MCP Filesystem Server

This MCP server provides secure access to the local filesystem via the Model Context Protocol (MCP).

The server serves exactly one directory, the workspace directory. Every path argument is interpreted
relative to it, and every path the server reports back is relative to it as well, using forward
slashes on every platform. The workspace directory itself is `.`. Absolute paths are still accepted
as arguments, but are rejected if they fall outside the workspace directory.

## Components

### Resources

- **file://**
  - Name: File System
  - Description: Access to files and directories on the local file system

### Tools

#### File Operations

- **read_file**
  - Read the contents of a file from the file system, either in full or a range of lines
  - Parameters:
    - `path` (required): Path to the file to read
    - `start_line` (optional): First line to read, 1-based and inclusive (default: 1). Text files only
    - `end_line` (optional): Last line to read, 1-based and inclusive (default: last line of the file). Text files only

- **read_multiple_files**
  - Read the contents of multiple files in a single operation
  - Parameters: `paths` (required): List of file paths to read

- **write_file**
  - Create a new file or overwrite an existing file with new content
  - Parameters: `path` (required): Path where to write the file, `content` (required): Content to write to the file

- **copy_file**
  - Copy files and directories
  - Parameters: `source` (required): Source path of the file or directory, `destination` (required): Destination path

- **move_file**
  - Move or rename files and directories
  - Parameters: `source` (required): Source path of the file or directory, `destination` (required): Destination path

- **delete_file**
  - Delete a file or directory from the file system
  - Parameters: `path` (required): Path to the file or directory to delete, `recursive` (optional): Whether to recursively delete directories (default: false)

- **modify_file**
  - Update file by finding and replacing text using string matching or regex
  - Parameters: `path` (required): Path to the file to modify, `find` (required): Text to search for, `replace` (required): Text to replace with, `all_occurrences` (optional): Replace all occurrences (default: true), `regex` (optional): Treat find pattern as regex (default: false)

#### Directory Operations

- **list_directory**
  - Get a detailed listing of all files and directories in a specified path
  - Parameters: `path` (required): Path of the directory to list

- **create_directory**
  - Create a new directory or ensure a directory exists
  - Parameters: `path` (required): Path of the directory to create

- **tree**
  - Returns a hierarchical JSON representation of a directory structure
  - Parameters: `path` (required): Path of the directory to traverse, `depth` (optional): Maximum depth to traverse (default: 2), `follow_symlinks` (optional): Whether to follow symbolic links (default: false)
  - Individual files are only listed when `depth` is 1. For `depth` greater than 1 the result contains directories only, so a large tree cannot blow up the response. Any directory whose files are not listed — the directories at the bottom of a `depth` 1 tree included — carries a `file_count` field with the number of files directly inside it, so it is never mistaken for an empty one.

#### Search and Information

- **search_files**
  - Recursively search for files and directories matching a pattern
  - Parameters: `path` (required): Starting path for the search, `pattern` (required): Search pattern to match against file names

- **search_within_files**
  - Search for text within file contents across directory trees
  - Parameters: `path` (required): Starting directory for the search, `substring` (required): Text to search for within file contents, or a regular expression pattern when `regex` is true, `depth` (optional): Maximum directory depth to search, `max_results` (optional): Maximum number of results to return (default: 1000), `regex` (optional): Treat the pattern as a regular expression (default: false), `ignore_case` (optional): Match case-insensitively (default: false)
  - Regex patterns use Go's [RE2 syntax](https://github.com/google/re2/wiki/Syntax) and are matched against one line at a time, so `^` and `$` anchor to line boundaries and a pattern cannot span multiple lines. Note that a pattern able to match an empty string (such as `a*`) matches every line.

- **get_file_info**
  - Retrieve detailed metadata about a file or directory
  - Parameters: `path` (required): Path to the file or directory

## Features

- Secure access to a single workspace directory
- Relative paths in and out, so absolute paths never leak into results
- Path validation to prevent directory traversal attacks
- Symlink resolution with security checks
- MIME type detection
- Support for text, binary, and image files
- Size limits for inline content and base64 encoding

## Getting Started

### Installation

#### Using Go Install

```bash
go install github.com/mark3labs/mcp-filesystem-server@latest
```

### Usage

#### As a standalone server

Start the MCP server. It serves the current working directory by default:

```bash
mcp-filesystem-server
```

To serve a different directory, pass it as the single argument:

```bash
mcp-filesystem-server /path/to/workspace/directory
```

#### As a library in your Go project

```go
package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-filesystem-server/filesystemserver"
)

func main() {
	// Create a new filesystem server rooted at the workspace directory
	fs, err := filesystemserver.NewFilesystemServer("/path/to/workspace/directory")
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Serve requests
	if err := fs.Serve(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
```

### Usage with Model Context Protocol

To integrate this server with apps that support MCP:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "mcp-filesystem-server",
      "args": ["/path/to/workspace/directory"]
    }
  }
}
```

### Docker

#### Running with Docker

You can run the Filesystem MCP server using Docker:

```bash
docker run -i --rm ghcr.io/mark3labs/mcp-filesystem-server:latest /path/to/workspace/directory
```

#### Docker Configuration with MCP

To integrate the Docker image with apps that support MCP:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "ghcr.io/mark3labs/mcp-filesystem-server:latest",
        "/path/to/workspace/directory"
      ]
    }
  }
}
```

If you need changes made inside the container to reflect on the host filesystem, you can mount a volume. This allows the container to access and modify files on the host system. Here's an example:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "--volume=/workspace/directory/in/host:/workspace/directory/in/container",
        "ghcr.io/mark3labs/mcp-filesystem-server:latest",
        "/workspace/directory/in/container"
      ]
    }
  }
}
```

## License

See the [LICENSE](LICENSE) file for details.
