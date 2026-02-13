package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTool(t *testing.T) {
	tool := NewReadTool()

	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line 1\nline 2\nline 3\nline 4\nline 5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("read entire file", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path": testFile,
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		if result.ForLLM == "" {
			t.Error("expected content, got empty string")
		}
	})

	t.Run("read with offset", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":   testFile,
			"offset": float64(3),
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		// Should start from line 3
		if !contains(result.ForLLM, "line 3") {
			t.Errorf("expected to contain 'line 3', got: %s", result.ForLLM)
		}
	})

	t.Run("read with limit", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":  testFile,
			"limit": float64(2),
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		// Should only have 2 lines
		if contains(result.ForLLM, "line 3") {
			t.Errorf("should not contain 'line 3', got: %s", result.ForLLM)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path": "/nonexistent/file.txt",
		})
		if !result.IsError {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{})
		if !result.IsError {
			t.Error("expected error for missing path")
		}
	})

	t.Run("directory error", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path": tmpDir,
		})
		if !result.IsError {
			t.Error("expected error for directory")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
