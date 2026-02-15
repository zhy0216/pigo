package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validatePath ensures the path is absolute, clean, and resolves symlinks.
func validatePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	cleaned := filepath.Clean(absPath)

	// Resolve symlinks if the path exists, to prevent symlink traversal
	if _, err := os.Lstat(cleaned); err == nil {
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return "", fmt.Errorf("failed to resolve symlinks: %w", err)
		}
		return resolved, nil
	}

	return cleaned, nil
}

// formatWithLineNumbers adds line numbers to content (like cat -n).
func formatWithLineNumbers(content string, offset, limit int) string {
	if offset < 1 {
		offset = 1
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var result strings.Builder
	lineNum := 0
	linesOutput := 0

	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if limit > 0 && linesOutput >= limit {
			break
		}

		line := scanner.Text()
		// Truncate long lines
		if len(line) > 500 {
			line = line[:500] + "..."
		}
		fmt.Fprintf(&result, "%6d\t%s\n", lineNum, line)
		linesOutput++
	}

	return result.String()
}

// truncateOutput limits output length and adds truncation notice.
func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
}
