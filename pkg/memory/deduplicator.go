package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/user/pigo/pkg/types"
	"github.com/user/pigo/pkg/util"
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

// MemoryDeduplicator uses LLM and vector similarity to deduplicate memories.
type MemoryDeduplicator struct {
	Client LLMClient
	Store  *MemoryStore
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

// DeduplicateMemory checks if a candidate memory is a duplicate of existing memories.
// Uses vector similarity pre-filtering + LLM decision making.
func (d *MemoryDeduplicator) DeduplicateMemory(ctx context.Context, candidate *CandidateMemory, vec []float64) DedupResult {
	category := MemoryCategory(candidate.Category)

	similar := d.Store.FindSimilar(vec, SimilarityThreshold, category)
	if len(similar) == 0 {
		return DedupResult{Decision: DedupCreate, Reason: "no similar memories found"}
	}

	if neverMergeCategories[category] {
		result := d.llmDedupDecision(ctx, candidate, similar)
		if result.Decision == DedupMerge {
			return DedupResult{Decision: DedupCreate, Reason: "events/cases always create new"}
		}
		return result
	}

	if alwaysMergeCategories[category] && len(similar) > 0 {
		return DedupResult{
			Decision:    DedupMerge,
			Reason:      "profile category always merges",
			MergeTarget: similar[0].Memory.ID,
		}
	}

	return d.llmDedupDecision(ctx, candidate, similar)
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
func (d *MemoryDeduplicator) llmDedupDecision(ctx context.Context, candidate *CandidateMemory, similar []SimilarMemory) DedupResult {
	var existingBuf strings.Builder
	for i, sm := range similar {
		if i >= 5 {
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

	messages := []types.Message{
		{Role: "system", Content: "You are a memory deduplication system. Output only JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := d.Client.Chat(ctx, messages, nil)
	if err != nil {
		return DedupResult{Decision: DedupCreate, Reason: "dedup LLM call failed, defaulting to create"}
	}

	content := strings.TrimSpace(resp.Content)
	content = util.StripCodeFence(content)

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
