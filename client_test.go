package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewClient(t *testing.T) {
	t.Run("create client with defaults", func(t *testing.T) {
		client := NewClient("test-key", "", "gpt-4", "chat")
		if client.GetModel() != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got '%s'", client.GetModel())
		}
	})

	t.Run("create client with custom base URL", func(t *testing.T) {
		client := NewClient("test-key", "https://custom.api.com", "gpt-4o", "chat")
		if client.GetModel() != "gpt-4o" {
			t.Errorf("expected model 'gpt-4o', got '%s'", client.GetModel())
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		result := GetEnvOrDefault("NONEXISTENT_VAR_12345", "default_value")
		if result != "default_value" {
			t.Errorf("expected 'default_value', got '%s'", result)
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_PIGO_VAR", "env_value")
		result := GetEnvOrDefault("TEST_PIGO_VAR", "default")
		if result != "env_value" {
			t.Errorf("expected 'env_value', got '%s'", result)
		}
	})
}

func TestClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello! How can I help you?",
					},
					"finish_reason": "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("successful chat completion", func(t *testing.T) {
		client := NewClient("test-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		resp, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Content != "Hello! How can I help you?" {
			t.Errorf("expected greeting response, got '%s'", resp.Content)
		}
		if resp.FinishReason != "stop" {
			t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
		}
	})

	t.Run("chat with tool definitions", func(t *testing.T) {
		client := NewClient("test-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Read a file"},
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
							"path": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		}

		resp, err := client.Chat(context.Background(), messages, toolDefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
	})
}

func TestClientChatWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-456",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "read",
									"arguments": "{\"path\": \"/tmp/test.txt\"}",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("response with tool calls", func(t *testing.T) {
		client := NewClient("test-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "user", Content: "Read /tmp/test.txt"},
		}

		resp, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
		}

		tc := resp.ToolCalls[0]
		if tc.ID != "call_123" {
			t.Errorf("expected tool call ID 'call_123', got '%s'", tc.ID)
		}
		if tc.Function.Name != "read" {
			t.Errorf("expected function name 'read', got '%s'", tc.Function.Name)
		}
	})
}

func TestClientChatWithAssistantToolCallMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-789",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "I read the file successfully.",
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("conversation with tool result", func(t *testing.T) {
		client := NewClient("test-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "user", Content: "Read a file"},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: struct {
							Name      string "json:\"name\""
							Arguments string "json:\"arguments\""
						}{
							Name:      "read",
							Arguments: "{\"path\": \"/tmp/test.txt\"}",
						},
					},
				},
			},
			{Role: "tool", Content: "file contents here", ToolCallID: "call_123"},
		}

		resp, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content == "" {
			t.Error("expected non-empty content")
		}
	})
}

func TestClientChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("{\"error\": {\"message\": \"Invalid API key\"}}"))
	}))
	defer server.Close()

	t.Run("API error handling", func(t *testing.T) {
		client := NewClient("invalid-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		_, err := client.Chat(context.Background(), messages, nil)
		if err == nil {
			t.Error("expected error for invalid API key")
		}
	})
}

func TestClientChatEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-empty",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("empty choices error", func(t *testing.T) {
		client := NewClient("test-key", server.URL, "gpt-4", "chat")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		_, err := client.Chat(context.Background(), messages, nil)
		if err == nil {
			t.Error("expected error for empty choices")
		}
	})
}

func TestResponsesAPITextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":     "resp-text-001",
			"object": "response",
			"output": []map[string]interface{}{
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]interface{}{
						{
							"type": "output_text",
							"text": "Hello from Responses API!",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("text response via responses API", func(t *testing.T) {
		client := NewClient("test-key", server.URL+"/v1", "gpt-4", "responses")
		messages := []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		}

		resp, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "Hello from Responses API!" {
			t.Errorf("expected 'Hello from Responses API!', got '%s'", resp.Content)
		}
		if resp.FinishReason != "stop" {
			t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
		}
		if len(resp.ToolCalls) != 0 {
			t.Errorf("expected 0 tool calls, got %d", len(resp.ToolCalls))
		}
	})
}

func TestResponsesAPIFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":     "resp-fc-001",
			"object": "response",
			"output": []map[string]interface{}{
				{
					"type":      "function_call",
					"call_id":   "call_resp_456",
					"name":      "read",
					"arguments": `{"path": "/tmp/test.txt"}`,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	t.Run("function call via responses API", func(t *testing.T) {
		client := NewClient("test-key", server.URL+"/v1", "gpt-4", "responses")
		messages := []Message{
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
					},
				},
			},
		}

		resp, err := client.Chat(context.Background(), messages, toolDefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
		}
		tc := resp.ToolCalls[0]
		if tc.ID != "call_resp_456" {
			t.Errorf("expected call ID 'call_resp_456', got '%s'", tc.ID)
		}
		if tc.Function.Name != "read" {
			t.Errorf("expected function name 'read', got '%s'", tc.Function.Name)
		}
		if tc.Function.Arguments != `{"path": "/tmp/test.txt"}` {
			t.Errorf("unexpected arguments: %s", tc.Function.Arguments)
		}
		if resp.FinishReason != "tool_calls" {
			t.Errorf("expected finish_reason 'tool_calls', got '%s'", resp.FinishReason)
		}
	})
}

func TestClientRetryOn429(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "rate limited"}}`))
			return
		}
		response := map[string]interface{}{
			"id":      "chatcmpl-retry",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Success after retries!",
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL, "gpt-4", "chat")
	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := client.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if resp.Content != "Success after retries!" {
		t.Errorf("expected 'Success after retries!', got '%s'", resp.Content)
	}
	if callCount.Load() < 3 {
		t.Errorf("expected at least 3 calls (2 retries + 1 success), got %d", callCount.Load())
	}
}

func TestWrapAPIError(t *testing.T) {
	t.Run("non-API error", func(t *testing.T) {
		err := wrapAPIError("test context", context.DeadlineExceeded)
		if !strings.Contains(err.Error(), "test context") {
			t.Errorf("expected context in error, got: %v", err)
		}
		if strings.Contains(err.Error(), "HTTP") {
			t.Errorf("non-API error should not contain HTTP status, got: %v", err)
		}
	})
}
