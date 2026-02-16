package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
}

// NewFindTool creates a new FindTool. Searches are restricted to allowedDir when non-empty.
func NewFindTool(allowedDir string) *FindTool {
	return &FindTool{allowedDir: allowedDir}
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
	info, err := os.Stat(resolvedPath)
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
	if fdPath, err := findFdBinary(); err == nil {
		return t.executeWithFd(ctx, fdPath, pattern, resolvedPath, typeFilter)
	}
	return t.executeNative(pattern, resolvedPath, typeFilter)
}

// findFdBinary looks for fd or fdfind (Debian/Ubuntu name) in PATH.
func findFdBinary() (string, error) {
	if p, err := exec.LookPath("fd"); err == nil {
		return p, nil
	}
	return exec.LookPath("fdfind")
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

	cmd := exec.CommandContext(ctx, fdPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return NewToolResult("No files found.")
		}
		if stderr.Len() > 0 {
			return ErrorResult(fmt.Sprintf("find error: %s", stderr.String()))
		}
		return ErrorResult(fmt.Sprintf("find error: %v", err))
	}

	output := stdout.String()
	if output == "" {
		return NewToolResult("No files found.")
	}

	// Relativize paths and apply byte limit
	return t.formatResults(output, searchPath)
}

func (t *FindTool) executeNative(pattern, searchPath, typeFilter string) *ToolResult {
	var result strings.Builder
	count := 0

	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
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
