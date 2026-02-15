package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTool(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
	tool := NewWriteTool(resolvedDir)

	t.Run("write new file", func(t *testing.T) {
		testFile := filepath.Join(resolvedDir, "new.txt")
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":    testFile,
			"content": "hello world",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}

		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "hello world" {
			t.Errorf("expected 'hello world', got '%s'", string(content))
		}
	})

	t.Run("overwrite existing file", func(t *testing.T) {
		testFile := filepath.Join(resolvedDir, "existing.txt")
		os.WriteFile(testFile, []byte("old content"), 0644)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":    testFile,
			"content": "new content",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}

		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "new content" {
			t.Errorf("expected 'new content', got '%s'", string(content))
		}
	})

	t.Run("create parent directories", func(t *testing.T) {
		testFile := filepath.Join(resolvedDir, "nested", "dir", "file.txt")
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":    testFile,
			"content": "nested content",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}

		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "nested content" {
			t.Errorf("expected 'nested content', got '%s'", string(content))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"content": "test",
		})
		if !result.IsError {
			t.Error("expected error for missing path")
		}
	})

	t.Run("missing content", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path": filepath.Join(resolvedDir, "test.txt"),
		})
		if !result.IsError {
			t.Error("expected error for missing content")
		}
	})

	t.Run("preserves existing file permissions on overwrite", func(t *testing.T) {
		testFile := filepath.Join(resolvedDir, "restricted.txt")
		os.WriteFile(testFile, []byte("old"), 0600)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":    testFile,
			"content": "new",
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}

		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("failed to stat: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected permissions 0600, got %04o", info.Mode().Perm())
		}
	})

	t.Run("path outside allowed directory", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":    "/tmp/outside-boundary.txt",
			"content": "should fail",
		})
		if !result.IsError {
			t.Error("expected error for path outside allowed directory")
		}
		if !contains(result.ForLLM, "outside the allowed directory") {
			t.Errorf("expected boundary error, got: %s", result.ForLLM)
		}
	})
}
