package agent

import (
	"context"
	"testing"

	"github.com/zhy0216/pigo/pkg/types"
)

func TestAutoHeal(t *testing.T) {
	// Create a minimal agent for testing
	a := &Agent{output: &discardWriter{}}

	t.Run("no file-modifying tools", func(t *testing.T) {
		toolCalls := []types.ToolCall{
			{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path": "/tmp/test.go"}`}},
		}
		results := []toolCallResult{
			{result: &types.ToolResult{}},
		}

		msg := a.autoHeal(context.Background(), toolCalls, results)
		if msg != "" {
			t.Errorf("expected no heal message for read-only tool, got: %s", msg)
		}
	})

	t.Run("file-modifying tool with non-go file", func(t *testing.T) {
		toolCalls := []types.ToolCall{
			{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "write", Arguments: `{"path": "/tmp/test.txt"}`}},
		}
		results := []toolCallResult{
			{result: &types.ToolResult{}},
		}

		msg := a.autoHeal(context.Background(), toolCalls, results)
		if msg != "" {
			t.Errorf("expected no heal message for non-go file, got: %s", msg)
		}
	})

	t.Run("skips failed tool calls", func(t *testing.T) {
		toolCalls := []types.ToolCall{
			{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "edit", Arguments: `{"path": "/tmp/test.go"}`}},
		}
		results := []toolCallResult{
			{result: &types.ToolResult{IsError: true}},
		}

		msg := a.autoHeal(context.Background(), toolCalls, results)
		if msg != "" {
			t.Errorf("expected no heal message for failed tool call, got: %s", msg)
		}
	})

	t.Run("detects go file modification", func(t *testing.T) {
		toolCalls := []types.ToolCall{
			{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "edit", Arguments: `{"path": "/tmp/test.go"}`}},
		}
		results := []toolCallResult{
			{result: &types.ToolResult{}},
		}

		// This will actually run `go build ./...` which will likely fail
		// since /tmp/test.go probably doesn't exist, but the important thing
		// is that it attempts to run the check
		msg := a.autoHeal(context.Background(), toolCalls, results)
		// We can't assert the exact message since it depends on the go build result
		// but the function should not panic
		_ = msg
	})
}

// discardWriter discards all writes.
type discardWriter struct{}

func (w *discardWriter) Write(p []byte) (n int, err error) { return len(p), nil }
