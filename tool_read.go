package main

import (
	"context"
	"fmt"
	"os"
)

// ReadTool reads file contents with line numbers.
type ReadTool struct {
	allowedDir string
}

// NewReadTool creates a new ReadTool. Files are restricted to allowedDir when non-empty.
func NewReadTool(allowedDir string) *ReadTool {
	return &ReadTool{allowedDir: allowedDir}
}

func (t *ReadTool) Name() string {
	return "read"
}

func (t *ReadTool) Description() string {
	return "Read the contents of a file with line numbers. Use offset and limit for large files."
}

func (t *ReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path to the file to read",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "Starting line number (1-indexed, default: 1)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of lines to read (default: all)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	path, err := extractString(args, "path")
	if err != nil {
		return ErrorResult(err.Error())
	}

	resolvedPath, err := validatePath(path, t.allowedDir)
	if err != nil {
		return ErrorResult(err.Error())
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("file not found: %s", path))
		}
		return ErrorResult(fmt.Sprintf("failed to stat file: %v", err))
	}

	if info.IsDir() {
		return ErrorResult(fmt.Sprintf("path is a directory: %s", path))
	}

	if info.Size() > maxReadFileSize {
		return ErrorResult(fmt.Sprintf("file is too large (%d bytes, max %d). Use offset and limit to read a portion", info.Size(), maxReadFileSize))
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	offset := extractInt(args, "offset", 1)
	limit := extractInt(args, "limit", 0)

	formatted := formatWithLineNumbers(string(content), offset, limit)
	return NewToolResult(formatted)
}
