package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EditTool replaces text in a file.
type EditTool struct{}

// NewEditTool creates a new EditTool.
func NewEditTool() *EditTool {
	return &EditTool{}
}

func (t *EditTool) Name() string {
	return "edit"
}

func (t *EditTool) Description() string {
	return "Edit a file by replacing old_string with new_string. The old_string must exist exactly in the file. Use all=true to replace all occurrences."
}

func (t *EditTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path to the file to edit",
			},
			"old_string": map[string]interface{}{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default: false)",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *EditTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	oldString, ok := args["old_string"].(string)
	if !ok {
		return ErrorResult("old_string is required")
	}

	newString, ok := args["new_string"].(string)
	if !ok {
		return ErrorResult("new_string is required")
	}

	replaceAll := false
	if v, ok := args["all"].(bool); ok {
		replaceAll = v
	}

	resolvedPath, err := validatePath(path)
	if err != nil {
		return ErrorResult(err.Error())
	}

	if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("file not found: %s", path))
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, oldString) {
		return ErrorResult("old_string not found in file. Make sure it matches exactly")
	}

	count := strings.Count(contentStr, oldString)
	if count > 1 && !replaceAll {
		return ErrorResult(fmt.Sprintf("old_string appears %d times. Use all=true to replace all, or provide more context to make it unique", count))
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(contentStr, oldString, newString)
	} else {
		newContent = strings.Replace(contentStr, oldString, newString, 1)
	}

	if err := os.WriteFile(resolvedPath, []byte(newContent), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}

	if replaceAll {
		return SilentResult(fmt.Sprintf("File edited: %s (%d replacements)", path, count))
	}
	return SilentResult(fmt.Sprintf("File edited: %s", path))
}
