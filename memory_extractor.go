package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CandidateMemory represents a memory candidate extracted by the LLM.
type CandidateMemory struct {
	Category string `json:"category"`
	Abstract string `json:"abstract"` // L0
	Overview string `json:"overview"` // L1
	Content  string `json:"content"`  // L2
}

const memoryExtractionPrompt = `You are a memory extraction system. Analyze the conversation below and extract important information worth remembering for future sessions.

## Categories

Classify each memory into exactly one category:

| Category | Owner | Decision Question | Examples |
|----------|-------|-------------------|----------|
| profile | User | "Who is the user?" | Name, role, background, skills, location |
| preferences | User | "What does the user prefer?" | Coding style, tool choices, communication style |
| entities | User | "What named things are involved?" | Projects, repos, APIs, teams, products |
| events | User | "What happened?" | Bug fixed, feature shipped, decision made |
| cases | Agent | "What problem was solved and how?" | Debug approach, architecture decision |
| patterns | Agent | "What reusable process was used?" | Workflow patterns, recurring solutions |

## Output Format

For each memory, provide three levels of detail:
- abstract (L0): One sentence index entry
- overview (L1): 2-4 sentence structured summary
- content (L2): Full detailed narrative (can be multiple paragraphs)

## Rules

1. Only extract information worth remembering across sessions
2. Skip trivial or transient information (e.g., "user said hello")
3. Match the user's language in your output
4. If no memories are worth extracting, return an empty array
5. Prefer fewer, higher-quality memories over many low-quality ones

Output a JSON array of objects with keys: category, abstract, overview, content.
Output ONLY the JSON array, no other text.`

// extractMemories extracts memories from discarded messages using LLM.
// Called during compaction before messages are lost.
func (a *App) extractMemories(ctx context.Context, discarded []Message) {
	if a.memory == nil || len(discarded) == 0 {
		return
	}

	// Format messages for extraction
	formatted := formatMessagesForExtraction(discarded)
	if formatted == "" {
		return
	}

	// Call LLM for extraction
	candidates, err := a.llmExtractMemories(ctx, formatted)
	if err != nil {
		fmt.Fprintf(a.output, "%s[memory extraction failed: %v]%s\n", colorYellow, err, colorReset)
		return
	}

	if len(candidates) == 0 {
		return
	}

	// Process each candidate through dedup pipeline
	created, merged, skipped := 0, 0, 0
	for _, candidate := range candidates {
		if !IsValidCategory(candidate.Category) {
			continue
		}

		// Generate embedding
		vec, err := a.client.Embed(ctx, candidate.Abstract+" "+candidate.Overview)
		if err != nil {
			// Store without vector; keyword search still works
			vec = nil
		}

		// Dedup check
		result := a.deduplicateMemory(ctx, &candidate, vec)

		switch result.Decision {
		case DedupCreate:
			mem := &Memory{
				Category: MemoryCategory(candidate.Category),
				Abstract: candidate.Abstract,
				Overview: candidate.Overview,
				Content:  candidate.Content,
				Vector:   vec,
			}
			a.memory.Add(mem)
			created++

		case DedupMerge:
			existing := a.memory.Get(result.MergeTarget)
			if existing != nil {
				merged += a.mergeMemory(ctx, existing, &candidate, vec)
			}

		case DedupSkip:
			skipped++
		}
	}

	if created+merged > 0 {
		a.memory.Save()
		fmt.Fprintf(a.output, "%s[memories: %d created, %d merged, %d skipped]%s\n",
			colorYellow, created, merged, skipped, colorReset)
	}
}

// llmExtractMemories calls the LLM to extract candidate memories from conversation text.
func (a *App) llmExtractMemories(ctx context.Context, conversationText string) ([]CandidateMemory, error) {
	messages := []Message{
		{Role: "system", Content: memoryExtractionPrompt},
		{Role: "user", Content: conversationText},
	}

	resp, err := a.client.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction call failed: %w", err)
	}

	// Parse JSON response
	content := strings.TrimSpace(resp.Content)
	// Strip markdown code fence if present
	content = stripCodeFence(content)

	var candidates []CandidateMemory
	if err := json.Unmarshal([]byte(content), &candidates); err != nil {
		return nil, fmt.Errorf("failed to parse extraction response: %w", err)
	}

	return candidates, nil
}

// mergeMemory merges a candidate into an existing memory using LLM.
// Returns 1 if merge succeeded, 0 otherwise.
func (a *App) mergeMemory(ctx context.Context, existing *Memory, candidate *CandidateMemory, vec []float64) int {
	merged, err := a.llmMergeMemories(ctx, existing, candidate)
	if err != nil {
		return 0
	}

	existing.Abstract = merged.Abstract
	existing.Overview = merged.Overview
	existing.Content = merged.Content
	if vec != nil {
		existing.Vector = vec
	}
	a.memory.Update(existing)
	return 1
}

const memoryMergePrompt = `You are merging two pieces of memory into one coherent entry.

## Rules
1. Remove duplicate information
2. Keep the most up-to-date details
3. Maintain a coherent narrative
4. Keep code identifiers, URIs, and model names unchanged (proper nouns)
5. Match the language of the existing memory

## Existing Memory
Category: %s

### Abstract (L0)
%s

### Overview (L1)
%s

### Content (L2)
%s

## New Information
### Abstract (L0)
%s

### Overview (L1)
%s

### Content (L2)
%s

Output the merged memory as a JSON object with keys: abstract, overview, content.
Output ONLY the JSON object, no other text.`

// llmMergeMemories calls the LLM to merge an existing memory with new candidate data.
func (a *App) llmMergeMemories(ctx context.Context, existing *Memory, candidate *CandidateMemory) (*CandidateMemory, error) {
	prompt := fmt.Sprintf(memoryMergePrompt,
		existing.Category,
		existing.Abstract, existing.Overview, existing.Content,
		candidate.Abstract, candidate.Overview, candidate.Content,
	)

	messages := []Message{
		{Role: "system", Content: "You merge memories. Output only JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := a.client.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("LLM merge call failed: %w", err)
	}

	content := strings.TrimSpace(resp.Content)
	content = stripCodeFence(content)

	var merged CandidateMemory
	if err := json.Unmarshal([]byte(content), &merged); err != nil {
		return nil, fmt.Errorf("failed to parse merge response: %w", err)
	}

	return &merged, nil
}

// formatMessagesForExtraction creates a text representation of messages for the LLM.
func formatMessagesForExtraction(messages []Message) string {
	var buf strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			fmt.Fprintf(&buf, "User: %s\n\n", truncateOutput(msg.Content, 1000))
		case "assistant":
			if msg.Content != "" {
				fmt.Fprintf(&buf, "Assistant: %s\n\n", truncateOutput(msg.Content, 1000))
			}
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&buf, "Tool call: %s(%s)\n", tc.Function.Name, truncateOutput(tc.Function.Arguments, 300))
			}
		case "tool":
			fmt.Fprintf(&buf, "Tool result: %s\n\n", truncateOutput(msg.Content, 500))
		}
	}
	return buf.String()
}

// stripCodeFence removes markdown code fences from LLM responses.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
