package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DedupDecision represents the deduplication decision for a memory candidate.
type DedupDecision string

const (
	DedupCreate DedupDecision = "CREATE" // New unique memory
	DedupMerge  DedupDecision = "MERGE"  // Should merge with existing
	DedupSkip   DedupDecision = "SKIP"   // Duplicate, discard
)

// DedupResult holds the deduplication decision and context.
type DedupResult struct {
	Decision    DedupDecision
	Reason      string
	MergeTarget string // ID of memory to merge with (if MERGE)
}

// Categories that always merge when similar (OpenViking-compatible)
var alwaysMergeCategories = map[MemoryCategory]bool{
	CatProfile: true,
}

// Categories that never merge (always create new)
var neverMergeCategories = map[MemoryCategory]bool{
	CatEvents: true,
	CatCases:  true,
}

// deduplicateMemory checks if a candidate memory is a duplicate of existing memories.
// Uses vector similarity pre-filtering + LLM decision making.
func (a *App) deduplicateMemory(ctx context.Context, candidate *CandidateMemory, vec []float64) DedupResult {
	category := MemoryCategory(candidate.Category)

	// Step 1: Vector pre-filter — find similar memories
	similar := a.memory.FindSimilar(vec, similarityThreshold, category)
	if len(similar) == 0 {
		return DedupResult{Decision: DedupCreate, Reason: "no similar memories found"}
	}

	// Step 2: Category-based rules
	if neverMergeCategories[category] {
		// Events and cases: check for exact duplicates via LLM, but never merge
		result := a.llmDedupDecision(ctx, candidate, similar)
		if result.Decision == DedupMerge {
			// Override: events/cases create new instead of merging
			return DedupResult{Decision: DedupCreate, Reason: "events/cases always create new"}
		}
		return result
	}

	if alwaysMergeCategories[category] && len(similar) > 0 {
		// Profile: always merge with most similar
		return DedupResult{
			Decision:    DedupMerge,
			Reason:      "profile category always merges",
			MergeTarget: similar[0].Memory.ID,
		}
	}

	// Step 3: LLM decision for other categories
	return a.llmDedupDecision(ctx, candidate, similar)
}

const dedupDecisionPrompt = `You are a memory deduplication system. Given a new memory candidate and a list of existing similar memories, decide what to do.

## Decisions

- **SKIP**: The candidate is a duplicate of an existing memory (no new information)
- **CREATE**: The candidate contains genuinely new information not in existing memories
- **MERGE**: The candidate overlaps with an existing memory and should be combined

## Candidate Memory
Category: %s
Abstract: %s
Overview: %s
Content: %s

## Existing Similar Memories
%s

Output a JSON object with keys: decision (SKIP/CREATE/MERGE), reason (brief explanation), merge_target (ID of memory to merge with, only if MERGE).
Output ONLY the JSON object, no other text.`

// llmDedupDecision calls the LLM to decide CREATE/MERGE/SKIP.
func (a *App) llmDedupDecision(ctx context.Context, candidate *CandidateMemory, similar []SimilarMemory) DedupResult {
	// Format existing memories
	var existingBuf strings.Builder
	for i, sm := range similar {
		if i >= 5 { // limit to top 5
			break
		}
		fmt.Fprintf(&existingBuf, "### Memory %d (ID: %s, similarity: %.2f)\n", i+1, sm.Memory.ID, sm.Score)
		fmt.Fprintf(&existingBuf, "Abstract: %s\n", sm.Memory.Abstract)
		fmt.Fprintf(&existingBuf, "Overview: %s\n\n", sm.Memory.Overview)
	}

	prompt := fmt.Sprintf(dedupDecisionPrompt,
		candidate.Category,
		candidate.Abstract,
		candidate.Overview,
		candidate.Content,
		existingBuf.String(),
	)

	messages := []Message{
		{Role: "system", Content: "You are a memory deduplication system. Output only JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := a.client.Chat(ctx, messages, nil)
	if err != nil {
		// On error, default to CREATE
		return DedupResult{Decision: DedupCreate, Reason: "dedup LLM call failed, defaulting to create"}
	}

	content := strings.TrimSpace(resp.Content)
	content = stripCodeFence(content)

	var result struct {
		Decision    string `json:"decision"`
		Reason      string `json:"reason"`
		MergeTarget string `json:"merge_target"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return DedupResult{Decision: DedupCreate, Reason: "failed to parse dedup response, defaulting to create"}
	}

	decision := DedupDecision(strings.ToUpper(result.Decision))
	switch decision {
	case DedupCreate, DedupMerge, DedupSkip:
		return DedupResult{
			Decision:    decision,
			Reason:      result.Reason,
			MergeTarget: result.MergeTarget,
		}
	default:
		return DedupResult{Decision: DedupCreate, Reason: "unknown decision, defaulting to create"}
	}
}
