package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

func TestGrepTool_Name(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	if tool.Name() != "grep" {
		t.Errorf("expected name 'grep', got %q", tool.Name())
	}
}

func TestGrepTool_Parameters(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	params := tool.Parameters()
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "pattern" {
		t.Errorf("expected required=[pattern], got %v", required)
	}
}

func TestGrepTool_MissingPattern(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing pattern")
	}
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	writeTestFile(t, dir, "test.txt", "hello world\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "[invalid",
		"path":    dir,
	})
	_ = result
}

func TestGrepTool_NativeSearch(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())

	writeTestFile(t, dir, "file1.go", "package main\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n")
	writeTestFile(t, dir, "file2.go", "package main\n\nfunc World() string {\n\treturn \"world\"\n}\n")
	writeTestFile(t, dir, "readme.txt", "This is a readme\nNo functions here\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "func.*\\(\\)",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Hello") {
		t.Error("expected match for Hello")
	}
	if !strings.Contains(result.ForLLM, "World") {
		t.Error("expected match for World")
	}
}

func TestGrepTool_IncludeFilter(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())

	writeTestFile(t, dir, "file.go", "hello world\n")
	writeTestFile(t, dir, "file.txt", "hello world\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "hello",
		"path":    dir,
		"include": "*.go",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "file.go") {
		t.Error("expected file.go in results")
	}
	if strings.Contains(result.ForLLM, "file.txt") {
		t.Error("file.txt should not appear with *.go include filter")
	}
}

func TestGrepTool_NoMatches(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	writeTestFile(t, dir, "test.txt", "hello world\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "zzz_not_found",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "No matches") {
		t.Error("expected 'No matches found' message")
	}
}

func TestGrepTool_AllowedDirBoundary(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "test",
		"path":    "/etc",
	})
	if !result.IsError {
		t.Error("expected error for path outside allowedDir")
	}
}

func TestGrepTool_PathNotFound(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "test",
		"path":    filepath.Join(dir, "nonexistent"),
	})
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestGrepTool_LineTruncation(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	longLine := strings.Repeat("x", 600) + "MATCH" + strings.Repeat("y", 100)
	writeTestFile(t, dir, "test.txt", longLine+"\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "MATCH",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "MATCH") && len(result.ForLLM) > types.GrepMaxLine+200 {
		t.Error("expected line to be truncated")
	}
}

func TestGrepTool_SkipsHiddenDirs(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	writeTestFile(t, dir, "visible.txt", "hello\n")

	hiddenDir := filepath.Join(dir, ".hidden")
	os.MkdirAll(hiddenDir, 0755)
	writeTestFile(t, dir, ".hidden/secret.txt", "hello\n")

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "hello",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "secret") {
		t.Error("hidden directory files should be skipped")
	}
}

func TestRelativizePath(t *testing.T) {
	tests := []struct {
		path, base, want string
	}{
		{"/home/user/project/foo.go", "/home/user/project", "foo.go"},
		{"/home/user/project/sub/bar.go", "/home/user/project", "sub/bar.go"},
	}
	for _, tt := range tests {
		got := util.RelativizePath(tt.path, tt.base)
		if got != tt.want {
			t.Errorf("RelativizePath(%q, %q) = %q, want %q", tt.path, tt.base, got, tt.want)
		}
	}
}

func TestParseRgStream_Matches(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})

	lines := []string{
		`{"type":"begin","data":{"path":{"text":"/tmp/test/foo.go"}}}`,
		`{"type":"match","data":{"path":{"text":"/tmp/test/foo.go"},"lines":{"text":"func Hello() string {\n"},"line_number":3}}`,
		`{"type":"match","data":{"path":{"text":"/tmp/test/bar.go"},"lines":{"text":"func World() string {\n"},"line_number":5}}`,
		`{"type":"end","data":{"path":{"text":"/tmp/test/bar.go"}}}`,
	}
	input := strings.NewReader(strings.Join(lines, "\n"))

	result, killed := tool.parseRgStream(input, "/tmp/test")
	if killed {
		t.Error("expected killed=false")
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "2 matches") {
		t.Errorf("expected 2 matches, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "foo.go:3:") {
		t.Error("expected foo.go:3 in output")
	}
	if !strings.Contains(result.ForLLM, "bar.go:5:") {
		t.Error("expected bar.go:5 in output")
	}
}

func TestParseRgStream_NoMatches(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	input := strings.NewReader(`{"type":"summary","data":{}}`)

	result, killed := tool.parseRgStream(input, "/tmp")
	if killed {
		t.Error("expected killed=false")
	}
	if !strings.Contains(result.ForLLM, "No matches") {
		t.Errorf("expected 'No matches', got: %s", result.ForLLM)
	}
}

func TestParseRgStream_MatchLimitKill(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})

	var lines []string
	for i := 0; i < types.GrepMaxMatches+5; i++ {
		line := fmt.Sprintf(`{"type":"match","data":{"path":{"text":"/tmp/test/f.go"},"lines":{"text":"line %d\n"},"line_number":%d}}`, i, i+1)
		lines = append(lines, line)
	}
	input := strings.NewReader(strings.Join(lines, "\n"))

	result, killed := tool.parseRgStream(input, "/tmp/test")
	if !killed {
		t.Error("expected killed=true when match limit is exceeded")
	}
	if !strings.Contains(result.ForLLM, "matches truncated") {
		t.Errorf("expected truncation notice, got: %s", result.ForLLM)
	}
}

func TestParseRgStream_ContextLines(t *testing.T) {
	tool := NewGrepTool("", &ops.RealFileOps{}, &ops.RealExecOps{})

	lines := []string{
		`{"type":"context","data":{"path":{"text":"/tmp/test/foo.go"},"lines":{"text":"// before\n"},"line_number":2}}`,
		`{"type":"match","data":{"path":{"text":"/tmp/test/foo.go"},"lines":{"text":"func Hello() {\n"},"line_number":3}}`,
		`{"type":"context","data":{"path":{"text":"/tmp/test/foo.go"},"lines":{"text":"// after\n"},"line_number":4}}`,
	}
	input := strings.NewReader(strings.Join(lines, "\n"))

	result, _ := tool.parseRgStream(input, "/tmp/test")
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "foo.go:2  ") {
		t.Error("expected context line with double-space separator")
	}
	if !strings.Contains(result.ForLLM, "foo.go:3: ") {
		t.Error("expected match line with colon separator")
	}
}

func TestIsBinaryExtension(t *testing.T) {
	binaries := []string{"test.exe", "lib.so", "image.png", "archive.zip", "code.pyc"}
	for _, name := range binaries {
		if !util.IsBinaryExtension(name) {
			t.Errorf("expected %q to be binary", name)
		}
	}

	textFiles := []string{"main.go", "readme.md", "config.json", "style.css"}
	for _, name := range textFiles {
		if util.IsBinaryExtension(name) {
			t.Errorf("expected %q to not be binary", name)
		}
	}
}

// writeTestFile creates a file in dir with the given name and content.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
