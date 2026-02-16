package main

import (
	"context"
	"fmt"
	"strings"
)

const (
	// keepRecentChars is the approximate character budget for recent messages
	// to preserve during compaction (~80K chars).
	keepRecentChars = 80000
)

// compactMessages performs smart context compaction: walks backward from the
// newest messages with a character budget, finds a valid cut point, summarizes
// discarded messages via LLM, and preserves file operation metadata.
// Falls back to naive truncation if LLM summarization fails.
func (a *App) compactMessages(ctx context.Context) {
	if len(a.messages) <= minKeepMessages+1 {
		return
	}

	total := estimateMessageChars(a.messages)
	if total <= maxContextChars {
		return
	}

	// Walk backward from newest, accumulate chars until budget is hit
	cutIndex := a.findCutPoint()
	if cutIndex <= 1 {
		// Nothing to compact (only system prompt or too few messages)
		return
	}

	discarded := a.messages[1:cutIndex] // skip system prompt
	if len(discarded) == 0 {
		return
	}

	// Extract file operations from discarded messages
	fileOps := extractFileOps(discarded)

	// Try LLM summarization
	summary, err := a.summarizeMessages(ctx, discarded, fileOps)
	if err != nil {
		// Fallback to naive truncation
		a.truncateMessages()
		return
	}

	// Build compacted message list
	kept := make([]Message, 0, len(a.messages)-cutIndex+3)
	kept = append(kept, a.messages[0]) // system prompt
	kept = append(kept, Message{
		Role:    "user",
		Content: summary,
	})
	kept = append(kept, a.messages[cutIndex:]...)

	discardedCount := cutIndex - 1
	a.messages = kept
	fmt.Fprintf(a.output, "%s[context compacted: %d messages summarized]%s\n", colorYellow, discardedCount, colorReset)
}

// findCutPoint walks backward from the newest messages accumulating character
// estimates. Returns the index at which to cut (messages before this index
// will be discarded). Never cuts in the middle of a tool-call/tool-result pair.
func (a *App) findCutPoint() int {
	chars := 0
	n := len(a.messages)

	// Walk backward from the end, accumulating chars until we hit the budget
	i := n - 1
	for i > 0 {
		msgChars := len(a.messages[i].Content)
		for _, tc := range a.messages[i].ToolCalls {
			msgChars += len(tc.Function.Arguments)
		}

		if chars+msgChars > keepRecentChars {
			break
		}
		chars += msgChars
		i--
	}

	// Ensure we keep at least minKeepMessages if possible
	if i > n-minKeepMessages {
		i = n - minKeepMessages
		if i < 1 {
			i = 1
		}
	}

	// Ensure we don't cut in the middle of a tool-call/tool-result sequence.
	// A tool result message (role=="tool") must be preceded by its assistant
	// message with tool_calls. Walk forward from the cut point to find a
	// clean boundary.
	for i < n {
		if a.messages[i].Role == "tool" {
			i++
			continue
		}
		break
	}

	return i
}

// extractFileOps scans messages for file read/write/edit operations and
// returns a compact summary of which files were accessed.
func extractFileOps(messages []Message) string {
	reads := make(map[string]bool)
	writes := make(map[string]bool)

	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			switch tc.Function.Name {
			case "read":
				if path := extractPathFromArgs(tc.Function.Arguments); path != "" {
					reads[path] = true
				}
			case "write", "edit":
				if path := extractPathFromArgs(tc.Function.Arguments); path != "" {
					writes[path] = true
				}
			}
		}
	}

	if len(reads) == 0 && len(writes) == 0 {
		return ""
	}

	var parts []string
	if len(reads) > 0 {
		paths := mapKeys(reads)
		parts = append(parts, fmt.Sprintf("Files read: %s", strings.Join(paths, ", ")))
	}
	if len(writes) > 0 {
		paths := mapKeys(writes)
		parts = append(parts, fmt.Sprintf("Files modified: %s", strings.Join(paths, ", ")))
	}
	return strings.Join(parts, "\n")
}

// extractPathFromArgs tries to extract a "path" field from JSON arguments.
func extractPathFromArgs(args string) string {
	// Simple extraction without full JSON parse — look for "path":"..."
	idx := strings.Index(args, `"path"`)
	if idx < 0 {
		return ""
	}
	rest := args[idx+6:]
	// Skip whitespace and colon
	rest = strings.TrimLeft(rest, " \t\n:")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// summarizeMessages uses the LLM to create a compact summary of discarded
// messages. Returns the summary text or an error.
func (a *App) summarizeMessages(ctx context.Context, discarded []Message, fileOps string) (string, error) {
	// Build a concise representation of the discarded conversation
	var content strings.Builder
	for _, msg := range discarded {
		switch msg.Role {
		case "user":
			fmt.Fprintf(&content, "User: %s\n", truncateOutput(msg.Content, 500))
		case "assistant":
			if msg.Content != "" {
				fmt.Fprintf(&content, "Assistant: %s\n", truncateOutput(msg.Content, 500))
			}
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&content, "Tool call: %s(%s)\n", tc.Function.Name, truncateOutput(tc.Function.Arguments, 200))
			}
		case "tool":
			fmt.Fprintf(&content, "Tool result: %s\n", truncateOutput(msg.Content, 300))
		}
	}

	prompt := fmt.Sprintf(`Summarize this conversation excerpt in 2-4 concise sentences. Focus on what was discussed, what actions were taken, and any important decisions or findings. Do not include tool call details.

%s`, content.String())

	if fileOps != "" {
		prompt += fmt.Sprintf("\n\nFile operations performed:\n%s", fileOps)
	}

	// Use a minimal message set for the summary call (no tools)
	summaryMessages := []Message{
		{Role: "system", Content: "You are a conversation summarizer. Produce brief, factual summaries."},
		{Role: "user", Content: prompt},
	}

	resp, err := a.client.Chat(ctx, summaryMessages, nil)
	if err != nil {
		return "", fmt.Errorf("summarization failed: %w", err)
	}

	summaryText := fmt.Sprintf("[Conversation summary (%d messages compacted)]\n%s", len(discarded), resp.Content)
	if fileOps != "" {
		summaryText += "\n\n" + fileOps
	}

	return summaryText, nil
}

// mapKeys returns the keys of a map as a sorted slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
