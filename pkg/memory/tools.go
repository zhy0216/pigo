package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
)

// MemoryToolDeps holds the dependencies needed by memory tools,
// replacing the direct *App reference.
type MemoryToolDeps struct {
	Client       LLMClient
	Store        *MemoryStore
	Extractor    *MemoryExtractor
	Deduplicator *MemoryDeduplicator
}

// MemoryRecallTool retrieves relevant memories for a query.
type MemoryRecallTool struct {
	deps MemoryToolDeps
}

func NewMemoryRecallTool(deps MemoryToolDeps) *MemoryRecallTool {
	return &MemoryRecallTool{deps: deps}
}

func (t *MemoryRecallTool) Name() string { return "memory_recall" }

func (t *MemoryRecallTool) Description() string {
	return "Search and retrieve relevant memories. Returns L0 abstracts and L1 overviews of matching memories."
}

func (t *MemoryRecallTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query to find relevant memories",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Optional category filter: profile, preferences, entities, events, cases, patterns",
				"enum":        []string{"profile", "preferences", "entities", "events", "cases", "patterns"},
			},
			"top_k": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of memories to return (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryRecallTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	query, err := util.ExtractString(args, "query")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	category := ""
	if cat, ok := args["category"].(string); ok {
		category = cat
	}

	topK := 5
	if k, ok := args["top_k"].(float64); ok && k > 0 {
		topK = int(k)
	}

	var results []*Memory
	vec, err := t.deps.Client.Embed(ctx, query)
	if err == nil && len(vec) > 0 {
		results = t.deps.Store.SearchByVector(vec, topK, MemoryCategory(category))
	}

	if len(results) == 0 {
		results = t.deps.Store.SearchByKeyword(query, topK)
		if category != "" {
			var filtered []*Memory
			for _, m := range results {
				if string(m.Category) == category {
					filtered = append(filtered, m)
				}
			}
			results = filtered
		}
	}

	if len(results) == 0 {
		return types.NewToolResult("No relevant memories found.")
	}

	for _, m := range results {
		t.deps.Store.IncrementActive(m.ID)
	}
	t.deps.Store.Save()

	var buf strings.Builder
	fmt.Fprintf(&buf, "Found %d relevant memories:\n\n", len(results))
	for i, m := range results {
		fmt.Fprintf(&buf, "## Memory %d [%s] (id: %s)\n", i+1, m.Category, m.ID)
		fmt.Fprintf(&buf, "**Abstract**: %s\n", m.Abstract)
		if m.Overview != "" {
			fmt.Fprintf(&buf, "**Overview**: %s\n", m.Overview)
		}
		buf.WriteString("\n")
	}

	return &types.ToolResult{
		ForLLM:  buf.String(),
		ForUser: fmt.Sprintf("Found %d relevant memories", len(results)),
	}
}

// MemoryRememberTool explicitly saves a memory.
type MemoryRememberTool struct {
	deps MemoryToolDeps
}

func NewMemoryRememberTool(deps MemoryToolDeps) *MemoryRememberTool {
	return &MemoryRememberTool{deps: deps}
}

func (t *MemoryRememberTool) Name() string { return "memory_remember" }

func (t *MemoryRememberTool) Description() string {
	return "Save an important piece of information as a long-term memory. Use when the user asks you to remember something or when you identify important context."
}

func (t *MemoryRememberTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Memory category: profile (user identity), preferences (user preferences), entities (named things), events (things that happened), cases (problem+solution), patterns (reusable processes)",
				"enum":        []string{"profile", "preferences", "entities", "events", "cases", "patterns"},
			},
			"abstract": map[string]interface{}{
				"type":        "string",
				"description": "L0: One sentence summary of the memory",
			},
			"overview": map[string]interface{}{
				"type":        "string",
				"description": "L1: 2-4 sentence structured summary",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "L2: Full detailed content of the memory",
			},
		},
		"required": []string{"category", "abstract", "content"},
	}
}

func (t *MemoryRememberTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	category, err := util.ExtractString(args, "category")
	if err != nil {
		return types.ErrorResult(err.Error())
	}
	if !IsValidCategory(category) {
		return types.ErrorResult(fmt.Sprintf("invalid category: %s (must be one of: profile, preferences, entities, events, cases, patterns)", category))
	}

	abstract, err := util.ExtractString(args, "abstract")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	content, err := util.ExtractString(args, "content")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	overview := ""
	if o, ok := args["overview"].(string); ok {
		overview = o
	}
	if overview == "" {
		overview = abstract
	}

	vec, embErr := t.deps.Client.Embed(ctx, abstract+" "+overview)
	if embErr != nil {
		vec = nil
	}

	candidate := &CandidateMemory{
		Category: category,
		Abstract: abstract,
		Overview: overview,
		Content:  content,
	}
	result := t.deps.Deduplicator.DeduplicateMemory(ctx, candidate, vec)

	switch result.Decision {
	case DedupSkip:
		return types.NewToolResult(fmt.Sprintf("Memory already exists (skipped): %s", result.Reason))

	case DedupMerge:
		existing := t.deps.Store.Get(result.MergeTarget)
		if existing != nil {
			if t.deps.Extractor.MergeMemory(ctx, existing, candidate, vec) == 0 {
				return types.ErrorResult("failed to merge memory with existing")
			}
			t.deps.Store.Save()
			return types.NewToolResult(fmt.Sprintf("Memory merged with existing (id: %s): %s", existing.ID, result.Reason))
		}
		fallthrough

	case DedupCreate:
		mem := &Memory{
			Category: MemoryCategory(category),
			Abstract: abstract,
			Overview: overview,
			Content:  content,
			Vector:   vec,
		}
		t.deps.Store.Add(mem)
		t.deps.Store.Save()
		return types.NewToolResult(fmt.Sprintf("Memory saved (id: %s, category: %s): %s", mem.ID, category, abstract))

	default:
		return types.ErrorResult("unexpected dedup decision")
	}
}

// MemoryForgetTool deletes a memory by ID.
type MemoryForgetTool struct {
	deps MemoryToolDeps
}

func NewMemoryForgetTool(deps MemoryToolDeps) *MemoryForgetTool {
	return &MemoryForgetTool{deps: deps}
}

func (t *MemoryForgetTool) Name() string { return "memory_forget" }

func (t *MemoryForgetTool) Description() string {
	return "Delete a memory by its ID. Use when a memory is outdated or incorrect."
}

func (t *MemoryForgetTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The memory ID to delete (e.g., mem_abc123)",
			},
		},
		"required": []string{"id"},
	}
}

func (t *MemoryForgetTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	id, err := util.ExtractString(args, "id")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	mem := t.deps.Store.Get(id)
	if mem == nil {
		return types.ErrorResult(fmt.Sprintf("memory not found: %s", id))
	}

	abstract := mem.Abstract
	if err := t.deps.Store.Delete(id); err != nil {
		return types.ErrorResult(fmt.Sprintf("failed to delete memory: %v", err))
	}
	t.deps.Store.Save()

	return types.NewToolResult(fmt.Sprintf("Memory deleted (id: %s): %s", id, abstract))
}
