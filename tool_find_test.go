package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindTool_Name(t *testing.T) {
	tool := NewFindTool("")
	if tool.Name() != "find" {
		t.Errorf("expected name 'find', got %q", tool.Name())
	}
}

func TestFindTool_Parameters(t *testing.T) {
	tool := NewFindTool("")
	params := tool.Parameters()
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "pattern" {
		t.Errorf("expected required=[pattern], got %v", required)
	}
}

func TestFindTool_MissingPattern(t *testing.T) {
	tool := NewFindTool("")
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing pattern")
	}
}

func TestFindTool_BasicSearch(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "main.go", "package main")
	createTestFile(t, dir, "utils.go", "package main")
	createTestFile(t, dir, "readme.md", "# readme")

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "main.go") {
		t.Error("expected main.go in results")
	}
	if !strings.Contains(result.ForLLM, "utils.go") {
		t.Error("expected utils.go in results")
	}
	if strings.Contains(result.ForLLM, "readme.md") {
		t.Error("readme.md should not appear with *.go pattern")
	}
}

func TestFindTool_TypeFilterFile(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "file.txt", "content")
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    dir,
		"type":    "file",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "file.txt") {
		t.Error("expected file.txt in results")
	}
	if strings.Contains(result.ForLLM, "subdir") {
		t.Error("directories should be excluded with type=file")
	}
}

func TestFindTool_TypeFilterDirectory(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "file.txt", "content")
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    dir,
		"type":    "directory",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "subdir") {
		t.Error("expected subdir in results")
	}
	if strings.Contains(result.ForLLM, "file.txt") {
		t.Error("files should be excluded with type=directory")
	}
}

func TestFindTool_InvalidType(t *testing.T) {
	dir := t.TempDir()
	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    dir,
		"type":    "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid type")
	}
}

func TestFindTool_NoMatches(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "test.txt", "content")

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.xyz",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "No files found") {
		t.Errorf("expected 'No files found', got: %s", result.ForLLM)
	}
}

func TestFindTool_AllowedDirBoundary(t *testing.T) {
	dir := t.TempDir()
	tool := NewFindTool(dir)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    "/etc",
	})
	if !result.IsError {
		t.Error("expected error for path outside allowedDir")
	}
}

func TestFindTool_PathNotFound(t *testing.T) {
	dir := t.TempDir()
	tool := NewFindTool(dir)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    filepath.Join(dir, "nonexistent"),
	})
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestFindTool_PathIsFile(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "test.txt", "content")

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    filepath.Join(dir, "test.txt"),
	})
	if !result.IsError {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestFindTool_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "visible.go", "package main")

	hiddenDir := filepath.Join(dir, ".git")
	os.MkdirAll(hiddenDir, 0755)
	createTestFile(t, dir, ".git/config.go", "package git")

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "visible.go") {
		t.Error("expected visible.go in results")
	}
	if strings.Contains(result.ForLLM, "config.go") {
		t.Error("hidden directory files should be skipped")
	}
}

func TestFindTool_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	createTestFile(t, dir, "top.go", "package main")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	createTestFile(t, dir, "sub/nested.go", "package sub")

	tool := NewFindTool(dir)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "top.go") {
		t.Error("expected top.go in results")
	}
	if !strings.Contains(result.ForLLM, "nested.go") {
		t.Error("expected nested.go in results")
	}
}

func TestFindTool_FormatResults(t *testing.T) {
	tool := NewFindTool("")

	output := "/tmp/test/foo.go\n/tmp/test/bar.go\n/tmp/test/sub/baz.go\n"
	result := tool.formatResults(output, "/tmp/test")

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "3 results") {
		t.Errorf("expected 3 results, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "foo.go") {
		t.Error("expected foo.go in output")
	}
	if !strings.Contains(result.ForLLM, "sub/baz.go") {
		t.Error("expected sub/baz.go in output")
	}
}

func TestFindTool_FormatResultsEmpty(t *testing.T) {
	tool := NewFindTool("")
	result := tool.formatResults("", "/tmp")

	if !strings.Contains(result.ForLLM, "No files found") {
		t.Errorf("expected 'No files found', got: %s", result.ForLLM)
	}
}

// createTestFile creates a file inside dir at the given relative path.
func createTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
