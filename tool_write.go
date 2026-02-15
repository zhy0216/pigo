package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTool writes content to a file.
type WriteTool struct{}

// NewWriteTool creates a new WriteTool.
func NewWriteTool() *WriteTool {
	return &WriteTool{}
}

func (t *WriteTool) Name() string {
	return "write"
}

func (t *WriteTool) Description() string {
	return "Write content to a file. Creates directories if needed. Overwrites existing files."
}

func (t *WriteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return ErrorResult("content is required")
	}

	resolvedPath, err := validatePath(path)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Create parent directories if needed
	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err))
	}

	// Preserve permissions if file already exists, otherwise use 0644
	perm := os.FileMode(0644)
	if info, err := os.Stat(resolvedPath); err == nil {
		perm = info.Mode().Perm()
	}

	if err := os.WriteFile(resolvedPath, []byte(content), perm); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}

	return SilentResult(fmt.Sprintf("File written: %s (%d bytes)", path, len(content)))
}
