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
	return `Edit a file. Two modes:
1. String replacement: provide old_string and new_string. The old_string must match exactly. Use all=true for multiple occurrences.
2. Line range replacement: provide start_line and end_line (1-based, inclusive) with new_string to replace those lines.`
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
				"description": "The exact text to find and replace (string replacement mode)",
			},
			"new_string": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace with",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "Start line number, 1-based inclusive (line range mode)",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "End line number, 1-based inclusive (line range mode)",
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Replace all occurrences (default: false, string replacement mode only)",
			},
		},
		"required": []string{"path", "new_string"},
	}
}

func (t *EditTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	path, err := util.ExtractString(args, "path")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	newString, err := util.ExtractString(args, "new_string")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

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

	// Determine mode: line range vs string replacement
	startLine := util.ExtractInt(args, "start_line", 0)
	endLine := util.ExtractInt(args, "end_line", 0)

	var newContent string
	var resultMsg string

	if startLine > 0 && endLine > 0 {
		newContent, resultMsg, err = t.editByLineRange(string(content), newString, startLine, endLine, path)
	} else {
		oldString, extractErr := util.ExtractString(args, "old_string")
		if extractErr != nil {
			return types.ErrorResult("either old_string or start_line+end_line must be provided")
		}
		replaceAll := util.ExtractBool(args, "all", false)
		newContent, resultMsg, err = t.editByStringReplace(string(content), oldString, newString, replaceAll, path)
	}

	if err != nil {
		return types.ErrorResult(err.Error())
	}

	if writeErr := t.fileOps.WriteFile(resolvedPath, []byte(newContent), info.Mode()); writeErr != nil {
		return types.ErrorResult(fmt.Sprintf("failed to write file: %v", writeErr))
	}

	return types.SilentResult(resultMsg)
}

// editByStringReplace performs the original old_string/new_string replacement.
func (t *EditTool) editByStringReplace(content, oldString, newString string, replaceAll bool, path string) (string, string, error) {
	if !strings.Contains(content, oldString) {
		return "", "", fmt.Errorf("old_string not found in file. Make sure it matches exactly")
	}

	count := strings.Count(content, oldString)
	if count > 1 && !replaceAll {
		return "", "", fmt.Errorf("old_string appears %d times. Use all=true to replace all, or provide more context to make it unique", count)
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldString, newString)
		return newContent, fmt.Sprintf("File edited: %s (%d replacements)", path, count), nil
	}

	newContent = strings.Replace(content, oldString, newString, 1)
	return newContent, fmt.Sprintf("File edited: %s", path), nil
}

// editByLineRange replaces lines start_line through end_line (1-based, inclusive) with newString.
func (t *EditTool) editByLineRange(content, newString string, startLine, endLine int, path string) (string, string, error) {
	if startLine < 1 {
		return "", "", fmt.Errorf("start_line must be >= 1, got %d", startLine)
	}
	if endLine < startLine {
		return "", "", fmt.Errorf("end_line (%d) must be >= start_line (%d)", endLine, startLine)
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if startLine > totalLines {
		return "", "", fmt.Errorf("start_line %d exceeds file length (%d lines)", startLine, totalLines)
	}
	if endLine > totalLines {
		return "", "", fmt.Errorf("end_line %d exceeds file length (%d lines)", endLine, totalLines)
	}

	// Build new content: lines before + new content + lines after
	var parts []string
	if startLine > 1 {
		parts = append(parts, strings.Join(lines[:startLine-1], "\n"))
	}

	if newString != "" {
		parts = append(parts, newString)
	}

	if endLine < totalLines {
		parts = append(parts, strings.Join(lines[endLine:], "\n"))
	}

	newContent := strings.Join(parts, "\n")
	replacedCount := endLine - startLine + 1
	return newContent, fmt.Sprintf("File edited: %s (replaced lines %d-%d, %d lines)", path, startLine, endLine, replacedCount), nil
}
