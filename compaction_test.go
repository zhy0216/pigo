package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractPathFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{"simple", `{"path": "/foo/bar.go"}`, "/foo/bar.go"},
		{"no path", `{"command": "ls"}`, ""},
		{"empty", `{}`, ""},
		{"nested", `{"path":"/tmp/test.txt","content":"hello"}`, "/tmp/test.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("extractPathFromArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestExtractFileOps(t *testing.T) {
	messages := []Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read", Arguments: `{"path": "/foo/main.go"}`}},
			},
		},
		{Role: "tool", Content: "file content"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "write", Arguments: `{"path": "/foo/out.txt", "content": "hello"}`}},
			},
		},
		{Role: "tool", Content: "wrote file"},
	}

	result := extractFileOps(messages)
	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in reads")
	}
	if !strings.Contains(result, "out.txt") {
		t.Error("expected out.txt in writes")
	}
}

func TestExtractFileOpsEmpty(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	result := extractFileOps(messages)
	if result != "" {
		t.Errorf("expected empty string for no file ops, got: %q", result)
	}
}

func TestFindCutPoint(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	// System prompt + enough messages to trigger compaction
	app.messages = []Message{
		{Role: "system", Content: "system prompt"},
	}
	// Add 20 pairs of user/assistant messages (each ~10K chars)
	for i := 0; i < 20; i++ {
		app.messages = append(app.messages,
			Message{Role: "user", Content: fmt.Sprintf("q%d: %s", i, strings.Repeat("x", 5000))},
			Message{Role: "assistant", Content: fmt.Sprintf("a%d: %s", i, strings.Repeat("y", 5000))},
		)
	}

	cut := app.findCutPoint()
	// Should cut somewhere to keep recent messages under budget
	if cut < 1 {
		t.Errorf("expected cut point > 0, got %d", cut)
	}
	if cut >= len(app.messages) {
		t.Errorf("cut point should be less than total messages, got %d of %d", cut, len(app.messages))
	}
}

func TestFindCutPointPreservesToolSequences(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	// Create enough messages to trigger compaction with a tool sequence near the cut
	app.messages = []Message{
		{Role: "system", Content: "system"},
	}
	// Add 15 pairs of large messages
	for i := 0; i < 15; i++ {
		app.messages = append(app.messages,
			Message{Role: "user", Content: strings.Repeat("x", 5000)},
			Message{Role: "assistant", Content: strings.Repeat("y", 5000)},
		)
	}
	// Add a tool call/result pair followed by more messages
	app.messages = append(app.messages,
		Message{Role: "assistant", Content: "", ToolCalls: []ToolCall{
			{ID: "c1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "bash", Arguments: `{"command":"ls"}`}},
		}},
		Message{Role: "tool", Content: "file1\nfile2\n", ToolCallID: "c1"},
		Message{Role: "user", Content: "next question"},
		Message{Role: "assistant", Content: "answer"},
	)

	cut := app.findCutPoint()
	// Cut should not land on the tool message
	if cut > 0 && cut < len(app.messages) && app.messages[cut].Role == "tool" {
		t.Errorf("cut point should not be on a tool result message, got index %d with role %q",
			cut, app.messages[cut].Role)
	}
}

func TestCompactMessagesWithSummarization(t *testing.T) {
	// Mock server that returns a summary
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-compact",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "The user discussed file operations and code changes.",
					},
					"finish_reason": "stop",
				},
			},
		}
		// summarizeMessages uses Chat() not ChatStream(), so always respond with JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	app := NewApp(cfg)
	output := &bytes.Buffer{}
	app.output = output

	// Fill with enough messages to trigger compaction (exceed maxContextChars=200K)
	largeContent := strings.Repeat("x", 20000)
	for i := 0; i < 15; i++ {
		app.messages = append(app.messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("msg-%d: %s", i, largeContent),
		})
		app.messages = append(app.messages, Message{
			Role:    "assistant",
			Content: fmt.Sprintf("response-%d: %s", i, largeContent),
		})
	}

	app.compactMessages(context.Background())

	// Should have compacted
	outputStr := output.String()
	if !strings.Contains(outputStr, "compacted") {
		t.Error("expected compaction notice in output")
	}

	// System prompt should be preserved
	if app.messages[0].Role != "system" {
		t.Error("expected system prompt to be preserved")
	}

	// Second message should be the summary
	if !strings.Contains(app.messages[1].Content, "summary") &&
		!strings.Contains(app.messages[1].Content, "compacted") {
		t.Errorf("expected summary as second message, got: %s", app.messages[1].Content[:100])
	}

	// Message count should be significantly reduced
	if len(app.messages) >= 30 {
		t.Errorf("expected fewer messages after compaction, got %d", len(app.messages))
	}
}

func TestCompactMessagesNoop(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	output := &bytes.Buffer{}
	app.output = output

	// Just system prompt + a few small messages — should not compact
	app.messages = append(app.messages, Message{Role: "user", Content: "hello"})
	app.messages = append(app.messages, Message{Role: "assistant", Content: "hi"})

	originalLen := len(app.messages)
	app.compactMessages(context.Background())

	if len(app.messages) != originalLen {
		t.Errorf("expected no compaction, message count changed from %d to %d", originalLen, len(app.messages))
	}
	if output.String() != "" {
		t.Errorf("expected no output, got: %s", output.String())
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string]bool{"a": true, "b": true, "c": true}
	keys := mapKeys(m)
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}
