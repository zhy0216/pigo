package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxReadFileSize is the maximum file size that ReadTool will load (10 MB).
	maxReadFileSize = 10 * 1024 * 1024

	// maxContextChars is the approximate character budget for the message history.
	maxContextChars = 200000

	// minKeepMessages is the minimum number of recent messages to preserve during truncation.
	minKeepMessages = 10
)

// sensitiveEnvPrefixes lists environment variable prefixes that should be
// stripped from child process environments to avoid leaking secrets.
var sensitiveEnvPrefixes = []string{
	"OPENAI_",
	"API_KEY",
	"SECRET",
	"TOKEN",
	"AWS_SECRET",
}

// validatePath ensures the path is absolute, clean, resolves symlinks, and
// optionally checks that the resolved path is under allowedDir.
// Pass allowedDir="" to disable the boundary check.
func validatePath(path, allowedDir string) (string, error) {
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
		cleaned = resolved
	}

	// Boundary check
	if allowedDir != "" {
		rel, err := filepath.Rel(allowedDir, cleaned)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("path %s is outside the allowed directory", path)
		}
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

// extractString extracts a required string parameter from args.
// Returns an error if the key is missing or not a string.
func extractString(args map[string]interface{}, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return s, nil
}

// extractOptionalString extracts an optional string parameter from args,
// returning defaultVal if the key is missing or not a string.
func extractOptionalString(args map[string]interface{}, key, defaultVal string) string {
	v, ok := args[key].(string)
	if !ok {
		return defaultVal
	}
	return v
}

// extractInt extracts an integer parameter from args, handling the JSON
// float64 -> int conversion. Returns defaultVal if missing or not a number.
func extractInt(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key].(float64)
	if !ok {
		return defaultVal
	}
	return int(v)
}

// extractBool extracts a boolean parameter from args, returning defaultVal
// if missing or not a bool.
func extractBool(args map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := args[key].(bool)
	if !ok {
		return defaultVal
	}
	return v
}

// sanitizeEnv returns a copy of os.Environ() with sensitive variables removed.
func sanitizeEnv() []string {
	var result []string
	for _, entry := range os.Environ() {
		key := entry
		if idx := strings.Index(entry, "="); idx >= 0 {
			key = entry[:idx]
		}
		upper := strings.ToUpper(key)
		sensitive := false
		for _, prefix := range sensitiveEnvPrefixes {
			if strings.HasPrefix(upper, prefix) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			result = append(result, entry)
		}
	}
	return result
}

// estimateMessageChars returns a rough character count across all messages.
func estimateMessageChars(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}
