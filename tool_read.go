package main

import (
	"context"
	"fmt"
	"os"
)

// ReadTool reads file contents with line numbers.
type ReadTool struct{}

// NewReadTool creates a new ReadTool.
func NewReadTool() *ReadTool {
	return &ReadTool{}
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
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	resolvedPath, err := validatePath(path)
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

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	offset := 1
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}

	limit := 0
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	formatted := formatWithLineNumbers(string(content), offset, limit)
	return NewToolResult(formatted)
}
