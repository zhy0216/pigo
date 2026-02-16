package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	findMaxResults = 1000
	findMaxBytes   = 50 * 1024 // 50KB
)

// FindTool discovers files matching a glob pattern using fd or a Go fallback.
type FindTool struct {
	allowedDir string
	fileOps    FileOps
	execOps    ExecOps
}

// NewFindTool creates a new FindTool. Searches are restricted to allowedDir when non-empty.
func NewFindTool(allowedDir string, fileOps FileOps, execOps ExecOps) *FindTool {
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

func (t *FindTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	pattern, err := extractString(args, "pattern")
	if err != nil {
		return ErrorResult(err.Error())
	}

	searchPath := extractOptionalString(args, "path", ".")
	typeFilter := extractOptionalString(args, "type", "both")

	// Validate type filter
	switch typeFilter {
	case "file", "directory", "both":
		// valid
	default:
		return ErrorResult(fmt.Sprintf("invalid type filter: %q (use 'file', 'directory', or 'both')", typeFilter))
	}

	// Resolve and validate the search path
	resolvedPath, err := validatePath(searchPath, t.allowedDir)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Check if path exists
	info, err := t.fileOps.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("path not found: %s", searchPath))
		}
		return ErrorResult(fmt.Sprintf("failed to access path: %v", err))
	}
	if !info.IsDir() {
		return ErrorResult(fmt.Sprintf("path is not a directory: %s", searchPath))
	}

	// Try fd/fdfind first, fall back to Go native
	if fdPath, err := t.findFdBinary(); err == nil {
		return t.executeWithFd(ctx, fdPath, pattern, resolvedPath, typeFilter)
	}
	return t.executeNative(pattern, resolvedPath, typeFilter)
}

// findFdBinary looks for fd or fdfind (Debian/Ubuntu name) in PATH.
func (t *FindTool) findFdBinary() (string, error) {
	if p, err := t.execOps.LookPath("fd"); err == nil {
		return p, nil
	}
	return t.execOps.LookPath("fdfind")
}

func (t *FindTool) executeWithFd(ctx context.Context, fdPath, pattern, searchPath, typeFilter string) *ToolResult {
	args := []string{"--glob", "--max-results", fmt.Sprintf("%d", findMaxResults)}

	switch typeFilter {
	case "file":
		args = append(args, "--type", "f")
	case "directory":
		args = append(args, "--type", "d")
	}

	args = append(args, pattern, searchPath)

	stdout, stderr, exitCode, err := t.execOps.Run(ctx, fdPath, args, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("find error: %v", err))
	}

	if exitCode == 1 {
		return NewToolResult("No files found.")
	}
	if exitCode > 1 {
		if stderr != "" {
			return ErrorResult(fmt.Sprintf("find error: %s", stderr))
		}
		return ErrorResult(fmt.Sprintf("find error: exit code %d", exitCode))
	}

	if stdout == "" {
		return NewToolResult("No files found.")
	}

	// Relativize paths and apply byte limit
	return t.formatResults(stdout, searchPath)
}

func (t *FindTool) executeNative(pattern, searchPath, typeFilter string) *ToolResult {
	var result strings.Builder
	count := 0

	err := t.fileOps.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		// Skip hidden directories and common noise
		if d.IsDir() {
			name := d.Name()
			if name != "." && strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
		}

		// Apply type filter
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

		// Match against pattern
		matched, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return fmt.Errorf("invalid glob pattern: %v", err)
		}
		if !matched {
			return nil
		}

		count++
		if count > findMaxResults {
			result.WriteString(fmt.Sprintf("\n[results truncated at %d]", findMaxResults))
			return fmt.Errorf("result limit reached")
		}

		relPath := relativizePath(path, searchPath)
		line := relPath + "\n"
		if result.Len()+len(line) > findMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", findMaxBytes/1024))
			return fmt.Errorf("output limit reached")
		}
		result.WriteString(line)

		return nil
	})

	if count == 0 {
		if err != nil {
			return ErrorResult(fmt.Sprintf("find error: %v", err))
		}
		return NewToolResult("No files found.")
	}

	_ = err // limits handled via early returns
	return NewToolResult(fmt.Sprintf("%d results:\n%s", count, result.String()))
}

func (t *FindTool) formatResults(output, basePath string) *ToolResult {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var result strings.Builder
	count := 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		count++
		relPath := relativizePath(strings.TrimSpace(line), basePath)
		entry := relPath + "\n"
		if result.Len()+len(entry) > findMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", findMaxBytes/1024))
			break
		}
		result.WriteString(entry)
	}

	if count == 0 {
		return NewToolResult("No files found.")
	}

	return NewToolResult(fmt.Sprintf("%d results:\n%s", count, result.String()))
}
