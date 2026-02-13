package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	// Should handle invalid JSON gracefully
	err := app.ProcessInput(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
