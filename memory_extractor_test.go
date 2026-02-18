package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no fence", `[{"category": "profile"}]`, `[{"category": "profile"}]`},
		{"json fence", "```json\n[{\"category\": \"profile\"}]\n```", `[{"category": "profile"}]`},
		{"plain fence", "```\n[{\"category\": \"profile\"}]\n```", `[{"category": "profile"}]`},
		{"with whitespace", "  ```json\n[]\n```  ", `[]`},
		{"no closing fence", "```json\n[]", `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFence(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestFormatMessagesForExtraction(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "assistant", ToolCalls: []ToolCall{{Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "read", Arguments: `{"path": "/tmp/test.go"}`}}}},
		{Role: "tool", Content: "file contents here"},
	}

	result := formatMessagesForExtraction(messages)
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !strings.Contains(result, "User: Hello world") {
		t.Error("should contain user message")
	}
	if !strings.Contains(result, "Assistant: Hi there") {
		t.Error("should contain assistant message")
	}
	if !strings.Contains(result, "Tool call: read") {
		t.Error("should contain tool call")
	}
	if !strings.Contains(result, "Tool result: file contents here") {
		t.Error("should contain tool result")
	}
}

func TestFormatMessagesForExtractionEmpty(t *testing.T) {
	result := formatMessagesForExtraction(nil)
	if result != "" {
		t.Errorf("expected empty result for nil messages, got %q", result)
	}
}

// newExtractorTestApp creates an App with a mock server for extractor testing.
func newExtractorTestApp(t *testing.T, chatResp func(w http.ResponseWriter, r *http.Request), embedResp func(w http.ResponseWriter, r *http.Request)) (*App, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "embeddings") {
			if embedResp != nil {
				embedResp(w, r)
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"object": "list",
					"data": []map[string]interface{}{
						{"object": "embedding", "index": 0, "embedding": []float64{0.5, 0.6, 0.7}},
					},
				})
			}
			return
		}
		if chatResp != nil {
			chatResp(w, r)
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-test",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "[]"},
						"finish_reason": "stop",
					},
				},
			})
		}
	}))

	app := &App{
		client: NewClient("test-key", server.URL, "gpt-4", "chat"),
		memory: NewMemoryStore(),
		output: &bytes.Buffer{},
		events: NewEventEmitter(),
	}
	return app, server
}

// --- extractMemories ---

func TestExtractMemoriesNilMemoryStore(t *testing.T) {
	app, server := newExtractorTestApp(t, nil, nil)
	defer server.Close()

	app.memory = nil
	// Should return early without panicking
	app.extractMemories(context.Background(), []Message{
		{Role: "user", Content: "Remember my name is Alice"},
	})
	// No assertion needed; just verifying it doesn't panic
}

func TestExtractMemoriesEmptyMessages(t *testing.T) {
	app, server := newExtractorTestApp(t, nil, nil)
	defer server.Close()

	// Should return early without calling LLM
	app.extractMemories(context.Background(), []Message{})
	if app.memory.Count() != 0 {
		t.Errorf("expected 0 memories, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesNilMessages(t *testing.T) {
	app, server := newExtractorTestApp(t, nil, nil)
	defer server.Close()

	app.extractMemories(context.Background(), nil)
	if app.memory.Count() != 0 {
		t.Errorf("expected 0 memories, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesSuccessfulCreate(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			// Return a candidate memory for extraction
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-extract",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `[{"category":"profile","abstract":"User name is Alice","overview":"The user's name is Alice","content":"Alice is a software developer"}]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "My name is Alice and I'm a software developer"},
		{Role: "assistant", Content: "Nice to meet you, Alice!"},
	}

	app.extractMemories(context.Background(), messages)

	if app.memory.Count() != 1 {
		t.Fatalf("expected 1 memory created, got %d", app.memory.Count())
	}

	all := app.memory.All()
	if all[0].Abstract != "User name is Alice" {
		t.Errorf("expected abstract 'User name is Alice', got '%s'", all[0].Abstract)
	}
	if all[0].Category != CatProfile {
		t.Errorf("expected category 'profile', got '%s'", all[0].Category)
	}
}

func TestExtractMemoriesInvalidCategory(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-invalid-cat",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `[{"category":"invalid_category","abstract":"Something","overview":"Something","content":"Something"}]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "Some conversation"},
	}

	app.extractMemories(context.Background(), messages)

	if app.memory.Count() != 0 {
		t.Errorf("expected 0 memories for invalid category, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesLLMError(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "LLM error"}}`))
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "Important information"},
	}

	app.extractMemories(context.Background(), messages)

	output := app.output.(*bytes.Buffer).String()
	if !strings.Contains(output, "memory extraction failed") {
		t.Errorf("expected extraction failure message in output, got: %s", output)
	}
	if app.memory.Count() != 0 {
		t.Errorf("expected 0 memories on LLM error, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesMultipleCandidates(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-multi",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant",
							"content": `[
								{"category":"profile","abstract":"User is Bob","overview":"Bob overview","content":"Bob content"},
								{"category":"entities","abstract":"Project X exists","overview":"Project X overview","content":"Project X content"}
							]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "I'm Bob and I work on Project X"},
	}

	app.extractMemories(context.Background(), messages)

	if app.memory.Count() != 2 {
		t.Fatalf("expected 2 memories created, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesWithMerge(t *testing.T) {
	// Test extraction where dedup decides to merge with existing memory
	callCount := 0
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			switch callCount {
			case 1:
				// First call: extraction returns a candidate
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     "chatcmpl-extract-merge",
					"object": "chat.completion",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": `[{"category":"profile","abstract":"User role updated","overview":"Updated role","content":"New role details"}]`,
							},
							"finish_reason": "stop",
						},
					},
				})
			case 2:
				// Second call: dedup LLM decision (only reached if FindSimilar found something)
				// Since profile is alwaysMerge, this won't be called;
				// the dedup will directly return MERGE. So this is the merge call.
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     "chatcmpl-merge",
					"object": "chat.completion",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": `{"abstract":"Combined profile","overview":"Combined overview","content":"Combined content"}`,
							},
							"finish_reason": "stop",
						},
					},
				})
			default:
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     "chatcmpl-default",
					"object": "chat.completion",
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"message":       map[string]interface{}{"role": "assistant", "content": "ok"},
							"finish_reason": "stop",
						},
					},
				})
			}
		},
		nil,
	)
	defer server.Close()

	// Add existing profile memory with matching vector
	app.memory.Add(&Memory{
		ID:       "mem_existing_profile",
		Category: CatProfile,
		Abstract: "User is a developer",
		Overview: "Developer overview",
		Content:  "Developer content",
		Vector:   []float64{0.5, 0.6, 0.7}, // matches mock embedding
	})

	messages := []Message{
		{Role: "user", Content: "I'm now a senior developer"},
	}

	app.extractMemories(context.Background(), messages)

	// Should still have 1 memory (merged, not created new)
	if app.memory.Count() != 1 {
		t.Fatalf("expected 1 memory after merge, got %d", app.memory.Count())
	}

	mem := app.memory.Get("mem_existing_profile")
	if mem == nil {
		t.Fatal("expected existing memory to still exist")
	}
	if mem.Abstract != "Combined profile" {
		t.Errorf("expected merged abstract 'Combined profile', got '%s'", mem.Abstract)
	}
}

func TestExtractMemoriesWithSkip(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-extract-skip",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `[{"category":"preferences","abstract":"User likes Go","overview":"Go pref","content":"Go content"}]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	// Manually set up a skip scenario by not having a similar memory
	// (no similar = CREATE, so this test just exercises the extraction path
	// and memory creation)
	messages := []Message{
		{Role: "user", Content: "I like Go"},
	}

	app.extractMemories(context.Background(), messages)

	// With no similar memories, it should CREATE
	if app.memory.Count() != 1 {
		t.Fatalf("expected 1 memory, got %d", app.memory.Count())
	}
}

func TestExtractMemoriesOutputMessage(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-output",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `[{"category":"profile","abstract":"User info","overview":"Overview","content":"Content"}]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "I am a tester"},
	}

	app.extractMemories(context.Background(), messages)

	output := app.output.(*bytes.Buffer).String()
	if !strings.Contains(output, "memories:") {
		t.Errorf("expected status output about memories, got: %s", output)
	}
	if !strings.Contains(output, "1 created") {
		t.Errorf("expected '1 created' in output, got: %s", output)
	}
}

func TestExtractMemoriesEmptyExtraction(t *testing.T) {
	// LLM returns empty array, no memories created, no output
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-empty",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "[]"},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}

	app.extractMemories(context.Background(), messages)

	if app.memory.Count() != 0 {
		t.Errorf("expected 0 memories for empty extraction, got %d", app.memory.Count())
	}

	output := app.output.(*bytes.Buffer).String()
	if strings.Contains(output, "memories:") {
		t.Errorf("should not print status when nothing was created, got: %s", output)
	}
}

// --- llmExtractMemories ---

func TestLLMExtractMemoriesSuccess(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-extract-success",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `[{"category":"profile","abstract":"User is Alice","overview":"Alice overview","content":"Alice content"},{"category":"entities","abstract":"Project Alpha","overview":"Alpha overview","content":"Alpha content"}]`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	candidates, err := app.llmExtractMemories(context.Background(), "User: My name is Alice and I work on Project Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Category != "profile" {
		t.Errorf("expected first candidate category 'profile', got '%s'", candidates[0].Category)
	}
	if candidates[0].Abstract != "User is Alice" {
		t.Errorf("expected first candidate abstract 'User is Alice', got '%s'", candidates[0].Abstract)
	}
	if candidates[1].Category != "entities" {
		t.Errorf("expected second candidate category 'entities', got '%s'", candidates[1].Category)
	}
}

func TestLLMExtractMemoriesLLMError(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "server error"}}`))
		},
		nil,
	)
	defer server.Close()

	_, err := app.llmExtractMemories(context.Background(), "Some text")
	if err == nil {
		t.Error("expected error when LLM fails")
	}
	if !strings.Contains(err.Error(), "LLM extraction call failed") {
		t.Errorf("expected error about LLM extraction failure, got: %v", err)
	}
}

func TestLLMExtractMemoriesInvalidJSON(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-bad-json",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "not valid json at all"},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	_, err := app.llmExtractMemories(context.Background(), "Some text")
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "parse extraction response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLLMExtractMemoriesEmptyArray(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-empty-arr",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "[]"},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	candidates, err := app.llmExtractMemories(context.Background(), "Some text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestLLMExtractMemoriesCodeFence(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-fence",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "```json\n[{\"category\":\"profile\",\"abstract\":\"Test\",\"overview\":\"Overview\",\"content\":\"Content\"}]\n```",
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	candidates, err := app.llmExtractMemories(context.Background(), "Some text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate after code fence stripping, got %d", len(candidates))
	}
}

// --- mergeMemory ---

func TestMergeMemorySuccess(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-merge-ok",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `{"abstract":"Merged abstract","overview":"Merged overview","content":"Merged content"}`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_merge_target",
		Category: CatProfile,
		Abstract: "Old abstract",
		Overview: "Old overview",
		Content:  "Old content",
		Vector:   []float64{0.1, 0.2, 0.3},
	}
	app.memory.Add(existing)

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New abstract",
		Overview: "New overview",
		Content:  "New content",
	}
	vec := []float64{0.4, 0.5, 0.6}

	result := app.mergeMemory(context.Background(), existing, candidate, vec)
	if result != 1 {
		t.Errorf("expected merge result 1 (success), got %d", result)
	}

	mem := app.memory.Get("mem_merge_target")
	if mem == nil {
		t.Fatal("expected memory to still exist after merge")
	}
	if mem.Abstract != "Merged abstract" {
		t.Errorf("expected merged abstract, got: %s", mem.Abstract)
	}
	if mem.Overview != "Merged overview" {
		t.Errorf("expected merged overview, got: %s", mem.Overview)
	}
	if mem.Content != "Merged content" {
		t.Errorf("expected merged content, got: %s", mem.Content)
	}
	// Vector should be updated to the new vec
	if len(mem.Vector) != 3 || mem.Vector[0] != 0.4 {
		t.Errorf("expected vector updated to new vec, got: %v", mem.Vector)
	}
}

func TestMergeMemoryLLMError(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "merge error"}}`))
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_merge_fail",
		Category: CatProfile,
		Abstract: "Old abstract",
		Overview: "Old overview",
		Content:  "Old content",
	}
	app.memory.Add(existing)

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New abstract",
		Overview: "New overview",
		Content:  "New content",
	}

	result := app.mergeMemory(context.Background(), existing, candidate, nil)
	if result != 0 {
		t.Errorf("expected merge result 0 (failure), got %d", result)
	}

	// Original memory should be unchanged
	mem := app.memory.Get("mem_merge_fail")
	if mem.Abstract != "Old abstract" {
		t.Errorf("memory should be unchanged on merge failure, got abstract: %s", mem.Abstract)
	}
}

func TestMergeMemoryNilVector(t *testing.T) {
	// When vec is nil, existing vector should not be modified
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-merge-novec",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `{"abstract":"Merged","overview":"Merged overview","content":"Merged content"}`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_merge_novec",
		Category: CatProfile,
		Abstract: "Old abstract",
		Overview: "Old overview",
		Content:  "Old content",
		Vector:   []float64{0.1, 0.2, 0.3},
	}
	app.memory.Add(existing)

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New",
		Overview: "New overview",
		Content:  "New content",
	}

	result := app.mergeMemory(context.Background(), existing, candidate, nil)
	if result != 1 {
		t.Errorf("expected success, got %d", result)
	}

	mem := app.memory.Get("mem_merge_novec")
	// Vector should remain the old one since we passed nil
	if len(mem.Vector) != 3 || mem.Vector[0] != 0.1 {
		t.Errorf("expected vector to remain unchanged when nil passed, got: %v", mem.Vector)
	}
}

// --- llmMergeMemories ---

func TestLLMMergeMemoriesSuccess(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-llm-merge",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `{"abstract":"Combined abstract","overview":"Combined overview","content":"Combined content details"}`,
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_for_merge",
		Category: CatProfile,
		Abstract: "Existing abstract",
		Overview: "Existing overview",
		Content:  "Existing content",
	}

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New abstract",
		Overview: "New overview",
		Content:  "New content",
	}

	merged, err := app.llmMergeMemories(context.Background(), existing, candidate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Abstract != "Combined abstract" {
		t.Errorf("expected merged abstract 'Combined abstract', got '%s'", merged.Abstract)
	}
	if merged.Overview != "Combined overview" {
		t.Errorf("expected merged overview 'Combined overview', got '%s'", merged.Overview)
	}
	if merged.Content != "Combined content details" {
		t.Errorf("expected merged content, got '%s'", merged.Content)
	}
}

func TestLLMMergeMemoriesLLMError(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "server error"}}`))
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_err",
		Category: CatProfile,
		Abstract: "Existing",
		Overview: "Overview",
		Content:  "Content",
	}
	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New",
		Overview: "New overview",
		Content:  "New content",
	}

	_, err := app.llmMergeMemories(context.Background(), existing, candidate)
	if err == nil {
		t.Error("expected error when LLM fails")
	}
	if !strings.Contains(err.Error(), "LLM merge call failed") {
		t.Errorf("expected error about LLM merge failure, got: %v", err)
	}
}

func TestLLMMergeMemoriesInvalidJSON(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-bad-merge",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": "this is not json"},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_bad",
		Category: CatProfile,
		Abstract: "Existing",
		Overview: "Overview",
		Content:  "Content",
	}
	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New",
		Overview: "New overview",
		Content:  "New content",
	}

	_, err := app.llmMergeMemories(context.Background(), existing, candidate)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse merge response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLLMMergeMemoriesCodeFence(t *testing.T) {
	app, server := newExtractorTestApp(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-fence-merge",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "```json\n{\"abstract\":\"Fenced\",\"overview\":\"Fenced overview\",\"content\":\"Fenced content\"}\n```",
						},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	existing := &Memory{
		ID:       "mem_fence",
		Category: CatProfile,
		Abstract: "Existing",
		Overview: "Overview",
		Content:  "Content",
	}
	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "New",
		Overview: "New overview",
		Content:  "New content",
	}

	merged, err := app.llmMergeMemories(context.Background(), existing, candidate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged.Abstract != "Fenced" {
		t.Errorf("expected 'Fenced' after code fence stripping, got '%s'", merged.Abstract)
	}
}
