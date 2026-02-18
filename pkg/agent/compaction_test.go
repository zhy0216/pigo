package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
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
		{"invalid json", `not json`, ""},
		{"escaped quotes in path", `{"path":"/tmp/file\"name\".txt"}`, `/tmp/file"name".txt`},
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
	messages := []types.Message{
		{
			Role: "assistant",
			ToolCalls: []types.ToolCall{
				{Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read", Arguments: `{"path": "/foo/main.go"}`}},
			},
		},
		{Role: "tool", Content: "file content"},
		{
			Role: "assistant",
			ToolCalls: []types.ToolCall{
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
	messages := []types.Message{
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
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	// System prompt + enough messages to trigger compaction
	agent.messages = []types.Message{
		{Role: "system", Content: "system prompt"},
	}
	for i := 0; i < 20; i++ {
		agent.messages = append(agent.messages,
			types.Message{Role: "user", Content: fmt.Sprintf("q%d: %s", i, strings.Repeat("x", 5000))},
			types.Message{Role: "assistant", Content: fmt.Sprintf("a%d: %s", i, strings.Repeat("y", 5000))},
		)
	}

	cut := agent.findCutPoint()
	if cut < 1 {
		t.Errorf("expected cut point > 0, got %d", cut)
	}
	if cut >= len(agent.messages) {
		t.Errorf("cut point should be less than total messages, got %d of %d", cut, len(agent.messages))
	}
}

func TestFindCutPointAllToolMessages(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	agent.messages = []types.Message{
		{Role: "system", Content: "system prompt"},
	}
	for i := 0; i < 20; i++ {
		agent.messages = append(agent.messages,
			types.Message{Role: "user", Content: strings.Repeat("x", 5000)},
			types.Message{Role: "assistant", Content: strings.Repeat("y", 5000)},
		)
	}
	for i := 0; i < 5; i++ {
		agent.messages = append(agent.messages,
			types.Message{Role: "tool", Content: "result", ToolCallID: fmt.Sprintf("call_%d", i)},
		)
	}

	cut := agent.findCutPoint()
	if cut < 0 || cut > len(agent.messages) {
		t.Errorf("cut point out of range: %d (messages: %d)", cut, len(agent.messages))
	}
	if cut > 0 && cut < len(agent.messages) && agent.messages[cut].Role == "tool" {
		t.Errorf("cut point should not land on a tool message, got index %d", cut)
	}
}

func TestFindCutPointPreservesToolSequences(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	agent.messages = []types.Message{
		{Role: "system", Content: "system"},
	}
	for i := 0; i < 15; i++ {
		agent.messages = append(agent.messages,
			types.Message{Role: "user", Content: strings.Repeat("x", 5000)},
			types.Message{Role: "assistant", Content: strings.Repeat("y", 5000)},
		)
	}
	agent.messages = append(agent.messages,
		types.Message{Role: "assistant", Content: "", ToolCalls: []types.ToolCall{
			{ID: "c1", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "bash", Arguments: `{"command":"ls"}`}},
		}},
		types.Message{Role: "tool", Content: "file1\nfile2\n", ToolCallID: "c1"},
		types.Message{Role: "user", Content: "next question"},
		types.Message{Role: "assistant", Content: "answer"},
	)

	cut := agent.findCutPoint()
	if cut > 0 && cut < len(agent.messages) && agent.messages[cut].Role == "tool" {
		t.Errorf("cut point should not be on a tool result message, got index %d with role %q",
			cut, agent.messages[cut].Role)
	}
}

func TestCompactMessagesWithSummarization(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	agent := NewAgent(cfg)
	output := &bytes.Buffer{}
	agent.output = output

	largeContent := strings.Repeat("x", 20000)
	for i := 0; i < 15; i++ {
		agent.messages = append(agent.messages, types.Message{
			Role:    "user",
			Content: fmt.Sprintf("msg-%d: %s", i, largeContent),
		})
		agent.messages = append(agent.messages, types.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("response-%d: %s", i, largeContent),
		})
	}

	agent.compactMessages(context.Background())

	outputStr := output.String()
	if !strings.Contains(outputStr, "compacted") {
		t.Error("expected compaction notice in output")
	}

	if agent.messages[0].Role != "system" {
		t.Error("expected system prompt to be preserved")
	}

	if !strings.Contains(agent.messages[1].Content, "summary") &&
		!strings.Contains(agent.messages[1].Content, "compacted") {
		t.Errorf("expected summary as second message, got: %s", agent.messages[1].Content[:100])
	}

	if len(agent.messages) >= 30 {
		t.Errorf("expected fewer messages after compaction, got %d", len(agent.messages))
	}
}

func TestCompactMessagesNoop(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	output := &bytes.Buffer{}
	agent.output = output

	agent.messages = append(agent.messages, types.Message{Role: "user", Content: "hello"})
	agent.messages = append(agent.messages, types.Message{Role: "assistant", Content: "hi"})

	originalLen := len(agent.messages)
	agent.compactMessages(context.Background())

	if len(agent.messages) != originalLen {
		t.Errorf("expected no compaction, message count changed from %d to %d", originalLen, len(agent.messages))
	}
	if output.String() != "" {
		t.Errorf("expected no output, got: %s", output.String())
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string]bool{"a": true, "b": true, "c": true}
	keys := util.MapKeys(m)
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}
