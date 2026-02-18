package main

import (
	"context"
	"fmt"
	"strings"
)

// MemoryRecallTool retrieves relevant memories for a query.
type MemoryRecallTool struct {
	app *App
}

func NewMemoryRecallTool(app *App) *MemoryRecallTool {
	return &MemoryRecallTool{app: app}
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

func (t *MemoryRecallTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	query, err := extractString(args, "query")
	if err != nil {
		return ErrorResult(err.Error())
	}

	category := ""
	if cat, ok := args["category"].(string); ok {
		category = cat
	}

	topK := 5
	if k, ok := args["top_k"].(float64); ok && k > 0 {
		topK = int(k)
	}

	// Try vector search first
	var results []*Memory
	vec, err := t.app.client.Embed(ctx, query)
	if err == nil && len(vec) > 0 {
		results = t.app.memory.SearchByVector(vec, topK, MemoryCategory(category))
	}

	// Fallback to keyword search if vector search returned nothing
	if len(results) == 0 {
		results = t.app.memory.SearchByKeyword(query, topK)
		// Filter by category if specified
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
		return NewToolResult("No relevant memories found.")
	}

	// Increment active count for recalled memories
	for _, m := range results {
		t.app.memory.IncrementActive(m.ID)
	}
	t.app.memory.Save()

	// Format results with L0 and L1 only (L2 can be requested separately)
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

	return &ToolResult{
		ForLLM:  buf.String(),
		ForUser: fmt.Sprintf("Found %d relevant memories", len(results)),
	}
}

// MemoryRememberTool explicitly saves a memory.
type MemoryRememberTool struct {
	app *App
}

func NewMemoryRememberTool(app *App) *MemoryRememberTool {
	return &MemoryRememberTool{app: app}
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

func (t *MemoryRememberTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	category, err := extractString(args, "category")
	if err != nil {
		return ErrorResult(err.Error())
	}
	if !IsValidCategory(category) {
		return ErrorResult(fmt.Sprintf("invalid category: %s (must be one of: profile, preferences, entities, events, cases, patterns)", category))
	}

	abstract, err := extractString(args, "abstract")
	if err != nil {
		return ErrorResult(err.Error())
	}

	content, err := extractString(args, "content")
	if err != nil {
		return ErrorResult(err.Error())
	}

	overview := ""
	if o, ok := args["overview"].(string); ok {
		overview = o
	}
	if overview == "" {
		overview = abstract // fallback L1 = L0
	}

	// Generate embedding
	vec, embErr := t.app.client.Embed(ctx, abstract+" "+overview)
	if embErr != nil {
		vec = nil // store without vector
	}

	// Dedup check
	candidate := &CandidateMemory{
		Category: category,
		Abstract: abstract,
		Overview: overview,
		Content:  content,
	}
	result := t.app.deduplicateMemory(ctx, candidate, vec)

	switch result.Decision {
	case DedupSkip:
		return NewToolResult(fmt.Sprintf("Memory already exists (skipped): %s", result.Reason))

	case DedupMerge:
		existing := t.app.memory.Get(result.MergeTarget)
		if existing != nil {
			if t.app.mergeMemory(ctx, existing, candidate, vec) == 0 {
				return ErrorResult("failed to merge memory with existing")
			}
			t.app.memory.Save()
			return NewToolResult(fmt.Sprintf("Memory merged with existing (id: %s): %s", existing.ID, result.Reason))
		}
		// Fall through to create if merge target not found
		fallthrough

	case DedupCreate:
		mem := &Memory{
			Category: MemoryCategory(category),
			Abstract: abstract,
			Overview: overview,
			Content:  content,
			Vector:   vec,
		}
		t.app.memory.Add(mem)
		t.app.memory.Save()
		return NewToolResult(fmt.Sprintf("Memory saved (id: %s, category: %s): %s", mem.ID, category, abstract))

	default:
		return ErrorResult("unexpected dedup decision")
	}
}

// MemoryForgetTool deletes a memory by ID.
type MemoryForgetTool struct {
	app *App
}

func NewMemoryForgetTool(app *App) *MemoryForgetTool {
	return &MemoryForgetTool{app: app}
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

func (t *MemoryForgetTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	id, err := extractString(args, "id")
	if err != nil {
		return ErrorResult(err.Error())
	}

	mem := t.app.memory.Get(id)
	if mem == nil {
		return ErrorResult(fmt.Sprintf("memory not found: %s", id))
	}

	abstract := mem.Abstract
	if err := t.app.memory.Delete(id); err != nil {
		return ErrorResult(fmt.Sprintf("failed to delete memory: %v", err))
	}
	t.app.memory.Save()

	return NewToolResult(fmt.Sprintf("Memory deleted (id: %s): %s", id, abstract))
}
