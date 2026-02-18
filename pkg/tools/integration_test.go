package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/ops"
)

func TestIntegration_ReadWriteEditWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
	testFile := filepath.Join(resolvedDir, "test.txt")

	registry := NewToolRegistry()
	registry.Register(NewReadTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(NewWriteTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(NewEditTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(NewBashTool(&ops.RealExecOps{}))

	ctx := context.Background()

	t.Run("write file", func(t *testing.T) {
		result := registry.Execute(ctx, "write", map[string]interface{}{
			"path":    testFile,
			"content": "Hello World\nLine 2\nLine 3",
		})
		if result.IsError {
			t.Fatalf("write failed: %s", result.ForLLM)
		}
	})

	t.Run("read file", func(t *testing.T) {
		result := registry.Execute(ctx, "read", map[string]interface{}{
			"path": testFile,
		})
		if result.IsError {
			t.Fatalf("read failed: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "Hello World") {
			t.Errorf("expected 'Hello World' in content, got: %s", result.ForLLM)
		}
	})

	t.Run("edit file", func(t *testing.T) {
		result := registry.Execute(ctx, "edit", map[string]interface{}{
			"path":       testFile,
			"old_string": "Hello World",
			"new_string": "Hello Go",
		})
		if result.IsError {
			t.Fatalf("edit failed: %s", result.ForLLM)
		}
	})

	t.Run("verify edit", func(t *testing.T) {
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "Hello Go") {
			t.Errorf("expected 'Hello Go', got: %s", string(content))
		}
	})
}

func TestIntegration_BashCommand(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(NewBashTool(&ops.RealExecOps{}))

	ctx := context.Background()

	t.Run("execute pwd", func(t *testing.T) {
		result := registry.Execute(ctx, "bash", map[string]interface{}{
			"command": "pwd",
		})
		if result.IsError {
			t.Fatalf("bash failed: %s", result.ForLLM)
		}
		if result.ForLLM == "" {
			t.Error("expected non-empty output")
		}
	})

	t.Run("execute echo with pipe", func(t *testing.T) {
		result := registry.Execute(ctx, "bash", map[string]interface{}{
			"command": "echo 'test' | cat",
		})
		if result.IsError {
			t.Fatalf("bash failed: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "test") {
			t.Errorf("expected 'test' in output, got: %s", result.ForLLM)
		}
	})
}
