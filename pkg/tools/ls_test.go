package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/ops"
)

func TestLsTool_Name(t *testing.T) {
	tool := NewLsTool("", &ops.RealFileOps{})
	if tool.Name() != "ls" {
		t.Errorf("expected 'ls', got '%s'", tool.Name())
	}
}

func TestLsTool_Parameters(t *testing.T) {
	tool := NewLsTool("", &ops.RealFileOps{})
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Error("expected type 'object'")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok || props["path"] == nil {
		t.Error("expected 'path' property")
	}
}

func TestLsTool_BasicList(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpDir,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "file1.go") {
		t.Errorf("expected file1.go in output, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "file2.txt") {
		t.Errorf("expected file2.txt in output, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "subdir") {
		t.Errorf("expected subdir in output, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "[dir]") {
		t.Errorf("expected [dir] indicator, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "[file]") {
		t.Errorf("expected [file] indicator, got: %s", result.ForLLM)
	}
}

func TestLsTool_HiddenFiles(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("secret"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "visible"), []byte("hello"), 0644)

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpDir,
	})
	if strings.Contains(result.ForLLM, ".hidden") {
		t.Errorf("hidden file should not appear without 'all' flag, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "visible") {
		t.Errorf("visible file should appear, got: %s", result.ForLLM)
	}

	result = tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpDir,
		"all":  true,
	})
	if !strings.Contains(result.ForLLM, ".hidden") {
		t.Errorf("hidden file should appear with 'all' flag, got: %s", result.ForLLM)
	}
}

func TestLsTool_EmptyDir(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpDir,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "empty") {
		t.Errorf("expected empty directory message, got: %s", result.ForLLM)
	}
}

func TestLsTool_NotADirectory(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())
	filePath := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(filePath, []byte("hello"), 0644)

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": filePath,
	})

	if !result.IsError {
		t.Error("expected error for non-directory path")
	}
	if !strings.Contains(result.ForLLM, "not a directory") {
		t.Errorf("expected 'not a directory' error, got: %s", result.ForLLM)
	}
}

func TestLsTool_NotFound(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": filepath.Join(tmpDir, "nonexistent"),
	})

	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
	if !strings.Contains(result.ForLLM, "not found") {
		t.Errorf("expected 'not found' error, got: %s", result.ForLLM)
	}
}

func TestLsTool_AllowedDirBoundary(t *testing.T) {
	tmpDir, _ := filepath.EvalSymlinks(t.TempDir())

	tool := NewLsTool(tmpDir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": "/etc",
	})

	if !result.IsError {
		t.Error("expected error for path outside allowed directory")
	}
}

func TestLsTool_MissingPath(t *testing.T) {
	tool := NewLsTool("", &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{})

	if !result.IsError {
		t.Error("expected error for missing path")
	}
}
