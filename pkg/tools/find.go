package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

// FindTool discovers files matching a glob pattern using fd or a Go fallback.
type FindTool struct {
	allowedDir string
	fileOps    ops.FileOps
	execOps    ops.ExecOps
}

// NewFindTool creates a new FindTool. Searches are restricted to allowedDir when non-empty.
func NewFindTool(allowedDir string, fileOps ops.FileOps, execOps ops.ExecOps) *FindTool {
	return &FindTool{allowedDir: allowedDir, fileOps: fileOps, execOps: execOps}
}

func (t *FindTool) Name() string {
	return "find"
}

func (t *FindTool) Description() string {
	return "Find files and directories matching a glob pattern. Returns relative paths, one per line."
}

func (t *FindTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to match (e.g., '*.go', 'test_*')",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory to search in (default: current directory)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Filter by type: 'file', 'directory', or 'both' (default: 'both')",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *FindTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	pattern, err := util.ExtractString(args, "pattern")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	searchPath := util.ExtractOptionalString(args, "path", ".")
	typeFilter := util.ExtractOptionalString(args, "type", "both")

	switch typeFilter {
	case "file", "directory", "both":
		// valid
	default:
		return types.ErrorResult(fmt.Sprintf("invalid type filter: %q (use 'file', 'directory', or 'both')", typeFilter))
	}

	resolvedPath, err := util.ValidatePath(searchPath, t.allowedDir)
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	info, err := t.fileOps.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return types.ErrorResult(fmt.Sprintf("path not found: %s", searchPath))
		}
		return types.ErrorResult(fmt.Sprintf("failed to access path: %v", err))
	}
	if !info.IsDir() {
		return types.ErrorResult(fmt.Sprintf("path is not a directory: %s", searchPath))
	}

	if fdPath, err := t.findFdBinary(); err == nil {
		return t.executeWithFd(ctx, fdPath, pattern, resolvedPath, typeFilter)
	}
	return t.executeNative(pattern, resolvedPath, typeFilter)
}

func (t *FindTool) findFdBinary() (string, error) {
	if p, err := t.execOps.LookPath("fd"); err == nil {
		return p, nil
	}
	return t.execOps.LookPath("fdfind")
}

func (t *FindTool) executeWithFd(ctx context.Context, fdPath, pattern, searchPath, typeFilter string) *types.ToolResult {
	args := []string{"--glob", "--max-results", fmt.Sprintf("%d", types.FindMaxResults)}

	switch typeFilter {
	case "file":
		args = append(args, "--type", "f")
	case "directory":
		args = append(args, "--type", "d")
	}

	args = append(args, pattern, searchPath)

	stdout, stderr, exitCode, err := t.execOps.Run(ctx, fdPath, args, nil)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("find error: %v", err))
	}

	if exitCode == 1 {
		return types.NewToolResult("No files found.")
	}
	if exitCode > 1 {
		if stderr != "" {
			return types.ErrorResult(fmt.Sprintf("find error: %s", stderr))
		}
		return types.ErrorResult(fmt.Sprintf("find error: exit code %d", exitCode))
	}

	if stdout == "" {
		return types.NewToolResult("No files found.")
	}

	return t.formatResults(stdout, searchPath)
}

func (t *FindTool) executeNative(pattern, searchPath, typeFilter string) *types.ToolResult {
	var result strings.Builder
	count := 0

	err := t.fileOps.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if name != "." && strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
		}

		switch typeFilter {
		case "file":
			if d.IsDir() {
				return nil
			}
		case "directory":
			if !d.IsDir() {
				return nil
			}
		}

		matched, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return fmt.Errorf("invalid glob pattern: %v", err)
		}
		if !matched {
			return nil
		}

		count++
		if count > types.FindMaxResults {
			result.WriteString(fmt.Sprintf("\n[results truncated at %d]", types.FindMaxResults))
			return fmt.Errorf("result limit reached")
		}

		relPath := util.RelativizePath(path, searchPath)
		line := relPath + "\n"
		if result.Len()+len(line) > types.FindMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", types.FindMaxBytes/1024))
			return fmt.Errorf("output limit reached")
		}
		result.WriteString(line)

		return nil
	})

	if count == 0 {
		if err != nil {
			return types.ErrorResult(fmt.Sprintf("find error: %v", err))
		}
		return types.NewToolResult("No files found.")
	}

	_ = err
	return types.NewToolResult(fmt.Sprintf("%d results:\n%s", count, result.String()))
}

func (t *FindTool) formatResults(output, basePath string) *types.ToolResult {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var result strings.Builder
	count := 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		count++
		relPath := util.RelativizePath(strings.TrimSpace(line), basePath)
		entry := relPath + "\n"
		if result.Len()+len(entry) > types.FindMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", types.FindMaxBytes/1024))
			break
		}
		result.WriteString(entry)
	}

	if count == 0 {
		return types.NewToolResult("No files found.")
	}

	return types.NewToolResult(fmt.Sprintf("%d results:\n%s", count, result.String()))
}
