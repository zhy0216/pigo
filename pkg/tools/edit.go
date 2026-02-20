package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zhy0216/pigo/pkg/ops"
	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
)

// EditTool replaces text in a file.
type EditTool struct {
	allowedDir string
	fileOps    ops.FileOps
}

// NewEditTool creates a new EditTool. Files are restricted to allowedDir when non-empty.
func NewEditTool(allowedDir string, fileOps ops.FileOps) *EditTool {
	return &EditTool{allowedDir: allowedDir, fileOps: fileOps}
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

func (t *EditTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	path, err := util.ExtractString(args, "path")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	oldString, err := util.ExtractString(args, "old_string")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	newString, err := util.ExtractString(args, "new_string")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	replaceAll := util.ExtractBool(args, "all", false)

	resolvedPath, err := util.ValidatePath(path, t.allowedDir)
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	info, err := t.fileOps.Stat(resolvedPath)
	if os.IsNotExist(err) {
		return types.ErrorResult(fmt.Sprintf("file not found: %s", path))
	}
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to stat file: %v", err))
	}

	content, err := t.fileOps.ReadFile(resolvedPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, oldString) {
		return types.ErrorResult("old_string not found in file. Make sure it matches exactly")
	}

	count := strings.Count(contentStr, oldString)
	if count > 1 && !replaceAll {
		return types.ErrorResult(fmt.Sprintf("old_string appears %d times. Use all=true to replace all, or provide more context to make it unique", count))
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(contentStr, oldString, newString)
	} else {
		newContent = strings.Replace(contentStr, oldString, newString, 1)
	}

	if err := t.fileOps.WriteFile(resolvedPath, []byte(newContent), info.Mode()); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}

	if replaceAll {
		return types.SilentResult(fmt.Sprintf("File edited: %s (%d replacements)", path, count))
	}
	return types.SilentResult(fmt.Sprintf("File edited: %s", path))
}
