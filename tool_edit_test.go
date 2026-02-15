package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEditTool(t *testing.T) {
	tool := NewEditTool()
	tmpDir := t.TempDir()

	t.Run("replace unique string", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit1.txt")
		os.WriteFile(testFile, []byte("hello world"), 0644)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "world",
			"new_string": "gopher",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}

		content, _ := os.ReadFile(testFile)
		if string(content) != "hello gopher" {
			t.Errorf("expected 'hello gopher', got '%s'", string(content))
		}
	})

	t.Run("string not found", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit2.txt")
		os.WriteFile(testFile, []byte("hello world"), 0644)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "notfound",
			"new_string": "replaced",
		})
		if !result.IsError {
			t.Error("expected error for string not found")
		}
	})

	t.Run("multiple occurrences without all flag", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit3.txt")
		os.WriteFile(testFile, []byte("foo bar foo baz foo"), 0644)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "foo",
			"new_string": "qux",
		})
		if !result.IsError {
			t.Error("expected error for multiple occurrences")
		}
	})

	t.Run("replace all occurrences", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit4.txt")
		os.WriteFile(testFile, []byte("foo bar foo baz foo"), 0644)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "foo",
			"new_string": "qux",
			"all":        true,
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}

		content, _ := os.ReadFile(testFile)
		if string(content) != "qux bar qux baz qux" {
			t.Errorf("expected 'qux bar qux baz qux', got '%s'", string(content))
		}
	})

	t.Run("file not found", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       "/nonexistent/file.txt",
			"old_string": "foo",
			"new_string": "bar",
		})
		if !result.IsError {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("missing parameters", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit5.txt")
		os.WriteFile(testFile, []byte("test"), 0644)

		// Missing old_string
		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"new_string": "bar",
		})
		if !result.IsError {
			t.Error("expected error for missing old_string")
		}

		// Missing new_string
		result = tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "test",
		})
		if !result.IsError {
			t.Error("expected error for missing new_string")
		}
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "edit_perm.txt")
		os.WriteFile(testFile, []byte("secret data"), 0600)

		result := tool.Execute(context.Background(), map[string]interface{}{
			"path":       testFile,
			"old_string": "secret",
			"new_string": "public",
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}

		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected permissions 0600, got %04o", info.Mode().Perm())
		}

		content, _ := os.ReadFile(testFile)
		if string(content) != "public data" {
			t.Errorf("expected 'public data', got '%s'", string(content))
		}
	})
}
