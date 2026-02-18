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

func TestAlwaysMergeCategories(t *testing.T) {
	if !alwaysMergeCategories[CatProfile] {
		t.Error("profile should be in alwaysMergeCategories")
	}
	if alwaysMergeCategories[CatEvents] {
		t.Error("events should not be in alwaysMergeCategories")
	}
}

func TestNeverMergeCategories(t *testing.T) {
	if !neverMergeCategories[CatEvents] {
		t.Error("events should be in neverMergeCategories")
	}
	if !neverMergeCategories[CatCases] {
		t.Error("cases should be in neverMergeCategories")
	}
	if neverMergeCategories[CatProfile] {
		t.Error("profile should not be in neverMergeCategories")
	}
}

func TestDedupDecisionConstants(t *testing.T) {
	if DedupCreate != "CREATE" {
		t.Errorf("expected CREATE, got %s", DedupCreate)
	}
	if DedupMerge != "MERGE" {
		t.Errorf("expected MERGE, got %s", DedupMerge)
	}
	if DedupSkip != "SKIP" {
		t.Errorf("expected SKIP, got %s", DedupSkip)
	}
}

// newDedupTestApp creates an App with a mock server for dedup testing.
func newDedupTestApp(t *testing.T, chatResp func(w http.ResponseWriter, r *http.Request)) (*App, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "embeddings") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
				},
			})
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
						"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"CREATE","reason":"default"}`},
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

// --- deduplicateMemory ---

func TestDeduplicateMemoryNoSimilar(t *testing.T) {
	app, server := newDedupTestApp(t, nil)
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "User likes Go",
		Overview: "Go overview",
		Content:  "Go content",
	}
	vec := []float64{0.1, 0.2, 0.3}

	result := app.deduplicateMemory(context.Background(), candidate, vec)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE when no similar memories, got %s", result.Decision)
	}
	if !strings.Contains(result.Reason, "no similar") {
		t.Errorf("expected reason about no similar memories, got: %s", result.Reason)
	}
}

func TestDeduplicateMemoryProfileAlwaysMerges(t *testing.T) {
	app, server := newDedupTestApp(t, nil)
	defer server.Close()

	// Add existing memory with matching vector
	app.memory.Add(&Memory{
		ID:       "mem_profile_existing",
		Category: CatProfile,
		Abstract: "User is a developer",
		Overview: "Developer overview",
		Content:  "Developer content",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	candidate := &CandidateMemory{
		Category: "profile",
		Abstract: "User is a Go developer",
		Overview: "Go developer overview",
		Content:  "Go developer content",
	}
	vec := []float64{0.1, 0.2, 0.3} // identical vector -> similarity = 1.0

	result := app.deduplicateMemory(context.Background(), candidate, vec)
	if result.Decision != DedupMerge {
		t.Errorf("expected MERGE for profile category with similar memory, got %s", result.Decision)
	}
	if result.MergeTarget != "mem_profile_existing" {
		t.Errorf("expected merge target 'mem_profile_existing', got '%s'", result.MergeTarget)
	}
}

func TestDeduplicateMemoryEventsNeverMerge(t *testing.T) {
	// Events category: even if LLM says MERGE, it should be overridden to CREATE
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-events",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"MERGE","reason":"overlapping","merge_target":"mem_event1"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	app.memory.Add(&Memory{
		ID:       "mem_event1",
		Category: CatEvents,
		Abstract: "Bug was fixed",
		Content:  "Bug details",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	candidate := &CandidateMemory{
		Category: "events",
		Abstract: "Another bug fix",
		Content:  "Another bug",
	}
	vec := []float64{0.1, 0.2, 0.3}

	result := app.deduplicateMemory(context.Background(), candidate, vec)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE for events (never merge), got %s", result.Decision)
	}
}

func TestDeduplicateMemoryEventsSkipFromLLM(t *testing.T) {
	// Events category: if LLM says SKIP, that should be respected
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-events-skip",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"SKIP","reason":"exact duplicate"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	app.memory.Add(&Memory{
		ID:       "mem_event2",
		Category: CatEvents,
		Abstract: "Deploy happened",
		Content:  "Deploy details",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	candidate := &CandidateMemory{
		Category: "events",
		Abstract: "Deploy happened",
		Content:  "Deploy details",
	}
	vec := []float64{0.1, 0.2, 0.3}

	result := app.deduplicateMemory(context.Background(), candidate, vec)
	if result.Decision != DedupSkip {
		t.Errorf("expected SKIP for events with exact duplicate, got %s", result.Decision)
	}
}

func TestDeduplicateMemoryLLMDecisionCreate(t *testing.T) {
	// Non-special category with similar memory, LLM returns CREATE
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-create",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"CREATE","reason":"new info"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	app.memory.Add(&Memory{
		ID:       "mem_pref1",
		Category: CatPreferences,
		Abstract: "User likes tabs",
		Content:  "Tab preference",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	candidate := &CandidateMemory{
		Category: "preferences",
		Abstract: "User likes spaces",
		Content:  "Space preference",
	}
	vec := []float64{0.1, 0.2, 0.3}

	result := app.deduplicateMemory(context.Background(), candidate, vec)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE, got %s", result.Decision)
	}
}

// --- llmDedupDecision ---

func TestLLMDedupDecisionCreate(t *testing.T) {
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-llm-create",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"CREATE","reason":"genuinely new"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Project Alpha",
		Content:  "Project details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_sim1", Abstract: "Project Beta", Overview: "Beta overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE, got %s", result.Decision)
	}
	if result.Reason != "genuinely new" {
		t.Errorf("expected reason 'genuinely new', got '%s'", result.Reason)
	}
}

func TestLLMDedupDecisionSkip(t *testing.T) {
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-llm-skip",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"SKIP","reason":"duplicate"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Project Alpha",
		Content:  "Project details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_sim1", Abstract: "Project Alpha", Overview: "Alpha overview"}, Score: 0.95},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupSkip {
		t.Errorf("expected SKIP, got %s", result.Decision)
	}
}

func TestLLMDedupDecisionMerge(t *testing.T) {
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-llm-merge",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"MERGE","reason":"overlapping","merge_target":"mem_target"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Updated Alpha",
		Content:  "Updated details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_target", Abstract: "Project Alpha", Overview: "Alpha overview"}, Score: 0.85},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupMerge {
		t.Errorf("expected MERGE, got %s", result.Decision)
	}
	if result.MergeTarget != "mem_target" {
		t.Errorf("expected merge_target 'mem_target', got '%s'", result.MergeTarget)
	}
}

func TestLLMDedupDecisionLLMError(t *testing.T) {
	// When the LLM call fails, should default to CREATE
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "server error"}}`))
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Something",
		Content:  "Details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_x", Abstract: "Existing", Overview: "Overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE on LLM error, got %s", result.Decision)
	}
	if !strings.Contains(result.Reason, "failed") {
		t.Errorf("expected reason mentioning failure, got: %s", result.Reason)
	}
}

func TestLLMDedupDecisionInvalidJSON(t *testing.T) {
	// When LLM returns invalid JSON, should default to CREATE
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-invalid",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": "this is not json"},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Something",
		Content:  "Details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_x", Abstract: "Existing", Overview: "Overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE on invalid JSON, got %s", result.Decision)
	}
	if !strings.Contains(result.Reason, "parse") {
		t.Errorf("expected reason mentioning parse failure, got: %s", result.Reason)
	}
}

func TestLLMDedupDecisionUnknownDecision(t *testing.T) {
	// When LLM returns an unknown decision type, should default to CREATE
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-unknown",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"UNKNOWN","reason":"weird"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Something",
		Content:  "Details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_x", Abstract: "Existing", Overview: "Overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupCreate {
		t.Errorf("expected CREATE on unknown decision, got %s", result.Decision)
	}
	if !strings.Contains(result.Reason, "unknown") {
		t.Errorf("expected reason mentioning unknown, got: %s", result.Reason)
	}
}

func TestLLMDedupDecisionCodeFence(t *testing.T) {
	// When LLM wraps JSON in code fences, it should still parse correctly
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-fence",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": "```json\n{\"decision\":\"SKIP\",\"reason\":\"dup\"}\n```"},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Something",
		Content:  "Details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_x", Abstract: "Existing", Overview: "Overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupSkip {
		t.Errorf("expected SKIP after stripping code fence, got %s", result.Decision)
	}
}

func TestLLMDedupDecisionCaseInsensitive(t *testing.T) {
	// Decision should be case-insensitive (lowercased "skip" should work)
	app, server := newDedupTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "chatcmpl-lower",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"skip","reason":"duplicate"}`},
					"finish_reason": "stop",
				},
			},
		})
	})
	defer server.Close()

	candidate := &CandidateMemory{
		Category: "entities",
		Abstract: "Something",
		Content:  "Details",
	}
	similar := []SimilarMemory{
		{Memory: &Memory{ID: "mem_x", Abstract: "Existing", Overview: "Overview"}, Score: 0.8},
	}

	result := app.llmDedupDecision(context.Background(), candidate, similar)
	if result.Decision != DedupSkip {
		t.Errorf("expected SKIP (case insensitive), got %s", result.Decision)
	}
}
