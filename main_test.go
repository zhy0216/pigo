package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
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

func TestConcurrentToolExecutionActuallyParallel(t *testing.T) {
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

	// Verify parallel execution via peak concurrency, not wall-clock time.
	// Wall-clock assertions are flaky on slow CI runners.
	_ = elapsed
	if maxConc.Load() < 2 {
		t.Errorf("expected concurrent execution (peak concurrency >= 2), got %d", maxConc.Load())
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
