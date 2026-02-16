package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBashTool(t *testing.T) {
	tool := NewBashTool(&RealExecOps{})

	t.Run("simple command", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"command": "echo hello",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "hello") {
			t.Errorf("expected output to contain 'hello', got: %s", result.ForLLM)
		}
	})

	t.Run("command with exit code", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"command": "exit 1",
		})
		if !result.IsError {
			t.Error("expected error for non-zero exit code")
		}
	})

	t.Run("command with stderr", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"command": "echo error >&2",
		})
		if !strings.Contains(result.ForLLM, "STDERR") {
			t.Errorf("expected STDERR in output, got: %s", result.ForLLM)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		result := tool.Execute(ctx, map[string]interface{}{
			"command": "sleep 10",
			"timeout": float64(1),
		})
		if !result.IsError {
			t.Error("expected timeout error")
		}
	})

	t.Run("missing command", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{})
		if !result.IsError {
			t.Error("expected error for missing command")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]interface{}{
			"command": "true",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "(no output)") {
			t.Errorf("expected '(no output)', got: %s", result.ForLLM)
		}
	})

	t.Run("env sanitization hides sensitive vars", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "super-secret-key")

		result := tool.Execute(context.Background(), map[string]interface{}{
			"command": "env",
		})
		if result.IsError {
			t.Errorf("unexpected error: %s", result.ForLLM)
		}
		if strings.Contains(result.ForLLM, "super-secret-key") {
			t.Error("OPENAI_API_KEY should not appear in env output")
		}
		if strings.Contains(result.ForLLM, "OPENAI_API_KEY") {
			t.Error("OPENAI_API_KEY variable name should not appear in env output")
		}
	})
}
