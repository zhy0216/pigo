package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/ops"
)

func TestToolDescriptions(t *testing.T) {
	fops := &ops.RealFileOps{}
	eops := &ops.RealExecOps{}

	tools := []interface{ Description() string }{
		NewFindTool("", fops, eops),
		NewGrepTool("", fops, eops),
		NewLsTool("", fops),
	}
	for _, tool := range tools {
		desc := tool.Description()
		if desc == "" {
			t.Errorf("expected non-empty description for %T", tool)
		}
	}
}

// noRgExecOps is a mock ExecOps that always fails LookPath for "rg",
// forcing grep to use its native Go implementation.
type noRgExecOps struct {
	ops.RealExecOps
}

func (n *noRgExecOps) LookPath(file string) (string, error) {
	if file == "rg" {
		return "", fmt.Errorf("not found")
	}
	return n.RealExecOps.LookPath(file)
}

func TestGrepTool_NativeFallback(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())

	os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc Hello() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "world.go"), []byte("package main\n\nfunc World() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("no functions here\n"), 0644)

	tool := NewGrepTool(dir, &ops.RealFileOps{}, &noRgExecOps{})

	t.Run("basic search", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern": "func",
			"path":    dir,
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if result.ForLLM == "No matches found." {
			t.Error("expected matches for 'func'")
		}
	})

	t.Run("include filter", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern": "func",
			"path":    dir,
			"include": "*.go",
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if result.ForLLM == "No matches found." {
			t.Error("expected matches in .go files")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern": "zzz_nonexistent",
			"path":    dir,
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if result.ForLLM != "No matches found." {
			t.Errorf("expected 'No matches found.', got: %s", result.ForLLM)
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern": "[invalid",
			"path":    dir,
		})
		if !result.IsError {
			t.Error("expected error for invalid regex")
		}
	})

	t.Run("skips binary files", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "image.png"), []byte("func fake"), 0644)
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern": "fake",
			"path":    dir,
		})
		// Should not find "fake" because .png is a binary extension
		if result.ForLLM != "No matches found." {
			t.Errorf("expected no matches in binary files, got: %s", result.ForLLM)
		}
	})

	t.Run("context lines", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "context.txt"), []byte("one\nmatch\nthree\n"), 0644)
		result := tool.Execute(context.Background(), map[string]interface{}{
			"pattern":       "match",
			"path":          dir,
			"context_lines": float64(1),
		})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "context.txt:1  ") {
			t.Errorf("expected context line with double-space separator, got: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "context.txt:2: ") {
			t.Errorf("expected match line with colon separator, got: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "context.txt:3  ") {
			t.Errorf("expected trailing context line, got: %s", result.ForLLM)
		}
	})
}

func TestLsTool_Symlink(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	realFile := filepath.Join(dir, "real.txt")
	os.WriteFile(realFile, []byte("content"), 0644)
	os.Symlink(realFile, filepath.Join(dir, "link.txt"))

	tool := NewLsTool(dir, &ops.RealFileOps{})
	result := tool.Execute(context.Background(), map[string]interface{}{
		"path": dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM == "" {
		t.Error("expected non-empty output")
	}
}
