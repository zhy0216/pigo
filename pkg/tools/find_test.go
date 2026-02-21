package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhy0216/pigo/pkg/ops"
)

func TestFindTool_Name(t *testing.T) {
	tool := NewFindTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	if tool.Name() != "find" {
		t.Errorf("expected name 'find', got %q", tool.Name())
	}
}

func TestFindTool_Parameters(t *testing.T) {
	tool := NewFindTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
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
	tool := NewFindTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing pattern")
	}
}

func TestFindTool_BasicSearch(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "main.go", "package main")
	createTestFile(t, dir, "utils.go", "package main")
	createTestFile(t, dir, "readme.md", "# readme")

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "file.txt", "content")
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "file.txt", "content")
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "test.txt", "content")

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    "/etc",
	})
	if !result.IsError {
		t.Error("expected error for path outside allowedDir")
	}
}

func TestFindTool_PathNotFound(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    filepath.Join(dir, "nonexistent"),
	})
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestFindTool_PathIsFile(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "test.txt", "content")

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    filepath.Join(dir, "test.txt"),
	})
	if !result.IsError {
		t.Error("expected error when path is a file, not a directory")
	}
}

func TestFindTool_SkipsHiddenDirs(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "visible.go", "package main")

	hiddenDir := filepath.Join(dir, ".git")
	os.MkdirAll(hiddenDir, 0755)
	createTestFile(t, dir, ".git/config.go", "package git")

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	createTestFile(t, dir, "top.go", "package main")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	createTestFile(t, dir, "sub/nested.go", "package sub")

	tool := NewFindTool(dir, &ops.RealFileOps{}, &ops.RealExecOps{})
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
	tool := NewFindTool("", &ops.RealFileOps{}, &ops.RealExecOps{})

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
	tool := NewFindTool("", &ops.RealFileOps{}, &ops.RealExecOps{})
	result := tool.formatResults("", "/tmp")

	if !strings.Contains(result.ForLLM, "No files found") {
		t.Errorf("expected 'No files found', got: %s", result.ForLLM)
	}
}

// mockExecOps is a mock for ExecOps used to test fd-related paths.
type mockExecOps struct {
	lookPathFn func(file string) (string, error)
	runFn      func(ctx context.Context, name string, args []string, env []string) (string, string, int, error)
}

func (m *mockExecOps) LookPath(file string) (string, error) {
	return m.lookPathFn(file)
}

func (m *mockExecOps) Run(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
	return m.runFn(ctx, name, args, env)
}

func TestFindTool_FindFdBinary_Fdfind(t *testing.T) {
	// fd not found, fdfind found
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			if file == "fdfind" {
				return "/usr/bin/fdfind", nil
			}
			return "", fmt.Errorf("not found: %s", file)
		},
	}
	tool := NewFindTool("", &ops.RealFileOps{}, mock)
	path, err := tool.findFdBinary()
	if err != nil {
		t.Fatalf("expected fdfind to be found, got error: %v", err)
	}
	if path != "/usr/bin/fdfind" {
		t.Errorf("expected /usr/bin/fdfind, got %q", path)
	}
}

func TestFindTool_FindFdBinary_NeitherFound(t *testing.T) {
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "", fmt.Errorf("not found: %s", file)
		},
	}
	tool := NewFindTool("", &ops.RealFileOps{}, mock)
	_, err := tool.findFdBinary()
	if err == nil {
		t.Error("expected error when neither fd nor fdfind is found")
	}
}

func TestFindTool_ExecuteWithFd_Success(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			if file == "fd" {
				return "/usr/bin/fd", nil
			}
			return "", fmt.Errorf("not found")
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return dir + "/main.go\n" + dir + "/util.go\n", "", 0, nil
		},
	}
	// Create actual files so Stat works
	createTestFile(t, dir, "main.go", "package main")
	createTestFile(t, dir, "util.go", "package main")

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
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
	if !strings.Contains(result.ForLLM, "util.go") {
		t.Error("expected util.go in results")
	}
}

func TestFindTool_ExecuteWithFd_NoResults(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return "", "", 1, nil // exit code 1 = no results
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
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

func TestFindTool_ExecuteWithFd_ErrorWithStderr(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return "", "permission denied", 2, nil
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if !result.IsError {
		t.Error("expected error for exit code > 1")
	}
	if !strings.Contains(result.ForLLM, "permission denied") {
		t.Errorf("expected stderr in error, got: %s", result.ForLLM)
	}
}

func TestFindTool_ExecuteWithFd_ErrorWithoutStderr(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return "", "", 2, nil
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if !result.IsError {
		t.Error("expected error for exit code > 1")
	}
	if !strings.Contains(result.ForLLM, "exit code 2") {
		t.Errorf("expected exit code in error, got: %s", result.ForLLM)
	}
}

func TestFindTool_ExecuteWithFd_EmptyStdout(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return "", "", 0, nil // exit 0 but empty output
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "No files found") {
		t.Errorf("expected 'No files found', got: %s", result.ForLLM)
	}
}

func TestFindTool_ExecuteWithFd_RunError(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			return "", "", -1, fmt.Errorf("context cancelled")
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	if !result.IsError {
		t.Error("expected error for run failure")
	}
	if !strings.Contains(result.ForLLM, "context cancelled") {
		t.Errorf("expected run error in result, got: %s", result.ForLLM)
	}
}

func TestFindTool_ExecuteWithFd_TypeFilters(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())

	var capturedArgs []string
	mock := &mockExecOps{
		lookPathFn: func(file string) (string, error) {
			return "/usr/bin/fd", nil
		},
		runFn: func(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
			capturedArgs = args
			return "", "", 1, nil
		},
	}

	tool := NewFindTool(dir, &ops.RealFileOps{}, mock)

	// Test file filter
	tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
		"type":    "file",
	})
	found := false
	for i, arg := range capturedArgs {
		if arg == "--type" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "f" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --type f in args for file filter, got: %v", capturedArgs)
	}

	// Test directory filter
	tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*",
		"path":    dir,
		"type":    "directory",
	})
	found = false
	for i, arg := range capturedArgs {
		if arg == "--type" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "d" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --type d in args for directory filter, got: %v", capturedArgs)
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
