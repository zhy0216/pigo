package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user/pigo/pkg/llm"
)

// newMemToolTest creates MemoryToolDeps with a mock server.
func newMemToolTest(t *testing.T, chatResp func(w http.ResponseWriter, r *http.Request), embedResp func(w http.ResponseWriter, r *http.Request)) (MemoryToolDeps, *MemoryStore, *httptest.Server) {
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
						{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
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
						"message":       map[string]interface{}{"role": "assistant", "content": "ok"},
						"finish_reason": "stop",
					},
				},
			})
		}
	}))

	client := llm.NewClient("test-key", server.URL, "gpt-4", "chat", "")
	store := NewMemoryStore()
	dedup := &MemoryDeduplicator{Client: client, Store: store}
	extractor := &MemoryExtractor{Client: client, Store: store, Deduplicator: dedup}

	deps := MemoryToolDeps{
		Client:       client,
		Store:        store,
		Extractor:    extractor,
		Deduplicator: dedup,
	}
	return deps, store, server
}

func TestMemoryRecallToolMetadata(t *testing.T) {
	tool := NewMemoryRecallTool(MemoryToolDeps{})
	if tool.Name() != "memory_recall" {
		t.Errorf("expected 'memory_recall', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["query"]; !ok {
		t.Error("should have 'query' parameter")
	}
	if _, ok := props["category"]; !ok {
		t.Error("should have 'category' parameter")
	}
	if _, ok := props["top_k"]; !ok {
		t.Error("should have 'top_k' parameter")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("should have required fields")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Error("'query' should be the only required field")
	}
}

func TestMemoryRememberToolMetadata(t *testing.T) {
	tool := NewMemoryRememberTool(MemoryToolDeps{})
	if tool.Name() != "memory_remember" {
		t.Errorf("expected 'memory_remember', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["category"]; !ok {
		t.Error("should have 'category' parameter")
	}
	if _, ok := props["abstract"]; !ok {
		t.Error("should have 'abstract' parameter")
	}
	if _, ok := props["content"]; !ok {
		t.Error("should have 'content' parameter")
	}
}

func TestMemoryForgetToolMetadata(t *testing.T) {
	tool := NewMemoryForgetTool(MemoryToolDeps{})
	if tool.Name() != "memory_forget" {
		t.Errorf("expected 'memory_forget', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["id"]; !ok {
		t.Error("should have 'id' parameter")
	}
}

// --- MemoryRecallTool.Execute ---

func TestMemoryRecallExecuteMissingQuery(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error when query is missing")
	}
	if !strings.Contains(result.ForLLM, "query") {
		t.Errorf("error should mention 'query', got: %s", result.ForLLM)
	}
}

func TestMemoryRecallExecuteEmptyStore(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query": "something",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "No relevant memories found") {
		t.Errorf("expected 'No relevant memories found', got: %s", result.ForLLM)
	}
}

func TestMemoryRecallExecuteKeywordSearch(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	store.Add(&Memory{
		ID:       "mem_test1",
		Category: CatProfile,
		Abstract: "User likes Go programming",
		Overview: "The user prefers Go for backend work",
		Content:  "Full details about Go preference",
	})

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query": "Go programming",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Found 1 relevant memories") {
		t.Errorf("expected 1 memory found, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "mem_test1") {
		t.Errorf("expected memory ID in output, got: %s", result.ForLLM)
	}
}

func TestMemoryRecallExecuteWithCategoryFilter(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	store.Add(&Memory{ID: "mem_profile1", Category: CatProfile, Abstract: "User likes Go", Content: "Go details"})
	store.Add(&Memory{ID: "mem_event1", Category: CatEvents, Abstract: "User started Go project", Content: "Go project details"})

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query":    "Go",
		"category": "profile",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "mem_profile1") {
		t.Errorf("expected profile memory, got: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "mem_event1") {
		t.Errorf("should not contain events memory when filtered to profile, got: %s", result.ForLLM)
	}
}

func TestMemoryRecallExecuteWithTopK(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	store.Add(&Memory{
		ID:       "mem_a",
		Category: CatProfile,
		Abstract: "test memory alpha",
		Content:  "test content alpha",
	})
	store.Add(&Memory{
		ID:       "mem_b",
		Category: CatProfile,
		Abstract: "test memory beta",
		Content:  "test content beta",
	})

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query": "test",
		"top_k": float64(1),
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Found 1 relevant memories") {
		t.Errorf("expected exactly 1 memory with top_k=1, got: %s", result.ForLLM)
	}
}

func TestMemoryRecallExecuteVectorSearch(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	store.Add(&Memory{
		ID:       "mem_vec1",
		Category: CatProfile,
		Abstract: "vector test memory",
		Overview: "vector overview",
		Content:  "vector content",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	tool := NewMemoryRecallTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"query": "something",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "mem_vec1") {
		t.Errorf("expected vector search to find memory, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "vector overview") {
		t.Errorf("expected overview in output, got: %s", result.ForLLM)
	}
}

func TestMemoryRecallIncrementsActiveCount(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	store.Add(&Memory{ID: "mem_active", Category: CatProfile, Abstract: "count test", Content: "count content", ActiveCount: 0})

	tool := NewMemoryRecallTool(deps)
	tool.Execute(context.Background(), map[string]interface{}{"query": "count"})

	mem := store.Get("mem_active")
	if mem.ActiveCount != 1 {
		t.Errorf("expected ActiveCount=1 after recall, got %d", mem.ActiveCount)
	}
}

// --- MemoryRememberTool.Execute ---

func TestMemoryRememberExecuteMissingCategory(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"abstract": "some abstract",
		"content":  "some content",
	})
	if !result.IsError {
		t.Error("expected error when category is missing")
	}
}

func TestMemoryRememberExecuteMissingAbstract(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "profile",
		"content":  "some content",
	})
	if !result.IsError {
		t.Error("expected error when abstract is missing")
	}
}

func TestMemoryRememberExecuteMissingContent(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "profile",
		"abstract": "some abstract",
	})
	if !result.IsError {
		t.Error("expected error when content is missing")
	}
}

func TestMemoryRememberExecuteInvalidCategory(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "invalid_cat",
		"abstract": "some abstract",
		"content":  "some content",
	})
	if !result.IsError {
		t.Error("expected error for invalid category")
	}
	if !strings.Contains(result.ForLLM, "invalid category") {
		t.Errorf("expected 'invalid category' in error, got: %s", result.ForLLM)
	}
}

func TestMemoryRememberExecuteCreateSuccess(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "profile",
		"abstract": "User prefers dark mode",
		"overview": "The user always uses dark mode in editors",
		"content":  "Full dark mode preference details",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Memory saved") {
		t.Errorf("expected 'Memory saved', got: %s", result.ForLLM)
	}

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 memory in store, got %d", len(all))
	}
	if all[0].Abstract != "User prefers dark mode" {
		t.Errorf("expected abstract 'User prefers dark mode', got '%s'", all[0].Abstract)
	}
	if all[0].Overview != "The user always uses dark mode in editors" {
		t.Errorf("expected overview set, got '%s'", all[0].Overview)
	}
}

func TestMemoryRememberExecuteOverviewFallback(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "entities",
		"abstract": "Project Alpha is a Go service",
		"content":  "Full details about Project Alpha",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(all))
	}
	if all[0].Overview != all[0].Abstract {
		t.Errorf("expected overview to fall back to abstract, got overview=%q abstract=%q", all[0].Overview, all[0].Abstract)
	}
}

func TestMemoryRememberExecuteDedupSkip(t *testing.T) {
	deps, store, server := newMemToolTest(t,
		func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "chatcmpl-skip",
				"object": "chat.completion",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       map[string]interface{}{"role": "assistant", "content": `{"decision":"SKIP","reason":"duplicate info"}`},
						"finish_reason": "stop",
					},
				},
			})
		},
		nil,
	)
	defer server.Close()

	store.Add(&Memory{
		ID:       "mem_existing",
		Category: CatPreferences,
		Abstract: "User prefers dark mode",
		Overview: "Dark mode preference",
		Content:  "User always uses dark mode",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "preferences",
		"abstract": "User prefers dark mode",
		"content":  "User always uses dark mode",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "skipped") {
		t.Errorf("expected 'skipped' in result, got: %s", result.ForLLM)
	}
	if store.Count() != 1 {
		t.Errorf("expected 1 memory after skip, got %d", store.Count())
	}
}

func TestMemoryRememberExecuteDedupMerge(t *testing.T) {
	callCount := 0
	deps, store, server := newMemToolTest(t,
		func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     "chatcmpl-merge-dedup",
					"object": "chat.completion",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": `{"decision":"MERGE","reason":"overlapping info","merge_target":"mem_existing"}`,
							},
							"finish_reason": "stop",
						},
					},
				})
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     "chatcmpl-merge-content",
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
			}
		},
		nil,
	)
	defer server.Close()

	store.Add(&Memory{
		ID:       "mem_existing",
		Category: CatPreferences,
		Abstract: "User likes vim",
		Overview: "Vim preference",
		Content:  "Vim details",
		Vector:   []float64{0.1, 0.2, 0.3},
	})

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "preferences",
		"abstract": "User likes vim keybindings",
		"content":  "More vim details",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "merged") {
		t.Errorf("expected 'merged' in result, got: %s", result.ForLLM)
	}

	mem := store.Get("mem_existing")
	if mem == nil {
		t.Fatal("expected existing memory to still exist after merge")
	}
	if mem.Abstract != "Merged abstract" {
		t.Errorf("expected merged abstract, got: %s", mem.Abstract)
	}
}

func TestMemoryRememberExecuteEmbedError(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "embed error"}}`))
	})
	defer server.Close()

	tool := NewMemoryRememberTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"category": "profile",
		"abstract": "Test without vector",
		"content":  "Content without vector",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Memory saved") {
		t.Errorf("expected memory to be saved even without vector, got: %s", result.ForLLM)
	}

	all := store.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(all))
	}
	if all[0].Vector != nil {
		t.Error("expected nil vector when embed fails")
	}
}

// --- MemoryForgetTool.Execute ---

func TestMemoryForgetExecuteMissingID(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryForgetTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error when id is missing")
	}
	if !strings.Contains(result.ForLLM, "id") {
		t.Errorf("error should mention 'id', got: %s", result.ForLLM)
	}
}

func TestMemoryForgetExecuteNotFound(t *testing.T) {
	deps, _, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	tool := NewMemoryForgetTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"id": "mem_nonexistent",
	})
	if !result.IsError {
		t.Error("expected error when memory not found")
	}
	if !strings.Contains(result.ForLLM, "not found") {
		t.Errorf("error should mention 'not found', got: %s", result.ForLLM)
	}
}

func TestMemoryForgetExecuteSuccess(t *testing.T) {
	deps, store, server := newMemToolTest(t, nil, nil)
	defer server.Close()

	store.Add(&Memory{ID: "mem_to_delete", Category: CatEvents, Abstract: "Something happened", Content: "Event details"})

	tool := NewMemoryForgetTool(deps)
	result := tool.Execute(context.Background(), map[string]interface{}{
		"id": "mem_to_delete",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "deleted") {
		t.Errorf("expected 'deleted' in result, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "mem_to_delete") {
		t.Errorf("expected memory ID in result, got: %s", result.ForLLM)
	}

	if store.Get("mem_to_delete") != nil {
		t.Error("memory should be deleted from store")
	}
}
