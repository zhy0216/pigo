package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
)

func TestNewOpenAIProvider(t *testing.T) {
	t.Run("create client with defaults", func(t *testing.T) {
		client := NewOpenAIProvider("test-key", "", "gpt-4", "chat")
		if client.GetModel() != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got '%s'", client.GetModel())
		}
	})

	t.Run("create client with custom base URL", func(t *testing.T) {
		client := NewOpenAIProvider("test-key", "https://custom.api.com", "gpt-4o", "chat")
		if client.GetModel() != "gpt-4o" {
			t.Errorf("expected model 'gpt-4o', got '%s'", client.GetModel())
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		result := util.GetEnvOrDefault("NONEXISTENT_VAR_12345", "default_value")
		if result != "default_value" {
			t.Errorf("expected 'default_value', got '%s'", result)
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_PIGO_VAR", "env_value")
		result := util.GetEnvOrDefault("TEST_PIGO_VAR", "default")
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
		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
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
		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
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
		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
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

func TestClientChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("{\"error\": {\"message\": \"Invalid API key\"}}"))
	}))
	defer server.Close()

	t.Run("API error handling", func(t *testing.T) {
		client := NewOpenAIProvider("invalid-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
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
		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
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
		client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
		messages := []types.Message{
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
		client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
		messages := []types.Message{
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

	client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
	messages := []types.Message{
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

func TestIsContextOverflow(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if IsContextOverflow(nil) {
			t.Error("expected false for nil error")
		}
	})

	t.Run("non-API error", func(t *testing.T) {
		if IsContextOverflow(fmt.Errorf("some error")) {
			t.Error("expected false for non-API error")
		}
	})

	t.Run("context overflow 400", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "This model's maximum context length is 8192 tokens. However, your messages resulted in 10000 tokens.",
					"type":    "invalid_request_error",
					"code":    "context_length_exceeded",
				},
			})
		}))
		defer server.Close()

		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		_, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil)
		if !IsContextOverflow(err) {
			t.Errorf("expected true for context length exceeded, got false. Error: %v", err)
		}
	})

	t.Run("context window exceeded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Your input exceeds the context window of this model",
					"type":    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		_, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil)
		if !IsContextOverflow(err) {
			t.Errorf("expected true for context window exceeded, got false. Error: %v", err)
		}
	})

	t.Run("too many tokens", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Too many tokens in the request",
					"type":    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		_, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil)
		if !IsContextOverflow(err) {
			t.Errorf("expected true for too many tokens, got false. Error: %v", err)
		}
	})

	t.Run("unrelated 400 error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid model specified",
					"type":    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		_, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil)
		if IsContextOverflow(err) {
			t.Error("expected false for unrelated 400 error")
		}
	})
}

func TestSetModel(t *testing.T) {
	client := NewOpenAIProvider("test-key", "", "gpt-4", "chat")
	if client.GetModel() != "gpt-4" {
		t.Errorf("expected gpt-4, got %s", client.GetModel())
	}
	client.SetModel("gpt-3.5-turbo")
	if client.GetModel() != "gpt-3.5-turbo" {
		t.Errorf("expected gpt-3.5-turbo, got %s", client.GetModel())
	}
}

func TestChatStreamViaCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("expected flusher")
			return
		}

		chunks := []string{
			`{"id":"chatcmpl-stream1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-stream1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-stream1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	t.Run("text streaming", func(t *testing.T) {
		client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
		messages := []types.Message{
			{Role: "user", Content: "Hello"},
		}

		var buf strings.Builder
		resp, err := client.ChatStream(context.Background(), messages, nil, &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "Hello") {
			t.Errorf("expected streamed text to contain 'Hello', got '%s'", buf.String())
		}
		if resp.FinishReason != "stop" {
			t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
		}
	})
}

func TestChatStreamViaCompletionsWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_s1","type":"function","function":{"name":"read","arguments":""}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/t.txt\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
	messages := []types.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Read /tmp/t.txt"},
	}
	toolDefs := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read",
				"description": "Read a file",
				"parameters":  map[string]interface{}{"type": "object"},
			},
		},
	}

	var buf strings.Builder
	resp, err := client.ChatStream(context.Background(), messages, toolDefs, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read" {
		t.Errorf("expected tool name 'read', got '%s'", resp.ToolCalls[0].Function.Name)
	}
}

func TestChatStreamViaCompletionsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "server error"}}`))
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
	var buf strings.Builder
	_, err := client.ChatStream(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil, &buf)
	if err == nil {
		t.Error("expected error for server failure")
	}
}

func TestChatStreamViaResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []string{
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi \"}\n\n",
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"there!\"}\n\n",
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
	messages := []types.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Hi"},
	}

	var buf strings.Builder
	resp, err := client.ChatStream(context.Background(), messages, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Hi ") {
		t.Errorf("expected streamed 'Hi ', got '%s'", buf.String())
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
	}
}

func TestChatStreamViaResponsesWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		events := []string{
			"event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_r1\",\"name\":\"read\"}}\n\n",
			"event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{\\\"path\\\"\"}\n\n",
			"event: response.function_call_arguments.done\ndata: {\"type\":\"response.function_call_arguments.done\",\"arguments\":\"{\\\"path\\\":\\\"/tmp/test\\\"}\"}\n\n",
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":8,\"output_tokens\":3,\"total_tokens\":11}}}\n\n",
			"data: [DONE]\n\n",
		}
		for _, ev := range events {
			fmt.Fprint(w, ev)
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
	messages := []types.Message{
		{Role: "user", Content: "Read /tmp/test"},
	}
	toolDefs := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "read",
				"description": "Read a file",
				"parameters":  map[string]interface{}{"type": "object"},
			},
		},
	}

	var buf strings.Builder
	resp, err := client.ChatStream(context.Background(), messages, toolDefs, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read" {
		t.Errorf("expected 'read', got '%s'", resp.ToolCalls[0].Function.Name)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected 'tool_calls', got '%s'", resp.FinishReason)
	}
}

func TestChatStreamViaResponsesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "server error"}}`))
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
	var buf strings.Builder
	_, err := client.ChatStream(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, nil, &buf)
	if err == nil {
		t.Error("expected error for server failure")
	}
}

func TestChatStreamWithAllMessageTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"id":"chatcmpl-msg","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-msg","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
	// Include all message types: system, user, assistant with tool calls, tool result
	messages := []types.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Read file"},
		{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "call_1", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path":"/tmp/x"}`}},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "call_1"},
		{Role: "user", Content: "Summarize that"},
	}

	var buf strings.Builder
	resp, err := client.ChatStream(context.Background(), messages, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected 'OK', got '%s'", resp.Content)
	}
}

func TestChatCompletionWithMalformedToolDef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "chatcmpl-mf", "object": "chat.completion", "created": 1677652288, "model": "gpt-4",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "OK"}, "finish_reason": "stop"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL, "gpt-4", "chat")
	// Malformed tool def (no "function" key)
	toolDefs := []map[string]interface{}{
		{"type": "function", "not_function": "oops"},
	}
	resp, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, toolDefs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected 'OK', got '%s'", resp.Content)
	}
}

func TestResponsesAPIMalformedToolDef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "resp-mf", "object": "response",
			"output": []map[string]interface{}{
				{"type": "message", "role": "assistant", "content": []map[string]interface{}{
					{"type": "output_text", "text": "OK"},
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
	toolDefs := []map[string]interface{}{
		{"type": "function", "bad": "no function key"},
	}
	resp, err := client.Chat(context.Background(), []types.Message{{Role: "user", Content: "hi"}}, toolDefs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected 'OK', got '%s'", resp.Content)
	}
}

func TestResponsesAPIWithAllMessageTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "resp-all", "object": "response",
			"output": []map[string]interface{}{
				{"type": "message", "role": "assistant", "content": []map[string]interface{}{
					{"type": "output_text", "text": "Summary done"},
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOpenAIProvider("test-key", server.URL+"/v1", "gpt-4", "responses")
	messages := []types.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Read file"},
		{Role: "assistant", ToolCalls: []types.ToolCall{
			{ID: "call_r1", Type: "function", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "read", Arguments: `{"path":"/tmp/x"}`}},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "call_r1"},
		{Role: "assistant", Content: "I read the file"},
		{Role: "user", Content: "Summarize"},
	}

	resp, err := client.Chat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Summary done" {
		t.Errorf("expected 'Summary done', got '%s'", resp.Content)
	}
}
