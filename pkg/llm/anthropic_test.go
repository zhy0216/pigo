package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhy0216/pigo/pkg/types"
)

func TestNewAnthropicProvider(t *testing.T) {
	t.Run("with defaults", func(t *testing.T) {
		p := NewAnthropicProvider("sk-ant-test", "", "claude-sonnet-4-20250514")
		if p.GetModel() != "claude-sonnet-4-20250514" {
			t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", p.GetModel())
		}
	})

	t.Run("with base URL", func(t *testing.T) {
		p := NewAnthropicProvider("sk-ant-test", "https://custom.api.com", "claude-3-haiku")
		if p.GetModel() != "claude-3-haiku" {
			t.Errorf("expected model 'claude-3-haiku', got %q", p.GetModel())
		}
	})
}

func TestAnthropicSetModel(t *testing.T) {
	p := NewAnthropicProvider("sk-ant-test", "", "claude-3-haiku")
	if p.GetModel() != "claude-3-haiku" {
		t.Errorf("expected claude-3-haiku, got %s", p.GetModel())
	}
	p.SetModel("claude-sonnet-4-20250514")
	if p.GetModel() != "claude-sonnet-4-20250514" {
		t.Errorf("expected claude-sonnet-4-20250514, got %s", p.GetModel())
	}
}

func TestAnthropicChat_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		response := map[string]interface{}{
			"id":    "msg_123",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := p.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Claude!" {
		t.Errorf("expected 'Hello from Claude!', got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected finish_reason 'end_turn', got %q", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestAnthropicChat_WithToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":    "msg_456",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{
					"type": "tool_use",
					"id":   "toolu_123",
					"name": "read",
					"input": map[string]interface{}{
						"path": "/tmp/test.txt",
					},
				},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]interface{}{"input_tokens": 20, "output_tokens": 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Read /tmp/test.txt"},
	}
	toolDefs := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read",
				"description": "Read a file",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"path"},
				},
			},
		},
	}

	resp, err := p.Chat(context.Background(), messages, toolDefs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_123" {
		t.Errorf("expected tool call ID 'toolu_123', got %q", tc.ID)
	}
	if tc.Function.Name != "read" {
		t.Errorf("expected function name 'read', got %q", tc.Function.Name)
	}
	if tc.Type != "function" {
		t.Errorf("expected type 'function', got %q", tc.Type)
	}
}

func TestAnthropicChat_AllMessageTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":    "msg_789",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Done."},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 50, "output_tokens": 3},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Read file"},
		{Role: "assistant", Content: "I'll read it", ToolCalls: []types.ToolCall{
			{ID: "call_1", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path":"/tmp/x"}`}},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "call_1"},
		{Role: "user", Content: "Summarize"},
	}

	resp, err := p.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Done." {
		t.Errorf("expected 'Done.', got %q", resp.Content)
	}
}

func TestAnthropicChat_InvalidToolCallArgs(t *testing.T) {
	// Test fallback when tool call arguments are not valid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":    "msg_inv",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": "OK"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	// Assistant message with invalid JSON in tool call arguments
	messages := []types.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "call_bad", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: "not-json"}},
		}},
		{Role: "tool", Content: "result", ToolCallID: "call_bad"},
		{Role: "user", Content: "continue"},
	}

	resp, err := p.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected 'OK', got %q", resp.Content)
	}
}

func TestAnthropicChat_MalformedToolDef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":    "msg_mt",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]interface{}{
				{"type": "text", "text": "OK"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	toolDefs := []map[string]interface{}{
		{"type": "function", "not_function": "oops"},
	}
	resp, err := p.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, toolDefs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected 'OK', got %q", resp.Content)
	}
}

func TestAnthropicChat_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "invalid x-api-key",
			},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("bad-key", server.URL, "claude-sonnet-4-20250514")
	_, err := p.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Error("expected error for invalid API key")
	}
	if !strings.Contains(err.Error(), "anthropic chat failed") {
		t.Errorf("expected 'anthropic chat failed' in error, got: %v", err)
	}
}

func TestAnthropicChatStream_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected flusher")
			return
		}

		events := []string{
			fmt.Sprintf("event: message_start\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id":    "msg_s1",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-sonnet-4-20250514",
					"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 0},
				},
			})),
			fmt.Sprintf("event: content_block_start\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":          "content_block_start",
				"index":         0,
				"content_block": map[string]interface{}{"type": "text", "text": ""},
			})),
			fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{"type": "text_delta", "text": "Hello "},
			})),
			fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{"type": "text_delta", "text": "world!"},
			})),
			fmt.Sprintf("event: content_block_stop\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_stop",
				"index": 0,
			})),
			fmt.Sprintf("event: message_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "message_delta",
				"delta": map[string]interface{}{"stop_reason": "end_turn"},
				"usage": map[string]interface{}{"output_tokens": 5},
			})),
			fmt.Sprintf("event: message_stop\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type": "message_stop",
			})),
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "user", Content: "Hello"},
	}

	var buf strings.Builder
	resp, err := p.ChatStream(context.Background(), messages, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "Hello world!" {
		t.Errorf("expected streamed 'Hello world!', got %q", buf.String())
	}
	if resp.Content != "Hello world!" {
		t.Errorf("expected content 'Hello world!', got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected finish_reason 'end_turn', got %q", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
}

func TestAnthropicChatStream_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []string{
			fmt.Sprintf("event: message_start\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id":    "msg_tc",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-sonnet-4-20250514",
					"usage": map[string]interface{}{"input_tokens": 15, "output_tokens": 0},
				},
			})),
			fmt.Sprintf("event: content_block_start\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_stream_1",
					"name": "read",
				},
			})),
			fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": `{"path":`,
				},
			})),
			fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": `"/tmp/test"}`,
				},
			})),
			fmt.Sprintf("event: content_block_stop\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "content_block_stop",
				"index": 0,
			})),
			fmt.Sprintf("event: message_delta\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type":  "message_delta",
				"delta": map[string]interface{}{"stop_reason": "tool_use"},
				"usage": map[string]interface{}{"output_tokens": 8},
			})),
			fmt.Sprintf("event: message_stop\ndata: %s\n\n", mustJSON(map[string]interface{}{
				"type": "message_stop",
			})),
		}

		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "user", Content: "Read /tmp/test"},
	}

	var buf strings.Builder
	resp, err := p.ChatStream(context.Background(), messages, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_stream_1" {
		t.Errorf("expected tool call ID 'toolu_stream_1', got %q", tc.ID)
	}
	if tc.Function.Name != "read" {
		t.Errorf("expected function name 'read', got %q", tc.Function.Name)
	}
	if !strings.Contains(tc.Function.Arguments, "/tmp/test") {
		t.Errorf("expected arguments to contain '/tmp/test', got %q", tc.Function.Arguments)
	}
}

func TestAnthropicChatStream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "api_error",
				"message": "server error",
			},
		})
	}))
	defer server.Close()

	p := NewAnthropicProvider("sk-ant-test", server.URL, "claude-sonnet-4-20250514")
	var buf strings.Builder
	_, err := p.ChatStream(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil, &buf)
	if err == nil {
		t.Error("expected error for server failure")
	}
}

func TestAnthropicChat_CacheControlPlacement(t *testing.T) {
	// Test that buildRequest places cache control on system blocks and messages
	p := NewAnthropicProvider("sk-ant-test", "", "claude-sonnet-4-20250514")

	// With system messages and 4+ messages - tests cache control on 2nd-to-last and 4th-to-last
	messages := []types.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "resp1"},
		{Role: "user", Content: "msg2"},
		{Role: "assistant", Content: "resp2"},
		{Role: "user", Content: "msg3"},
	}

	params, err := p.buildRequest(messages, nil)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	// System blocks should have cache control on the last one
	if len(params.System) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(params.System))
	}
	if params.System[0].CacheControl.Type == "" {
		t.Error("expected cache control on last system block")
	}

	// Should have 5 messages (system is extracted out)
	if len(params.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(params.Messages))
	}
}

func TestAnthropicChat_NoSystem(t *testing.T) {
	p := NewAnthropicProvider("sk-ant-test", "", "claude-sonnet-4-20250514")
	messages := []types.Message{
		{Role: "user", Content: "Hello"},
	}
	params, err := p.buildRequest(messages, nil)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}
	if len(params.System) != 0 {
		t.Errorf("expected 0 system blocks, got %d", len(params.System))
	}
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
