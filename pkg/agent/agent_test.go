package agent

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

	"github.com/user/pigo/pkg/config"
	"github.com/user/pigo/pkg/skills"
	"github.com/user/pigo/pkg/types"
)

// writeSSEResponse converts a standard chat completion response map into SSE
// streaming format.
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

func isStreamingRequest(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return bytes.Contains(body, []byte(`"stream":true`)) || bytes.Contains(body, []byte(`"stream": true`))
}

func mockRespond(w http.ResponseWriter, r *http.Request, response map[string]interface{}) {
	if isStreamingRequest(r) {
		writeSSEResponse(w, response)
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func TestNewAgent(t *testing.T) {
	cfg := &config.Config{
		APIKey: "test-key",
		Model:  "gpt-4",
	}

	agent := NewAgent(cfg)
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.GetModel() != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", agent.GetModel())
	}
	if agent.GetRegistry() == nil {
		t.Error("expected non-nil registry")
	}

	tools := agent.GetRegistry().List()
	if len(tools) != 10 {
		t.Errorf("expected 10 tools, got %d", len(tools))
	}
}

func TestAgentHandleCommand(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	t.Run("quit command", func(t *testing.T) {
		handled, exit := agent.HandleCommand("/q")
		if !handled || !exit {
			t.Error("expected handled=true, exit=true for /q")
		}
	})

	t.Run("exit command", func(t *testing.T) {
		handled, exit := agent.HandleCommand("exit")
		if !handled || !exit {
			t.Error("expected handled=true, exit=true for exit")
		}
	})

	t.Run("clear command", func(t *testing.T) {
		agent.messages = append(agent.messages, types.Message{Role: "user", Content: "test"})
		initialLen := len(agent.messages)

		handled, exit := agent.HandleCommand("/c")
		if !handled || exit {
			t.Error("expected handled=true, exit=false for /c")
		}
		if len(agent.messages) != 1 {
			t.Errorf("expected 1 message after clear, got %d (was %d)", len(agent.messages), initialLen)
		}
	})

	t.Run("regular input", func(t *testing.T) {
		handled, exit := agent.HandleCommand("hello")
		if handled || exit {
			t.Error("expected handled=false, exit=false for regular input")
		}
	})

	t.Run("skills command no skills", func(t *testing.T) {
		buf := &bytes.Buffer{}
		agent.output = buf
		agent.Skills = nil
		handled, exit := agent.HandleCommand("/skills")
		if !handled || exit {
			t.Error("expected handled=true, exit=false for /skills")
		}
		if !bytes.Contains(buf.Bytes(), []byte("No skills loaded")) {
			t.Errorf("expected 'No skills loaded' in output, got: %s", buf.String())
		}
	})

	t.Run("skills command with skills", func(t *testing.T) {
		buf := &bytes.Buffer{}
		agent.output = buf
		agent.Skills = []skills.Skill{
			{Name: "greet", Description: "A greeting skill", Source: "user", FilePath: "/tmp/greet/SKILL.md"},
		}
		handled, exit := agent.HandleCommand("/skills")
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
		handled, exit := agent.HandleCommand("/skill:greet say hi")
		if handled || exit {
			t.Error("expected handled=false, exit=false for /skill:name (should flow to ProcessInput)")
		}
	})
}

func TestAgentProcessInput(t *testing.T) {
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

	cfg := &config.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-4",
	}

	t.Run("empty input", func(t *testing.T) {
		agent := NewAgent(cfg)
		agent.output = &bytes.Buffer{}

		err := agent.ProcessInput(context.Background(), "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("simple conversation", func(t *testing.T) {
		agent := NewAgent(cfg)
		output := &bytes.Buffer{}
		agent.output = output

		err := agent.ProcessInput(context.Background(), "Hello")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !bytes.Contains(output.Bytes(), []byte("Hello there!")) {
			t.Errorf("expected response in output, got: %s", output.String())
		}
	})
}

func TestAgentProcessInputWithToolCalls(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
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

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	output := &bytes.Buffer{}
	agent.output = output

	err := agent.ProcessInput(context.Background(), "Run echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputStr := output.String()
	if !bytes.Contains(output.Bytes(), []byte("[bash]")) {
		t.Errorf("expected [bash] in output, got: %s", outputStr)
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
func (s *slowTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	cur := s.running.Add(1)
	for {
		old := s.maxConc.Load()
		if cur <= old || s.maxConc.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(s.delay)
	s.running.Add(-1)
	return types.NewToolResult(fmt.Sprintf("done-%s", s.name))
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
				"id": "chatcmpl-par", "object": "chat.completion", "created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant", "content": "",
							"tool_calls": []map[string]interface{}{
								{"id": "call_sa", "type": "function", "function": map[string]interface{}{"name": "slow_a", "arguments": "{}"}},
								{"id": "call_sb", "type": "function", "function": map[string]interface{}{"name": "slow_b", "arguments": "{}"}},
								{"id": "call_sc", "type": "function", "function": map[string]interface{}{"name": "slow_c", "arguments": "{}"}},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
		} else {
			response = map[string]interface{}{
				"id": "chatcmpl-par2", "object": "chat.completion", "created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "Done."}, "finish_reason": "stop"},
				},
			}
		}
		mockRespond(w, r, response)
	}))
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}
	agent.registry.Register(toolA)
	agent.registry.Register(toolB)
	agent.registry.Register(toolC)

	err := agent.ProcessInput(context.Background(), "run slow tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if maxConc.Load() > 1 {
		t.Errorf("expected sequential execution (peak concurrency == 1), got %d", maxConc.Load())
	}

	var toolMsgs []types.Message
	for _, m := range agent.messages {
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
}

func TestTruncateMessages(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	output := &bytes.Buffer{}
	agent.output = output

	largeContent := strings.Repeat("x", 20000)
	for i := 0; i < 30; i++ {
		agent.messages = append(agent.messages, types.Message{
			Role:    "user",
			Content: fmt.Sprintf("msg-%d: %s", i, largeContent),
		})
	}

	agent.truncateMessages()

	outputStr := output.String()
	if !strings.Contains(outputStr, "context truncated") {
		t.Error("expected truncation warning in output")
	}

	if agent.messages[0].Role != "system" {
		t.Error("expected system prompt to be preserved as first message")
	}

	if !strings.Contains(agent.messages[1].Content, "truncated") {
		t.Error("expected truncation notice as second message")
	}
}

func TestProcessInputContextOverflowRetry(t *testing.T) {
	streamCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStreamingRequest(r) {
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

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	output := &bytes.Buffer{}
	agent.output = output

	for i := 0; i < 15; i++ {
		agent.messages = append(agent.messages, types.Message{
			Role:    "user",
			Content: fmt.Sprintf("msg %d: short message", i),
		})
	}

	err := agent.ProcessInput(context.Background(), "test retry")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	if streamCallCount < 2 {
		t.Errorf("expected at least 2 streaming API calls, got %d", streamCallCount)
	}

	if !strings.Contains(output.String(), "context overflow") {
		t.Errorf("expected overflow message in output, got: %s", output.String())
	}

	if !strings.Contains(output.String(), "After compaction!") {
		t.Errorf("expected retried response in output, got: %s", output.String())
	}
}

func TestProcessInputContextOverflowMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	err := agent.ProcessInput(context.Background(), "test max retries")
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

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}

	agent.ProcessInput(context.Background(), "hi")
	agent.ProcessInput(context.Background(), "hello again")

	u := agent.GetUsage()
	if u.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}
}

func TestUsageCommand(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	handled, exit := agent.HandleCommand("/usage")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /usage")
	}
	if !strings.Contains(buf.String(), "No tokens used") {
		t.Errorf("expected 'No tokens used' message, got: %s", buf.String())
	}

	buf.Reset()
	agent.addUsage(types.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	agent.HandleCommand("/usage")
	output := buf.String()
	if !strings.Contains(output, "100") || !strings.Contains(output, "50") || !strings.Contains(output, "150") {
		t.Errorf("expected usage numbers in output, got: %s", output)
	}
}

func TestQuitShowsUsage(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	agent.addUsage(types.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300})
	handled, exit := agent.HandleCommand("/q")
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
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	handled, exit := agent.HandleCommand("/model")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /model")
	}
	if !strings.Contains(buf.String(), "gpt-4") {
		t.Errorf("expected current model in output, got: %s", buf.String())
	}

	buf.Reset()
	handled, exit = agent.HandleCommand("/model gpt-4o-mini")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /model change")
	}
	if !strings.Contains(buf.String(), "gpt-4o-mini") {
		t.Errorf("expected new model in output, got: %s", buf.String())
	}
	if agent.GetModel() != "gpt-4o-mini" {
		t.Errorf("expected model to be changed to gpt-4o-mini, got: %s", agent.GetModel())
	}
}

func TestHandleCommandSessions(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	handled, exit := agent.HandleCommand("/sessions")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /sessions")
	}
}

func TestHandleCommandSaveLoad(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	// Save with default name
	handled, exit := agent.HandleCommand("/save")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /save")
	}

	// Save with custom name
	buf.Reset()
	handled, exit = agent.HandleCommand("/save test-session")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /save test-session")
	}
	if !strings.Contains(buf.String(), "test-session") {
		t.Errorf("expected session name in output, got: %s", buf.String())
	}

	// Load with missing name
	buf.Reset()
	handled, exit = agent.HandleCommand("/load ")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /load")
	}

	// Load the saved session
	buf.Reset()
	handled, exit = agent.HandleCommand("/load test-session")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /load test-session")
	}

	// Load nonexistent
	buf.Reset()
	handled, exit = agent.HandleCommand("/load nonexistent-abc-xyz")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /load nonexistent")
	}
}

func TestHandleCommandMemory(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	buf := &bytes.Buffer{}
	agent.output = buf

	// Memory when nil
	agent.Memory = nil
	handled, exit := agent.HandleCommand("/memory")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /memory")
	}
	if !strings.Contains(buf.String(), "No memories") {
		t.Errorf("expected 'No memories' message, got: %s", buf.String())
	}

	// Memory clear when nil
	buf.Reset()
	handled, exit = agent.HandleCommand("/memory clear")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /memory clear")
	}
}

func TestHandleCommandEvents(t *testing.T) {
	cfg := &config.Config{APIKey: "test", Model: "gpt-4"}
	agent := NewAgent(cfg)
	e := agent.Events()
	if e == nil {
		t.Error("expected non-nil EventEmitter")
	}
}
