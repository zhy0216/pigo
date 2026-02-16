package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeSSEResponse converts a standard chat completion response map into SSE
// streaming format. It breaks the response into chunks: one for content/tool_calls
// and one for the finish_reason.
func writeSSEResponse(w http.ResponseWriter, response map[string]interface{}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)

	id, _ := response["id"].(string)
	model, _ := response["model"].(string)
	created := response["created"]

	choices, _ := response["choices"].([]map[string]interface{})
	if len(choices) == 0 {
		return
	}
	choice := choices[0]
	message, _ := choice["message"].(map[string]interface{})
	finishReason, _ := choice["finish_reason"].(string)

	content, _ := message["content"].(string)
	toolCalls, hasToolCalls := message["tool_calls"].([]map[string]interface{})

	if hasToolCalls && len(toolCalls) > 0 {
		// Send tool call chunks: first chunk has role + tool call ids/names
		firstDelta := map[string]interface{}{
			"role": "assistant",
		}
		var tcDeltas []map[string]interface{}
		for i, tc := range toolCalls {
			tcDeltas = append(tcDeltas, map[string]interface{}{
				"index": i,
				"id":    tc["id"],
				"type":  tc["type"],
				"function": map[string]interface{}{
					"name":      tc["function"].(map[string]interface{})["name"],
					"arguments": "",
				},
			})
		}
		firstDelta["tool_calls"] = tcDeltas

		chunk := map[string]interface{}{
			"id": id, "object": "chat.completion.chunk",
			"created": created, "model": model,
			"choices": []map[string]interface{}{
				{"index": 0, "delta": firstDelta, "finish_reason": nil},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}

		// Send arguments chunks
		for i, tc := range toolCalls {
			args := tc["function"].(map[string]interface{})["arguments"].(string)
			argChunk := map[string]interface{}{
				"id": id, "object": "chat.completion.chunk",
				"created": created, "model": model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"tool_calls": []map[string]interface{}{
								{"index": i, "function": map[string]interface{}{"arguments": args}},
							},
						},
						"finish_reason": nil,
					},
				},
			}
			data, _ := json.Marshal(argChunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	} else if content != "" {
		// Send content chunk
		chunk := map[string]interface{}{
			"id": id, "object": "chat.completion.chunk",
			"created": created, "model": model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Send finish chunk (include usage if present)
	finishChunk := map[string]interface{}{
		"id": id, "object": "chat.completion.chunk",
		"created": created, "model": model,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": map[string]interface{}{}, "finish_reason": finishReason},
		},
	}
	if usage, ok := response["usage"]; ok {
		finishChunk["usage"] = usage
	}
	data, _ := json.Marshal(finishChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// isStreamingRequest checks if the request body contains stream:true.
func isStreamingRequest(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	// Match both "stream":true and "stream": true
	return bytes.Contains(body, []byte(`"stream":true`)) || bytes.Contains(body, []byte(`"stream": true`))
}

// mockRespond writes the response as SSE if streaming, or JSON otherwise.
func mockRespond(w http.ResponseWriter, r *http.Request, response map[string]interface{}) {
	if isStreamingRequest(r) {
		writeSSEResponse(w, response)
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing API key", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "")
		_, err := LoadConfig()
		if err == nil {
			t.Error("expected error for missing API key")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "test-key")
		t.Setenv("OPENAI_BASE_URL", "https://api.example.com")
		t.Setenv("PIGO_MODEL", "gpt-4")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.APIKey != "test-key" {
			t.Errorf("expected 'test-key', got '%s'", cfg.APIKey)
		}
		if cfg.BaseURL != "https://api.example.com" {
			t.Errorf("expected base URL, got '%s'", cfg.BaseURL)
		}
		if cfg.Model != "gpt-4" {
			t.Errorf("expected 'gpt-4', got '%s'", cfg.Model)
		}
	})

	t.Run("default model", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "test-key")
		t.Setenv("PIGO_MODEL", "")

		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Model != "gpt-4o" {
			t.Errorf("expected default 'gpt-4o', got '%s'", cfg.Model)
		}
	})
}

func TestNewApp(t *testing.T) {
	cfg := &Config{
		APIKey: "test-key",
		Model:  "gpt-4",
	}

	app := NewApp(cfg)
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.GetModel() != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", app.GetModel())
	}
	if app.GetRegistry() == nil {
		t.Error("expected non-nil registry")
	}

	// Check all tools are registered
	tools := app.GetRegistry().List()
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}
}

func TestAppHandleCommand(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	t.Run("quit command", func(t *testing.T) {
		handled, exit := app.HandleCommand("/q")
		if !handled || !exit {
			t.Error("expected handled=true, exit=true for /q")
		}
	})

	t.Run("exit command", func(t *testing.T) {
		handled, exit := app.HandleCommand("exit")
		if !handled || !exit {
			t.Error("expected handled=true, exit=true for exit")
		}
	})

	t.Run("clear command", func(t *testing.T) {
		// Add some messages first
		app.messages = append(app.messages, Message{Role: "user", Content: "test"})
		initialLen := len(app.messages)

		handled, exit := app.HandleCommand("/c")
		if !handled || exit {
			t.Error("expected handled=true, exit=false for /c")
		}
		if len(app.messages) != 1 {
			t.Errorf("expected 1 message after clear, got %d (was %d)", len(app.messages), initialLen)
		}
	})

	t.Run("regular input", func(t *testing.T) {
		handled, exit := app.HandleCommand("hello")
		if handled || exit {
			t.Error("expected handled=false, exit=false for regular input")
		}
	})

	t.Run("skills command no skills", func(t *testing.T) {
		buf := &bytes.Buffer{}
		app.output = buf
		app.skills = nil
		handled, exit := app.HandleCommand("/skills")
		if !handled || exit {
			t.Error("expected handled=true, exit=false for /skills")
		}
		if !bytes.Contains(buf.Bytes(), []byte("No skills loaded")) {
			t.Errorf("expected 'No skills loaded' in output, got: %s", buf.String())
		}
	})

	t.Run("skills command with skills", func(t *testing.T) {
		buf := &bytes.Buffer{}
		app.output = buf
		app.skills = []Skill{
			{Name: "greet", Description: "A greeting skill", Source: "user", FilePath: "/tmp/greet/SKILL.md"},
		}
		handled, exit := app.HandleCommand("/skills")
		if !handled || exit {
			t.Error("expected handled=true, exit=false for /skills")
		}
		out := buf.String()
		if !bytes.Contains(buf.Bytes(), []byte("/skill:greet")) {
			t.Errorf("expected '/skill:greet' in output, got: %s", out)
		}
		if !bytes.Contains(buf.Bytes(), []byte("[user]")) {
			t.Errorf("expected '[user]' in output, got: %s", out)
		}
	})

	t.Run("skill colon command not handled", func(t *testing.T) {
		handled, exit := app.HandleCommand("/skill:greet say hi")
		if handled || exit {
			t.Error("expected handled=false, exit=false for /skill:name (should flow to ProcessInput)")
		}
	})
}

func TestAppProcessInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						"content": "Hello there!",
					},
					"finish_reason": "stop",
				},
			},
		}
		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	t.Run("empty input", func(t *testing.T) {
		app := NewApp(cfg)
		app.output = &bytes.Buffer{}

		err := app.ProcessInput(context.Background(), "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("simple conversation", func(t *testing.T) {
		app := NewApp(cfg)
		output := &bytes.Buffer{}
		app.output = output

		err := app.ProcessInput(context.Background(), "Hello")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !bytes.Contains(output.Bytes(), []byte("Hello there!")) {
			t.Errorf("expected response in output, got: %s", output.String())
		}
	})
}

func TestAppProcessInputWithToolCalls(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
			// First call: return tool call
			response = map[string]interface{}{
				"id":      "chatcmpl-tc1",
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
									"id":   "call_001",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": "{\"command\": \"echo hello\"}",
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			// Second call: return final response
			response = map[string]interface{}{
				"id":      "chatcmpl-tc2",
				"object":  "chat.completion",
				"created": 1677652288,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Done! The command printed hello.",
						},
						"finish_reason": "stop",
					},
				},
			}
		}

		mockRespond(w, r, response)
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

	err := app.ProcessInput(context.Background(), "Run echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that tool was executed
	outputStr := output.String()
	if !bytes.Contains(output.Bytes(), []byte("[bash]")) {
		t.Errorf("expected [bash] in output, got: %s", outputStr)
	}
}

func TestAppProcessInputWithInvalidToolArgs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":      "chatcmpl-err",
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
								"id":   "call_bad",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "read",
									"arguments": "invalid json",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	// Should handle invalid JSON gracefully but hit max iterations since mock always returns tool calls
	err := app.ProcessInput(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for max iterations, got nil")
	}
	if !strings.Contains(err.Error(), "maximum iterations") {
		t.Fatalf("expected max iterations error, got: %v", err)
	}
}

func TestAppProcessInputConcurrentToolCalls(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
			// Return 3 concurrent tool calls
			response = map[string]interface{}{
				"id":      "chatcmpl-conc",
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
									"id":   "call_a",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command": "echo aaa"}`,
									},
								},
								{
									"id":   "call_b",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command": "echo bbb"}`,
									},
								},
								{
									"id":   "call_c",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command": "echo ccc"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			response = map[string]interface{}{
				"id":      "chatcmpl-conc2",
				"object":  "chat.completion",
				"created": 1677652288,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "All done.",
						},
						"finish_reason": "stop",
					},
				},
			}
		}

		mockRespond(w, r, response)
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

	err := app.ProcessInput(context.Background(), "run three commands")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := output.String()

	// All three tool names should appear
	if !strings.Contains(outputStr, "[bash]") {
		t.Errorf("expected [bash] in output, got: %s", outputStr)
	}

	// All three tool results should be in messages, in order
	var toolMsgs []Message
	for _, m := range app.messages {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}
	if len(toolMsgs) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(toolMsgs))
	}
	if toolMsgs[0].ToolCallID != "call_a" {
		t.Errorf("expected first tool result for call_a, got %s", toolMsgs[0].ToolCallID)
	}
	if toolMsgs[1].ToolCallID != "call_b" {
		t.Errorf("expected second tool result for call_b, got %s", toolMsgs[1].ToolCallID)
	}
	if toolMsgs[2].ToolCallID != "call_c" {
		t.Errorf("expected third tool result for call_c, got %s", toolMsgs[2].ToolCallID)
	}
}

// slowTool is a test tool that sleeps for a given duration.
type slowTool struct {
	name    string
	delay   time.Duration
	running *atomic.Int32
	maxConc *atomic.Int32
}

func (s *slowTool) Name() string        { return s.name }
func (s *slowTool) Description() string { return "slow tool for testing" }
func (s *slowTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}
func (s *slowTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	cur := s.running.Add(1)
	// Track peak concurrency
	for {
		old := s.maxConc.Load()
		if cur <= old || s.maxConc.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(s.delay)
	s.running.Add(-1)
	return NewToolResult(fmt.Sprintf("done-%s", s.name))
}

func TestSequentialToolExecution(t *testing.T) {
	var running atomic.Int32
	var maxConc atomic.Int32

	toolA := &slowTool{name: "slow_a", delay: 100 * time.Millisecond, running: &running, maxConc: &maxConc}
	toolB := &slowTool{name: "slow_b", delay: 100 * time.Millisecond, running: &running, maxConc: &maxConc}
	toolC := &slowTool{name: "slow_c", delay: 100 * time.Millisecond, running: &running, maxConc: &maxConc}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
			response = map[string]interface{}{
				"id":      "chatcmpl-par",
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
									"id":   "call_sa",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "slow_a",
										"arguments": "{}",
									},
								},
								{
									"id":   "call_sb",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "slow_b",
										"arguments": "{}",
									},
								},
								{
									"id":   "call_sc",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "slow_c",
										"arguments": "{}",
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			response = map[string]interface{}{
				"id":      "chatcmpl-par2",
				"object":  "chat.completion",
				"created": 1677652288,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Done.",
						},
						"finish_reason": "stop",
					},
				},
			}
		}

		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	app := NewApp(cfg)
	app.output = &bytes.Buffer{}
	app.registry.Register(toolA)
	app.registry.Register(toolB)
	app.registry.Register(toolC)

	start := time.Now()
	err := app.ProcessInput(context.Background(), "run slow tools")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify sequential execution via peak concurrency
	_ = elapsed
	if maxConc.Load() > 1 {
		t.Errorf("expected sequential execution (peak concurrency == 1), got %d", maxConc.Load())
	}

	// Results should still be in order
	var toolMsgs []Message
	for _, m := range app.messages {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}
	if len(toolMsgs) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(toolMsgs))
	}
	if toolMsgs[0].ToolCallID != "call_sa" {
		t.Errorf("expected first result for call_sa, got %s", toolMsgs[0].ToolCallID)
	}
	if toolMsgs[1].ToolCallID != "call_sb" {
		t.Errorf("expected second result for call_sb, got %s", toolMsgs[1].ToolCallID)
	}
	if toolMsgs[2].ToolCallID != "call_sc" {
		t.Errorf("expected third result for call_sc, got %s", toolMsgs[2].ToolCallID)
	}
}

func TestToolExecutionInterrupt(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}
		if callCount == 1 {
			response = map[string]interface{}{
				"id": "chatcmpl-int", "object": "chat.completion", "created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant", "content": "",
							"tool_calls": []map[string]interface{}{
								{"id": "call_1", "type": "function", "function": map[string]interface{}{"name": "bash", "arguments": `{"command":"echo a"}`}},
								{"id": "call_2", "type": "function", "function": map[string]interface{}{"name": "bash", "arguments": `{"command":"echo b"}`}},
								{"id": "call_3", "type": "function", "function": map[string]interface{}{"name": "bash", "arguments": `{"command":"echo c"}`}},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			response = map[string]interface{}{
				"id": "chatcmpl-int2", "object": "chat.completion", "created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "Done."}, "finish_reason": "stop"},
				},
			}
		}
		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	// Cancel context immediately to test interrupt path
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := app.ProcessInput(ctx, "run tools")
	// Should either error or complete gracefully with skipped results
	_ = err

	// Check that skipped messages exist
	var toolMsgs []Message
	for _, m := range app.messages {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}
	// All tool results should be present (skipped ones too)
	if len(toolMsgs) > 0 {
		hasSkipped := false
		for _, m := range toolMsgs {
			if strings.Contains(m.Content, "Skipped") || strings.Contains(m.Content, "interrupt") {
				hasSkipped = true
			}
		}
		if len(toolMsgs) == 3 && !hasSkipped {
			t.Error("expected at least some skipped tool results after interrupt")
		}
	}
}

func TestConcurrentToolCallsWithMixedErrors(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
			response = map[string]interface{}{
				"id":      "chatcmpl-mix",
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
									"id":   "call_ok",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command": "echo ok"}`,
									},
								},
								{
									"id":   "call_bad_json",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": "not json",
									},
								},
								{
									"id":   "call_ok2",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command": "echo ok2"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			response = map[string]interface{}{
				"id":      "chatcmpl-mix2",
				"object":  "chat.completion",
				"created": 1677652288,
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Done.",
						},
						"finish_reason": "stop",
					},
				},
			}
		}

		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	err := app.ProcessInput(context.Background(), "run mixed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolMsgs []Message
	for _, m := range app.messages {
		if m.Role == "tool" {
			toolMsgs = append(toolMsgs, m)
		}
	}

	if len(toolMsgs) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(toolMsgs))
	}

	// First: successful
	if toolMsgs[0].ToolCallID != "call_ok" {
		t.Errorf("expected call_ok, got %s", toolMsgs[0].ToolCallID)
	}
	if strings.Contains(toolMsgs[0].Content, "failed to parse") {
		t.Errorf("first call should succeed, got: %s", toolMsgs[0].Content)
	}

	// Second: parse error
	if toolMsgs[1].ToolCallID != "call_bad_json" {
		t.Errorf("expected call_bad_json, got %s", toolMsgs[1].ToolCallID)
	}
	if !strings.Contains(toolMsgs[1].Content, "failed to parse") {
		t.Errorf("second call should have parse error, got: %s", toolMsgs[1].Content)
	}

	// Third: successful
	if toolMsgs[2].ToolCallID != "call_ok2" {
		t.Errorf("expected call_ok2, got %s", toolMsgs[2].ToolCallID)
	}
	if strings.Contains(toolMsgs[2].Content, "failed to parse") {
		t.Errorf("third call should succeed, got: %s", toolMsgs[2].Content)
	}
}

func TestTruncateMessages(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	output := &bytes.Buffer{}
	app.output = output

	// Fill with 30 large messages (20K chars each = 600K total, well above 200K threshold)
	largeContent := strings.Repeat("x", 20000)
	for i := 0; i < 30; i++ {
		app.messages = append(app.messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("msg-%d: %s", i, largeContent),
		})
	}

	// Test the truncateMessages fallback directly
	app.truncateMessages()

	// Verify truncation happened
	outputStr := output.String()
	if !strings.Contains(outputStr, "context truncated") {
		t.Error("expected truncation warning in output")
	}

	// System prompt should be preserved
	if app.messages[0].Role != "system" {
		t.Error("expected system prompt to be preserved as first message")
	}

	// Truncation notice should be second message
	if !strings.Contains(app.messages[1].Content, "truncated") {
		t.Error("expected truncation notice as second message")
	}
}

func TestProcessInputContextOverflowRetry(t *testing.T) {
	streamCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStreamingRequest(r) {
			// Non-streaming calls (e.g. compaction summarization) - return success
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "chatcmpl-sum", "object": "chat.completion",
				"created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "summary of earlier conversation",
						},
						"finish_reason": "stop",
					},
				},
			})
			return
		}

		streamCallCount++
		if streamCallCount == 1 {
			// First streaming call: return context overflow error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "This model's maximum context length is 8192 tokens. However, your messages resulted in 10000 tokens.",
					"type":    "invalid_request_error",
					"code":    "context_length_exceeded",
				},
			})
			return
		}
		// Subsequent streaming calls: return success
		response := map[string]interface{}{
			"id": "chatcmpl-retry", "object": "chat.completion",
			"created": 1677652288, "model": "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "After compaction!",
					},
					"finish_reason": "stop",
				},
			},
		}
		writeSSEResponse(w, response)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	output := &bytes.Buffer{}
	app.output = output

	// Add small messages so compactMessages is a no-op but we still have
	// something to truncate when the overflow retry triggers truncateMessages
	for i := 0; i < 15; i++ {
		app.messages = append(app.messages, Message{
			Role:    "user",
			Content: fmt.Sprintf("msg %d: short message", i),
		})
	}

	err := app.ProcessInput(context.Background(), "test retry")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	// Should have retried
	if streamCallCount < 2 {
		t.Errorf("expected at least 2 streaming API calls (overflow + retry), got %d", streamCallCount)
	}

	// Output should mention compaction
	if !strings.Contains(output.String(), "context overflow") {
		t.Errorf("expected overflow message in output, got: %s", output.String())
	}

	// Should have received the retried response
	if !strings.Contains(output.String(), "After compaction!") {
		t.Errorf("expected retried response in output, got: %s", output.String())
	}
}

func TestProcessInputContextOverflowMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return context overflow
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "This model's maximum context length is 8192 tokens.",
				"type":    "invalid_request_error",
				"code":    "context_length_exceeded",
			},
		})
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	err := app.ProcessInput(context.Background(), "test max retries")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "chat error") {
		t.Errorf("expected chat error, got: %v", err)
	}
}

func TestUsageTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "chatcmpl-usage", "object": "chat.completion",
			"created": 1677652288, "model": "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     float64(50),
				"completion_tokens": float64(10),
				"total_tokens":      float64(60),
			},
		}
		writeSSEResponse(w, response)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	// First call
	err := app.ProcessInput(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call
	err = app.ProcessInput(context.Background(), "hello again")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Usage should be accumulated across calls
	u := app.GetUsage()
	if u.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}
}

func TestUsageCommand(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	// No usage yet
	handled, exit := app.HandleCommand("/usage")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /usage")
	}
	if !strings.Contains(buf.String(), "No tokens used") {
		t.Errorf("expected 'No tokens used' message, got: %s", buf.String())
	}

	// Add some usage
	buf.Reset()
	app.addUsage(TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	app.HandleCommand("/usage")
	output := buf.String()
	if !strings.Contains(output, "100") || !strings.Contains(output, "50") || !strings.Contains(output, "150") {
		t.Errorf("expected usage numbers in output, got: %s", output)
	}
}

func TestQuitShowsUsage(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	app.addUsage(TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300})
	handled, exit := app.HandleCommand("/q")
	if !handled || !exit {
		t.Error("expected handled=true, exit=true for /q")
	}
	output := buf.String()
	if !strings.Contains(output, "300") {
		t.Errorf("expected total tokens in quit output, got: %s", output)
	}
	if !strings.Contains(output, "Goodbye") {
		t.Errorf("expected Goodbye in output, got: %s", output)
	}
}

func TestModelCommand(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	// Show current model
	handled, exit := app.HandleCommand("/model")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /model")
	}
	if !strings.Contains(buf.String(), "gpt-4") {
		t.Errorf("expected current model in output, got: %s", buf.String())
	}

	// Change model
	buf.Reset()
	handled, exit = app.HandleCommand("/model gpt-4o-mini")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /model change")
	}
	if !strings.Contains(buf.String(), "gpt-4o-mini") {
		t.Errorf("expected new model in output, got: %s", buf.String())
	}
	if app.GetModel() != "gpt-4o-mini" {
		t.Errorf("expected model to be changed to gpt-4o-mini, got: %s", app.GetModel())
	}

	// Verify current model after change
	buf.Reset()
	app.HandleCommand("/model")
	if !strings.Contains(buf.String(), "gpt-4o-mini") {
		t.Errorf("expected gpt-4o-mini, got: %s", buf.String())
	}
}

func TestSetModel(t *testing.T) {
	client := NewClient("test-key", "", "gpt-4", "chat")
	if client.GetModel() != "gpt-4" {
		t.Errorf("expected gpt-4, got %s", client.GetModel())
	}
	client.SetModel("gpt-3.5-turbo")
	if client.GetModel() != "gpt-3.5-turbo" {
		t.Errorf("expected gpt-3.5-turbo, got %s", client.GetModel())
	}
}
