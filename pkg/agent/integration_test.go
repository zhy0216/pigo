package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/ops"
	"github.com/user/pigo/pkg/tools"
)

func TestIntegration_ReadWriteEditWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
	testFile := filepath.Join(resolvedDir, "test.txt")

	registry := tools.NewToolRegistry()
	registry.Register(tools.NewReadTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewWriteTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewEditTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewBashTool(&ops.RealExecOps{}))

	ctx := context.Background()

	// Step 1: Write a file
	t.Run("write file", func(t *testing.T) {
		result := registry.Execute(ctx, "write", map[string]interface{}{
			"path":    testFile,
			"content": "Hello World\nLine 2\nLine 3",
		})
		if result.IsError {
			t.Fatalf("write failed: %s", result.ForLLM)
		}
	})

	// Step 2: Read the file
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

	// Step 3: Edit the file
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

	// Step 4: Verify edit
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
	registry := tools.NewToolRegistry()
	registry.Register(tools.NewBashTool(&ops.RealExecOps{}))

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

func TestIntegration_ToolNotFound(t *testing.T) {
	registry := tools.NewToolRegistry()
	ctx := context.Background()

	result := registry.Execute(ctx, "nonexistent", map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for nonexistent tool")
	}
	if !strings.Contains(result.ForLLM, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result.ForLLM)
	}
}

func TestIntegration_NestedDirectoryWrite(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
	nestedFile := filepath.Join(resolvedDir, "a", "b", "c", "file.txt")

	registry := tools.NewToolRegistry()
	registry.Register(tools.NewWriteTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewReadTool(resolvedDir, &ops.RealFileOps{}))

	ctx := context.Background()

	result := registry.Execute(ctx, "write", map[string]interface{}{
		"path":    nestedFile,
		"content": "nested content",
	})
	if result.IsError {
		t.Fatalf("write failed: %s", result.ForLLM)
	}

	result = registry.Execute(ctx, "read", map[string]interface{}{
		"path": nestedFile,
	})
	if result.IsError {
		t.Fatalf("read failed: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "nested content") {
		t.Errorf("expected 'nested content', got: %s", result.ForLLM)
	}
}

func TestIntegration_EditWithReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
	testFile := filepath.Join(resolvedDir, "replace.txt")

	registry := tools.NewToolRegistry()
	registry.Register(tools.NewWriteTool(resolvedDir, &ops.RealFileOps{}))
	registry.Register(tools.NewEditTool(resolvedDir, &ops.RealFileOps{}))

	ctx := context.Background()

	registry.Execute(ctx, "write", map[string]interface{}{
		"path":    testFile,
		"content": "foo bar foo baz foo",
	})

	result := registry.Execute(ctx, "edit", map[string]interface{}{
		"path":       testFile,
		"old_string": "foo",
		"new_string": "qux",
		"all":        true,
	})
	if result.IsError {
		t.Fatalf("edit failed: %s", result.ForLLM)
	}

	content, _ := os.ReadFile(testFile)
	if string(content) != "qux bar qux baz qux" {
		t.Errorf("expected all replacements, got: %s", string(content))
	}
}
