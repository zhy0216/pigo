package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

// LsTool lists directory contents with type indicators.
type LsTool struct {
	allowedDir string
	fileOps    ops.FileOps
}

// NewLsTool creates a new LsTool. Paths are restricted to allowedDir when non-empty.
func NewLsTool(allowedDir string, fileOps ops.FileOps) *LsTool {
	return &LsTool{allowedDir: allowedDir, fileOps: fileOps}
}

func (t *LsTool) Name() string {
	return "ls"
}

func (t *LsTool) Description() string {
	return "List directory contents with type indicators (file/dir/symlink). Returns entries one per line."
}

func (t *LsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to list. Defaults to current working directory.",
			},
			"all": map[string]interface{}{
				"type":        "boolean",
				"description": "Show hidden files (starting with '.') when true. Default: false.",
			},
		},
	}
}

func (t *LsTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	pathArg := util.ExtractOptionalString(args, "path", "")
	if pathArg == "" {
		if t.allowedDir != "" {
			pathArg = t.allowedDir
		} else {
			pathArg = "."
		}
	}
	showAll := util.ExtractBool(args, "all", false)

	resolvedPath, err := util.ValidatePath(pathArg, t.allowedDir)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("path error: %v", err))
	}

	info, err := t.fileOps.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return types.ErrorResult(fmt.Sprintf("path not found: %s", pathArg))
		}
		return types.ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if !info.IsDir() {
		return types.ErrorResult(fmt.Sprintf("not a directory: %s", pathArg))
	}

	entries, err := t.fileOps.ReadDir(resolvedPath)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("cannot read directory: %v", err))
	}

	var lines []string
	truncated := false
	for _, entry := range entries {
		name := entry.Name()

		if !showAll && strings.HasPrefix(name, ".") {
			continue
		}

		indicator := typeIndicator(entry)
		lines = append(lines, fmt.Sprintf("%s %s", indicator, name))

		if len(lines) >= types.LsMaxEntries {
			truncated = true
			break
		}
	}

	sort.Strings(lines)

	if truncated {
		lines = append(lines, fmt.Sprintf("... (truncated, %d+ entries)", types.LsMaxEntries))
	}

	if len(lines) == 0 {
		return types.NewToolResult("(empty directory)")
	}

	displayPath := pathArg
	if t.allowedDir != "" {
		if rel, err := filepath.Rel(t.allowedDir, resolvedPath); err == nil {
			displayPath = rel
		}
	}

	result := fmt.Sprintf("Directory: %s\n%s", displayPath, strings.Join(lines, "\n"))
	return types.NewToolResult(result)
}

func typeIndicator(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "[link]"
	}
	if entry.IsDir() {
		return "[dir] "
	}
	return "[file]"
}
