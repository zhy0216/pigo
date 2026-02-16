package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTool writes content to a file.
type WriteTool struct {
	allowedDir string
	fileOps    FileOps
}

// NewWriteTool creates a new WriteTool. Files are restricted to allowedDir when non-empty.
func NewWriteTool(allowedDir string, fileOps FileOps) *WriteTool {
	return &WriteTool{allowedDir: allowedDir, fileOps: fileOps}
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
	path, err := extractString(args, "path")
	if err != nil {
		return ErrorResult(err.Error())
	}

	content, err := extractString(args, "content")
	if err != nil {
		return ErrorResult(err.Error())
	}

	resolvedPath, err := validatePath(path, t.allowedDir)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Create parent directories if needed
	dir := filepath.Dir(resolvedPath)
	if err := t.fileOps.MkdirAll(dir, 0755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err))
	}

	// Preserve permissions if file already exists, otherwise use 0644
	perm := os.FileMode(0644)
	if info, err := t.fileOps.Stat(resolvedPath); err == nil {
		perm = info.Mode().Perm()
	}

	if err := t.fileOps.WriteFile(resolvedPath, []byte(content), perm); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}

	return SilentResult(fmt.Sprintf("File written: %s (%d bytes)", path, len(content)))
}
