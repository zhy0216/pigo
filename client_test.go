package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// mockOpenAIResponse represents a mock OpenAI chat completion response
type mockOpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func newMockResponse(content string, finishReason string, toolCalls []ToolCall) mockOpenAIResponse {
	resp := mockOpenAIResponse{
		ID:     "chatcmpl-test",
		Object: "chat.completion",
		Model:  "gpt-4",
	}
	resp.Choices = make([]struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}, 1)
	resp.Choices[0].Index = 0
	resp.Choices[0].Message.Role = "assistant"
	resp.Choices[0].Message.Content = content
	resp.Choices[0].FinishReason = finishReason

	if len(toolCalls) > 0 {
		resp.Choices[0].Message.ToolCalls = make([]struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		}, len(toolCalls))
		for i, tc := range toolCalls {
			resp.Choices[0].Message.ToolCalls[i].ID = tc.ID
			resp.Choices[0].Message.ToolCalls[i].Type = tc.Type
			resp.Choices[0].Message.ToolCalls[i].Function.Name = tc.Function.Name
			resp.Choices[0].Message.ToolCalls[i].Function.Arguments = tc.Function.Arguments
		}
	}
	return resp
}

func TestNewClient(t *testing.T) {
	t.Run("creates client with default settings", func(t *testing.T) {
		client := NewClient("test-api-key", "", "gpt-4")
		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", client.model)
		}
	})

	t.Run("creates client with custom base URL", func(t *testing.T) {
		client := NewClient("test-api-key", "http://localhost:8080", "gpt-4")
		if client == nil {
			t.Fatal("expected non-nil client")
		}
		if client.model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", client.model)
		}
	})
}

func TestGetModel(t *testing.T) {
	t.Run("returns model name", func(t *testing.T) {
		client := NewClient("test-api-key", "", "gpt-4-turbo")
		if got := client.GetModel(); got != "gpt-4-turbo" {
			t.Errorf("GetModel() = %s, want gpt-4-turbo", got)
		}
	})

	t.Run("returns different model", func(t *testing.T) {
		client := NewClient("test-api-key", "", "gpt-3.5-turbo")
		if got := client.GetModel(); got != "gpt-3.5-turbo" {
			t.Errorf("GetModel() = %s, want gpt-3.5-turbo", got)
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns environment variable when set", func(t *testing.T) {
		os.Setenv("TEST_VAR_PIGO", "test-value")
		defer os.Unsetenv("TEST_VAR_PIGO")

		got := GetEnvOrDefault("TEST_VAR_PIGO", "default")
		if got != "test-value" {
			t.Errorf("GetEnvOrDefault() = %s, want test-value", got)
		}
	})

	t.Run("returns default when env not set", func(t *testing.T) {
		os.Unsetenv("TEST_VAR_NOT_SET")

		got := GetEnvOrDefault("TEST_VAR_NOT_SET", "my-default")
		if got != "my-default" {
			t.Errorf("GetEnvOrDefault() = %s, want my-default", got)
		}
	})

	t.Run("returns default when env is empty", func(t *testing.T) {
		os.Setenv("TEST_VAR_EMPTY", "")
		defer os.Unsetenv("TEST_VAR_EMPTY")

		got := GetEnvOrDefault("TEST_VAR_EMPTY", "fallback")
		if got != "fallback" {
			t.Errorf("GetEnvOrDefault() = %s, want fallback", got)
		}
	})
}

func TestChat(t *testing.T) {
	t.Run("simple user message response", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Path, "/chat/completions") {
				t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
			}

			// Parse request body to verify message conversion
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			json.Unmarshal(body, &reqBody)

			// Return mock response
			resp := newMockResponse("Hello! How can I help?", "stop", nil)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		response, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Content != "Hello! How can I help?" {
			t.Errorf("unexpected content: %s", response.Content)
		}
		if response.FinishReason != "stop" {
			t.Errorf("unexpected finish reason: %s", response.FinishReason)
		}
	})

	t.Run("system and user messages", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := newMockResponse("I understand my role.", "stop", nil)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Who are you?"},
		}

		response, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Content != "I understand my role." {
			t.Errorf("unexpected content: %s", response.Content)
		}
	})

	t.Run("response with tool calls", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify tools are in request
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			json.Unmarshal(body, &reqBody)

			if _, ok := reqBody["tools"]; !ok {
				t.Error("expected tools in request")
			}

			// Return response with tool call
			toolCalls := []ToolCall{
				{
					ID:   "call_abc123",
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      "read_file",
						Arguments: `{"path": "/tmp/test.txt"}`,
					},
				},
			}
			resp := newMockResponse("", "tool_calls", toolCalls)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Read the file /tmp/test.txt"},
		}
		toolDefs := []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "read_file",
					"description": "Read a file from the filesystem",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "Path to the file",
							},
						},
						"required": []string{"path"},
					},
				},
			},
		}

		response, err := client.Chat(context.Background(), messages, toolDefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(response.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(response.ToolCalls))
		}
		if response.ToolCalls[0].Function.Name != "read_file" {
			t.Errorf("expected read_file, got %s", response.ToolCalls[0].Function.Name)
		}
		if response.ToolCalls[0].ID != "call_abc123" {
			t.Errorf("expected call_abc123, got %s", response.ToolCalls[0].ID)
		}
		if response.FinishReason != "tool_calls" {
			t.Errorf("expected tool_calls, got %s", response.FinishReason)
		}
	})

	t.Run("tool result message in conversation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the request contains tool message
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			json.Unmarshal(body, &reqBody)

			messages := reqBody["messages"].([]interface{})
			// Should have user, assistant with tool call, tool result
			if len(messages) < 3 {
				t.Errorf("expected at least 3 messages, got %d", len(messages))
			}

			resp := newMockResponse("The file contains 'Hello World'.", "stop", nil)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Read the file"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   "call_xyz",
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      "read_file",
							Arguments: `{"path": "/tmp/test.txt"}`,
						},
					},
				},
			},
			{Role: "tool", Content: "Hello World", ToolCallID: "call_xyz"},
		}

		response, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Content != "The file contains 'Hello World'." {
			t.Errorf("unexpected content: %s", response.Content)
		}
	})

	t.Run("assistant message without tool calls", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := newMockResponse("Continuing the conversation.", "stop", nil)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		}

		response, err := client.Chat(context.Background(), messages, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Content != "Continuing the conversation." {
			t.Errorf("unexpected content: %s", response.Content)
		}
	})

	t.Run("multiple tool calls in response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			toolCalls := []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      "read_file",
						Arguments: `{"path": "/a.txt"}`,
					},
				},
				{
					ID:   "call_2",
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      "read_file",
						Arguments: `{"path": "/b.txt"}`,
					},
				},
			}
			resp := newMockResponse("", "tool_calls", toolCalls)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Read both files"},
		}
		toolDefs := []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "read_file",
					"description": "Read a file",
					"parameters":  map[string]interface{}{},
				},
			},
		}

		response, err := client.Chat(context.Background(), messages, toolDefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(response.ToolCalls) != 2 {
			t.Fatalf("expected 2 tool calls, got %d", len(response.ToolCalls))
		}
		if response.ToolCalls[0].ID != "call_1" || response.ToolCalls[1].ID != "call_2" {
			t.Error("tool call IDs mismatch")
		}
	})

	t.Run("error on empty choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return response with no choices
			resp := mockOpenAIResponse{
				ID:      "chatcmpl-test",
				Object:  "chat.completion",
				Model:   "gpt-4",
				Choices: []struct {
					Index   int `json:"index"`
					Message struct {
						Role      string `json:"role"`
						Content   string `json:"content"`
						ToolCalls []struct {
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls,omitempty"`
					} `json:"message"`
					FinishReason string `json:"finish_reason"`
				}{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		_, err := client.Chat(context.Background(), messages, nil)
		if err == nil {
			t.Fatal("expected error for empty choices")
		}
		if !strings.Contains(err.Error(), "no choices") {
			t.Errorf("expected 'no choices' error, got: %v", err)
		}
	})

	t.Run("error on API failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "Internal server error"}}`))
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		_, err := client.Chat(context.Background(), messages, nil)
		if err == nil {
			t.Fatal("expected error for API failure")
		}
	})

	t.Run("tool definition without function skipped", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify tools handling
			body, _ := io.ReadAll(r.Body)
			var reqBody map[string]interface{}
			json.Unmarshal(body, &reqBody)

			// Check if tools array exists and has correct length
			tools, ok := reqBody["tools"].([]interface{})
			if ok && len(tools) != 1 {
				t.Errorf("expected 1 valid tool, got %d", len(tools))
			}

			resp := newMockResponse("OK", "stop", nil)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Test"},
		}
		// One valid tool definition, one invalid (missing function key)
		toolDefs := []map[string]interface{}{
			{"type": "invalid"}, // This should be skipped
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "valid_tool",
					"description": "A valid tool",
					"parameters":  map[string]interface{}{},
				},
			},
		}

		response, err := client.Chat(context.Background(), messages, toolDefs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.Content != "OK" {
			t.Errorf("unexpected content: %s", response.Content)
		}
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't respond immediately - let context cancel
			<-r.Context().Done()
		}))
		defer server.Close()

		client := NewClient("test-key", server.URL, "gpt-4")
		messages := []Message{
			{Role: "user", Content: "Hello"},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := client.Chat(ctx, messages, nil)
		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}
