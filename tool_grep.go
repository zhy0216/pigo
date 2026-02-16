package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	grepMaxMatches = 100
	grepMaxBytes   = 50 * 1024 // 50KB
	grepMaxLine    = 500
)

// GrepTool searches files for patterns using ripgrep or a Go fallback.
type GrepTool struct {
	allowedDir string
	fileOps    FileOps
	execOps    ExecOps
}

// NewGrepTool creates a new GrepTool. Searches are restricted to allowedDir when non-empty.
func NewGrepTool(allowedDir string, fileOps FileOps, execOps ExecOps) *GrepTool {
	return &GrepTool{allowedDir: allowedDir, fileOps: fileOps, execOps: execOps}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Description() string {
	return "Search file contents for a regex pattern. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Regex pattern to search for",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory or file to search (default: current directory)",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g., '*.go', '*.js')",
			},
			"context_lines": map[string]interface{}{
				"type":        "integer",
				"description": "Number of context lines before and after each match (default: 0)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	pattern, err := extractString(args, "pattern")
	if err != nil {
		return ErrorResult(err.Error())
	}

	searchPath := extractOptionalString(args, "path", ".")
	include := extractOptionalString(args, "include", "")
	contextLines := extractInt(args, "context_lines", 0)

	// Resolve and validate the search path
	resolvedPath, err := validatePath(searchPath, t.allowedDir)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Check if path exists
	if _, err := t.fileOps.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("path not found: %s", searchPath))
		}
		return ErrorResult(fmt.Sprintf("failed to access path: %v", err))
	}

	// Try ripgrep first, fall back to Go native
	if rgPath, err := t.execOps.LookPath("rg"); err == nil {
		return t.executeWithRg(ctx, rgPath, pattern, resolvedPath, include, contextLines)
	}
	return t.executeNative(pattern, resolvedPath, include, contextLines)
}

// rgMatch represents a parsed ripgrep JSON match.
type rgMatch struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rgMatchData struct {
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	LineNumber int `json:"line_number"`
}

func (t *GrepTool) executeWithRg(ctx context.Context, rgPath, pattern, searchPath, include string, contextLines int) *ToolResult {
	args := []string{"--json"}
	if contextLines > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", contextLines))
	}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, pattern, searchPath)

	stdout, stderr, exitCode, err := t.execOps.Run(ctx, rgPath, args, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("grep error: %v", err))
	}

	if exitCode == 1 {
		return NewToolResult("No matches found.")
	}
	if exitCode > 1 {
		if stderr != "" {
			return ErrorResult(fmt.Sprintf("grep error: %s", stderr))
		}
		return ErrorResult(fmt.Sprintf("grep error: exit code %d", exitCode))
	}

	// Parse the buffered JSON output
	result, _ := t.parseRgStream(strings.NewReader(stdout), searchPath)
	return result
}

// parseRgStream reads rg JSON output from r, collecting matches up to the
// configured limits. It returns the result and whether the match/byte limit
// was reached (signalling the caller should kill the rg process).
func (t *GrepTool) parseRgStream(r io.Reader, basePath string) (*ToolResult, bool) {
	var result strings.Builder
	matches := 0
	limitHit := false
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		if result.Len() >= grepMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", grepMaxBytes/1024))
			limitHit = true
			break
		}

		var entry rgMatch
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "match":
			var data rgMatchData
			if err := json.Unmarshal(entry.Data, &data); err != nil {
				continue
			}
			matches++
			if matches > grepMaxMatches {
				result.WriteString(fmt.Sprintf("\n[matches truncated at %d]", grepMaxMatches))
				limitHit = true
				break
			}
			line := strings.TrimRight(data.Lines.Text, "\n\r")
			if len(line) > grepMaxLine {
				line = line[:grepMaxLine] + "..."
			}
			relPath := relativizePath(data.Path.Text, basePath)
			fmt.Fprintf(&result, "%s:%d: %s\n", relPath, data.LineNumber, line)

		case "context":
			var data rgMatchData
			if err := json.Unmarshal(entry.Data, &data); err != nil {
				continue
			}
			line := strings.TrimRight(data.Lines.Text, "\n\r")
			if len(line) > grepMaxLine {
				line = line[:grepMaxLine] + "..."
			}
			relPath := relativizePath(data.Path.Text, basePath)
			fmt.Fprintf(&result, "%s:%d  %s\n", relPath, data.LineNumber, line)
		}
	}

	if matches == 0 {
		return NewToolResult("No matches found."), limitHit
	}

	return NewToolResult(fmt.Sprintf("%d matches:\n%s", matches, result.String())), limitHit
}

func (t *GrepTool) executeNative(pattern, searchPath, include string, contextLines int) *ToolResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid regex pattern: %v", err))
	}

	var includePattern string
	if include != "" {
		includePattern = include
	}

	var result strings.Builder
	matches := 0

	err = t.fileOps.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply include filter
		if includePattern != "" {
			matched, _ := filepath.Match(includePattern, d.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files (check extension)
		if isBinaryExtension(d.Name()) {
			return nil
		}

		// Check byte budget
		if result.Len() >= grepMaxBytes {
			return fmt.Errorf("output limit reached")
		}
		if matches >= grepMaxMatches {
			return fmt.Errorf("match limit reached")
		}

		content, err := t.fileOps.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		relPath := relativizePath(path, searchPath)

		for i, line := range lines {
			if re.MatchString(line) {
				matches++
				if matches > grepMaxMatches {
					result.WriteString(fmt.Sprintf("\n[matches truncated at %d]", grepMaxMatches))
					return fmt.Errorf("match limit reached")
				}
				displayLine := line
				if len(displayLine) > grepMaxLine {
					displayLine = displayLine[:grepMaxLine] + "..."
				}
				fmt.Fprintf(&result, "%s:%d: %s\n", relPath, i+1, displayLine)

				if result.Len() >= grepMaxBytes {
					result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", grepMaxBytes/1024))
					return fmt.Errorf("output limit reached")
				}
			}
		}

		return nil
	})

	if matches == 0 {
		return NewToolResult("No matches found.")
	}

	_ = err // limits are handled via early returns
	return NewToolResult(fmt.Sprintf("%d matches:\n%s", matches, result.String()))
}

// relativizePath returns path relative to basePath, or path unchanged on error.
func relativizePath(path, basePath string) string {
	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return path
	}
	return rel
}

// isBinaryExtension returns true for common binary file extensions.
func isBinaryExtension(name string) bool {
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
