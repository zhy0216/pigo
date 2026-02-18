package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

// ReadTool reads file contents with line numbers.
type ReadTool struct {
	allowedDir string
	fileOps    ops.FileOps
}

// NewReadTool creates a new ReadTool. Files are restricted to allowedDir when non-empty.
func NewReadTool(allowedDir string, fileOps ops.FileOps) *ReadTool {
	return &ReadTool{allowedDir: allowedDir, fileOps: fileOps}
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

func (t *ReadTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	path, err := util.ExtractString(args, "path")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	resolvedPath, err := util.ValidatePath(path, t.allowedDir)
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	info, err := t.fileOps.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return types.ErrorResult(fmt.Sprintf("file not found: %s", path))
		}
		return types.ErrorResult(fmt.Sprintf("failed to stat file: %v", err))
	}

	if info.IsDir() {
		return types.ErrorResult(fmt.Sprintf("path is a directory: %s", path))
	}

	if info.Size() > types.MaxReadFileSize {
		return types.ErrorResult(fmt.Sprintf("file is too large (%d bytes, max %d). Use offset and limit to read a portion", info.Size(), types.MaxReadFileSize))
	}

	content, err := t.fileOps.ReadFile(resolvedPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to read file: %v", err))
	}

	offset := util.ExtractInt(args, "offset", 1)
	limit := util.ExtractInt(args, "limit", 0)

	formatted := util.FormatWithLineNumbers(string(content), offset, limit)
	return types.NewToolResult(formatted)
}
