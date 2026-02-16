package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	lsMaxEntries = 1000
)

// LsTool lists directory contents with type indicators.
type LsTool struct {
	allowedDir string
}

// NewLsTool creates a new LsTool. Paths are restricted to allowedDir when non-empty.
func NewLsTool(allowedDir string) *LsTool {
	return &LsTool{allowedDir: allowedDir}
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
		"required": []string{"path"},
	}
}

func (t *LsTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	pathArg, err := extractString(args, "path")
	if err != nil || pathArg == "" {
		return ErrorResult("'path' parameter is required")
	}
	showAll := extractBool(args, "all", false)

	resolvedPath, err := validatePath(pathArg, t.allowedDir)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path error: %v", err))
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("path not found: %s", pathArg))
		}
		return ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if !info.IsDir() {
		return ErrorResult(fmt.Sprintf("not a directory: %s", pathArg))
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot read directory: %v", err))
	}

	var lines []string
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless -all is set
		if !showAll && strings.HasPrefix(name, ".") {
			continue
		}

		indicator := typeIndicator(entry)
		lines = append(lines, fmt.Sprintf("%s %s", indicator, name))

		if len(lines) >= lsMaxEntries {
			lines = append(lines, fmt.Sprintf("... (truncated, %d+ entries)", lsMaxEntries))
			break
		}
	}

	sort.Strings(lines)

	if len(lines) == 0 {
		return NewToolResult("(empty directory)")
	}

	// Relativize the path for display
	displayPath := pathArg
	if t.allowedDir != "" {
		if rel, err := filepath.Rel(t.allowedDir, resolvedPath); err == nil {
			displayPath = rel
		}
	}

	result := fmt.Sprintf("Directory: %s\n%s", displayPath, strings.Join(lines, "\n"))
	return NewToolResult(result)
}

// typeIndicator returns a short type indicator for a directory entry.
func typeIndicator(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "[link]"
	}
	if entry.IsDir() {
		return "[dir] "
	}
	return "[file]"
}
