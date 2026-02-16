package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestEventEmitter(t *testing.T) {
	t.Run("subscribe and emit", func(t *testing.T) {
		emitter := NewEventEmitter()
		var received []AgentEvent

		emitter.Subscribe(func(e AgentEvent) {
			received = append(received, e)
		})

		emitter.Emit(AgentEvent{Type: EventAgentStart})
		emitter.Emit(AgentEvent{Type: EventToolStart, ToolName: "bash"})
		emitter.Emit(AgentEvent{Type: EventAgentEnd})

		if len(received) != 3 {
			t.Fatalf("expected 3 events, got %d", len(received))
		}
		if received[0].Type != EventAgentStart {
			t.Errorf("expected AgentStart, got %s", received[0].Type)
		}
		if received[1].ToolName != "bash" {
			t.Errorf("expected tool name 'bash', got %s", received[1].ToolName)
		}
		if received[2].Type != EventAgentEnd {
			t.Errorf("expected AgentEnd, got %s", received[2].Type)
		}
	})

	t.Run("multiple subscribers", func(t *testing.T) {
		emitter := NewEventEmitter()
		var count1, count2 int

		emitter.Subscribe(func(e AgentEvent) { count1++ })
		emitter.Subscribe(func(e AgentEvent) { count2++ })

		emitter.Emit(AgentEvent{Type: EventAgentStart})

		if count1 != 1 || count2 != 1 {
			t.Errorf("expected both subscribers to receive event, got %d and %d", count1, count2)
		}
	})

	t.Run("unsubscribe", func(t *testing.T) {
		emitter := NewEventEmitter()
		var count int

		unsub := emitter.Subscribe(func(e AgentEvent) { count++ })
		emitter.Emit(AgentEvent{Type: EventAgentStart})

		unsub()
		emitter.Emit(AgentEvent{Type: EventAgentEnd})

		if count != 1 {
			t.Errorf("expected 1 event after unsubscribe, got %d", count)
		}
	})

	t.Run("concurrent emit", func(t *testing.T) {
		emitter := NewEventEmitter()
		var mu sync.Mutex
		var count int

		emitter.Subscribe(func(e AgentEvent) {
			mu.Lock()
			count++
			mu.Unlock()
		})

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				emitter.Emit(AgentEvent{Type: EventTurnStart})
			}()
		}
		wg.Wait()

		if count != 100 {
			t.Errorf("expected 100 events, got %d", count)
		}
	})

	t.Run("no subscribers", func(t *testing.T) {
		emitter := NewEventEmitter()
		// Should not panic
		emitter.Emit(AgentEvent{Type: EventAgentStart})
	})
}

func TestProcessInputEmitsEvents(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var response map[string]interface{}

		if callCount == 1 {
			response = map[string]interface{}{
				"id": "chatcmpl-ev1", "object": "chat.completion",
				"created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant", "content": "",
							"tool_calls": []map[string]interface{}{
								{
									"id": "call_ev1", "type": "function",
									"function": map[string]interface{}{
										"name":      "bash",
										"arguments": `{"command":"echo hello"}`,
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
				"id": "chatcmpl-ev2", "object": "chat.completion",
				"created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Done!",
						},
						"finish_reason": "stop",
					},
				},
			}
		}
		writeSSEResponse(w, response)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	var events []AgentEvent
	app.Events().Subscribe(func(e AgentEvent) {
		events = append(events, e)
	})

	err := app.ProcessInput(context.Background(), "run something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify event sequence
	expectedTypes := []AgentEventType{
		EventAgentStart,
		EventTurnStart,
		EventToolStart, // bash
		EventToolEnd,   // bash
		EventTurnEnd,
		EventTurnStart,
		EventMessageEnd, // "Done!"
		EventTurnEnd,
		EventAgentEnd,
	}

	if len(events) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedTypes), len(events), eventTypes(events))
	}

	for i, expected := range expectedTypes {
		if events[i].Type != expected {
			t.Errorf("event[%d]: expected %s, got %s", i, expected, events[i].Type)
		}
	}

	// Verify ToolStart has tool name
	if events[2].ToolName != "bash" {
		t.Errorf("expected ToolStart with tool name 'bash', got '%s'", events[2].ToolName)
	}

	// Verify MessageEnd has content
	if events[6].Content != "Done!" {
		t.Errorf("expected MessageEnd with content 'Done!', got '%s'", events[6].Content)
	}

	// Verify AgentEnd has no error
	if events[8].Error != nil {
		t.Errorf("expected AgentEnd with no error, got: %v", events[8].Error)
	}
}

func TestProcessInputEmitsAgentEndOnError(t *testing.T) {
	// Server that always returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	var events []AgentEvent
	app.Events().Subscribe(func(e AgentEvent) {
		events = append(events, e)
	})

	err := app.ProcessInput(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}

	// Should still have AgentStart, TurnStart, TurnEnd, AgentEnd
	hasAgentEnd := false
	for _, e := range events {
		if e.Type == EventAgentEnd {
			hasAgentEnd = true
			if e.Error == nil {
				t.Error("expected AgentEnd to carry error")
			}
		}
	}
	if !hasAgentEnd {
		t.Error("expected AgentEnd event on error")
	}
}

func TestDefaultOutputHandlerToolStart(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	app.defaultOutputHandler(AgentEvent{Type: EventToolStart, ToolName: "read"})

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("[read]")) {
		t.Errorf("expected [read] in output, got: %s", output)
	}
}

func TestDefaultOutputHandlerToolEnd(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	app.defaultOutputHandler(AgentEvent{Type: EventToolEnd, ToolName: "bash", Content: "hello world\n"})

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("hello world")) {
		t.Errorf("expected tool output in output, got: %s", output)
	}
}

func TestDefaultOutputHandlerMessageEnd(t *testing.T) {
	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	app.defaultOutputHandler(AgentEvent{Type: EventMessageEnd, Content: "response text"})

	output := buf.String()
	if output != "\n\n" {
		t.Errorf("expected two newlines for MessageEnd, got: %q", output)
	}
}

func TestProcessInputTextOnlyEmitsEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id": "chatcmpl-txt", "object": "chat.completion",
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
		}
		writeSSEResponse(w, response)
	}))
	defer server.Close()

	cfg := &Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	app := NewApp(cfg)
	app.output = &bytes.Buffer{}

	var events []AgentEvent
	app.Events().Subscribe(func(e AgentEvent) {
		events = append(events, e)
	})

	err := app.ProcessInput(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTypes := []AgentEventType{
		EventAgentStart,
		EventTurnStart,
		EventMessageEnd,
		EventTurnEnd,
		EventAgentEnd,
	}

	if len(events) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedTypes), len(events), eventTypes(events))
	}

	for i, expected := range expectedTypes {
		if events[i].Type != expected {
			t.Errorf("event[%d]: expected %s, got %s", i, expected, events[i].Type)
		}
	}
}

func eventTypes(events []AgentEvent) []AgentEventType {
	types := make([]AgentEventType, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

// mockRespondJSON writes a JSON response (non-streaming).
func mockRespondJSON(w http.ResponseWriter, response map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
