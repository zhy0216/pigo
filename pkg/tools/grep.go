package tools

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

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

// GrepTool searches files for patterns using ripgrep or a Go fallback.
type GrepTool struct {
	allowedDir string
	fileOps    ops.FileOps
	execOps    ops.ExecOps
}

// NewGrepTool creates a new GrepTool. Searches are restricted to allowedDir when non-empty.
func NewGrepTool(allowedDir string, fileOps ops.FileOps, execOps ops.ExecOps) *GrepTool {
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

func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	pattern, err := util.ExtractString(args, "pattern")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	searchPath := util.ExtractOptionalString(args, "path", ".")
	include := util.ExtractOptionalString(args, "include", "")
	contextLines := util.ExtractInt(args, "context_lines", 0)

	resolvedPath, err := util.ValidatePath(searchPath, t.allowedDir)
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	if _, err := t.fileOps.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			return types.ErrorResult(fmt.Sprintf("path not found: %s", searchPath))
		}
		return types.ErrorResult(fmt.Sprintf("failed to access path: %v", err))
	}

	if rgPath, err := t.execOps.LookPath("rg"); err == nil {
		return t.executeWithRg(ctx, rgPath, pattern, resolvedPath, include, contextLines)
	}
	return t.executeNative(pattern, resolvedPath, include, contextLines)
}

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

func (t *GrepTool) executeWithRg(ctx context.Context, rgPath, pattern, searchPath, include string, contextLines int) *types.ToolResult {
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
		return types.ErrorResult(fmt.Sprintf("grep error: %v", err))
	}

	if exitCode == 1 {
		return types.NewToolResult("No matches found.")
	}
	if exitCode > 1 {
		if stderr != "" {
			return types.ErrorResult(fmt.Sprintf("grep error: %s", stderr))
		}
		return types.ErrorResult(fmt.Sprintf("grep error: exit code %d", exitCode))
	}

	result, _ := t.parseRgStream(strings.NewReader(stdout), searchPath)
	return result
}

func (t *GrepTool) parseRgStream(r io.Reader, basePath string) (*types.ToolResult, bool) {
	var result strings.Builder
	matches := 0
	limitHit := false
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		if result.Len() >= types.GrepMaxBytes {
			result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", types.GrepMaxBytes/1024))
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
			if matches > types.GrepMaxMatches {
				result.WriteString(fmt.Sprintf("\n[matches truncated at %d]", types.GrepMaxMatches))
				limitHit = true
				break
			}
			line := strings.TrimRight(data.Lines.Text, "\n\r")
			if len(line) > types.GrepMaxLine {
				line = line[:types.GrepMaxLine] + "..."
			}
			relPath := util.RelativizePath(data.Path.Text, basePath)
			fmt.Fprintf(&result, "%s:%d: %s\n", relPath, data.LineNumber, line)

		case "context":
			var data rgMatchData
			if err := json.Unmarshal(entry.Data, &data); err != nil {
				continue
			}
			line := strings.TrimRight(data.Lines.Text, "\n\r")
			if len(line) > types.GrepMaxLine {
				line = line[:types.GrepMaxLine] + "..."
			}
			relPath := util.RelativizePath(data.Path.Text, basePath)
			fmt.Fprintf(&result, "%s:%d  %s\n", relPath, data.LineNumber, line)
		}
	}

	if matches == 0 {
		return types.NewToolResult("No matches found."), limitHit
	}

	return types.NewToolResult(fmt.Sprintf("%d matches:\n%s", matches, result.String())), limitHit
}

func (t *GrepTool) executeNative(pattern, searchPath, include string, contextLines int) *types.ToolResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return types.ErrorResult(fmt.Sprintf("invalid regex pattern: %v", err))
	}

	var includePattern string
	if include != "" {
		includePattern = include
	}

	var result strings.Builder
	matches := 0

	err = t.fileOps.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		if includePattern != "" {
			matched, _ := filepath.Match(includePattern, d.Name())
			if !matched {
				return nil
			}
		}

		if util.IsBinaryExtension(d.Name()) {
			return nil
		}

		if result.Len() >= types.GrepMaxBytes {
			return fmt.Errorf("output limit reached")
		}
		if matches >= types.GrepMaxMatches {
			return fmt.Errorf("match limit reached")
		}

		content, err := t.fileOps.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		relPath := util.RelativizePath(path, searchPath)

		for i, line := range lines {
			if re.MatchString(line) {
				matches++
				if matches > types.GrepMaxMatches {
					result.WriteString(fmt.Sprintf("\n[matches truncated at %d]", types.GrepMaxMatches))
					return fmt.Errorf("match limit reached")
				}
				displayLine := line
				if len(displayLine) > types.GrepMaxLine {
					displayLine = displayLine[:types.GrepMaxLine] + "..."
				}
				fmt.Fprintf(&result, "%s:%d: %s\n", relPath, i+1, displayLine)

				if result.Len() >= types.GrepMaxBytes {
					result.WriteString(fmt.Sprintf("\n[output truncated at %dKB]", types.GrepMaxBytes/1024))
					return fmt.Errorf("output limit reached")
				}
			}
		}

		return nil
	})

	if matches == 0 {
		return types.NewToolResult("No matches found.")
	}

	_ = err
	return types.NewToolResult(fmt.Sprintf("%d matches:\n%s", matches, result.String()))
}
