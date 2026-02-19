package util

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/pigo/pkg/types"
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

// ValidatePath ensures the path is absolute, clean, resolves symlinks, and
// optionally checks that the resolved path is under allowedDir.
// Pass allowedDir="" to disable the boundary check.
func ValidatePath(path, allowedDir string) (string, error) {
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

// FormatWithLineNumbers adds line numbers to content (like cat -n).
func FormatWithLineNumbers(content string, offset, limit int) string {
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
		if len(line) > types.MaxLineLength {
			line = line[:types.MaxLineLength] + "..."
		}
		fmt.Fprintf(&result, "%6d\t%s\n", lineNum, line)
		linesOutput++
	}

	return result.String()
}

// FormatWithLineNumbersFromReader formats lines from a reader with line numbers.
// It truncates long lines to MaxLineLength and supports offset/limit without
// reading the full input into memory.
func FormatWithLineNumbersFromReader(r io.Reader, offset, limit int) (string, error) {
	if offset < 1 {
		offset = 1
	}

	reader := bufio.NewReader(r)
	var result strings.Builder
	lineNum := 0
	linesOutput := 0

	for {
		line, truncated, err := readLineWithLimit(reader, types.MaxLineLength)
		if err != nil && err != io.EOF {
			return result.String(), err
		}
		if err == io.EOF && line == "" && !truncated {
			break
		}

		lineNum++
		if lineNum >= offset {
			if limit > 0 && linesOutput >= limit {
				break
			}
			if truncated {
				line += "..."
			}
			fmt.Fprintf(&result, "%6d\t%s\n", lineNum, line)
			linesOutput++
		}

		if err == io.EOF {
			break
		}
	}

	return result.String(), nil
}

func readLineWithLimit(r *bufio.Reader, maxLen int) (string, bool, error) {
	var buf bytes.Buffer
	truncated := false

	for {
		frag, err := r.ReadSlice('\n')
		if err != nil && err != bufio.ErrBufferFull && err != io.EOF {
			return buf.String(), truncated, err
		}

		if len(frag) > 0 {
			if frag[len(frag)-1] == '\n' {
				frag = frag[:len(frag)-1]
				if len(frag) > 0 && frag[len(frag)-1] == '\r' {
					frag = frag[:len(frag)-1]
				}
			}

			if !truncated && maxLen > 0 {
				remaining := maxLen - buf.Len()
				if remaining > 0 {
					if len(frag) > remaining {
						buf.Write(frag[:remaining])
						truncated = true
					} else {
						buf.Write(frag)
					}
				} else {
					truncated = true
				}
			}
		}

		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if buf.Len() == 0 && len(frag) == 0 && !truncated {
				return "", false, io.EOF
			}
			return buf.String(), truncated, io.EOF
		}

		return buf.String(), truncated, nil
	}
}

// TruncateOutput limits output length by keeping the head and adds truncation notice.
func TruncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
}

// TruncateTail limits output length by keeping the tail (last maxLen chars).
// Useful for bash output where errors and exit status appear at the end.
func TruncateTail(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return fmt.Sprintf("[output truncated: showing last %d of %d chars]\n", maxLen, len(output)) + output[len(output)-maxLen:]
}

// ExtractString extracts a required string parameter from args.
// Returns an error if the key is missing or not a string.
func ExtractString(args map[string]interface{}, key string) (string, error) {
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

// ExtractOptionalString extracts an optional string parameter from args,
// returning defaultVal if the key is missing or not a string.
func ExtractOptionalString(args map[string]interface{}, key, defaultVal string) string {
	v, ok := args[key].(string)
	if !ok {
		return defaultVal
	}
	return v
}

// ExtractInt extracts an integer parameter from args, handling the JSON
// float64 -> int conversion. Returns defaultVal if missing or not a number.
func ExtractInt(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key].(float64)
	if !ok {
		return defaultVal
	}
	return int(v)
}

// ExtractBool extracts a boolean parameter from args, returning defaultVal
// if missing or not a bool.
func ExtractBool(args map[string]interface{}, key string, defaultVal bool) bool {
	v, ok := args[key].(bool)
	if !ok {
		return defaultVal
	}
	return v
}

// SanitizeEnv returns a copy of os.Environ() with sensitive variables removed.
func SanitizeEnv() []string {
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

// EstimateMessageChars returns a rough character count across all messages.
func EstimateMessageChars(messages []types.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}

// GetEnvOrDefault returns environment variable or default value.
func GetEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// RelativizePath returns path relative to basePath, or path unchanged on error.
func RelativizePath(path, basePath string) string {
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return path
	}
	return rel
}

// IsBinaryExtension returns true for common binary file extensions.
func IsBinaryExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".exe", ".bin", ".so", ".dylib", ".dll", ".o", ".a",
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z",
		".pdf", ".wasm", ".pyc", ".class":
		return true
	}
	return false
}

// StripCodeFence removes markdown code fences from LLM responses.
func StripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

// MapKeys returns the keys of a map as a slice.
func MapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// XmlEscape escapes special XML characters.
func XmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
