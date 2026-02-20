package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
)

// compactMessages performs smart context compaction: walks backward from the
// newest messages with a character budget, finds a valid cut point, summarizes
// discarded messages via LLM, and preserves file operation metadata.
// Falls back to naive truncation if LLM summarization fails.
func (a *Agent) compactMessages(ctx context.Context) {
	if len(a.messages) <= types.MinKeepMessages+1 {
		return
	}

	total := util.EstimateMessageChars(a.messages)
	threshold := int(float64(types.MaxContextChars) * types.ProactiveCompactThreshold)
	if total <= threshold {
		return
	}

	cutIndex := a.findCutPoint()
	if cutIndex <= 1 {
		return
	}

	discarded := a.messages[1:cutIndex]
	if len(discarded) == 0 {
		return
	}

	fileOps := extractFileOps(discarded)

	summary, err := a.summarizeMessages(ctx, discarded, fileOps)
	if err != nil {
		a.truncateMessages()
		return
	}

	kept := make([]types.Message, 0, len(a.messages)-cutIndex+3)
	kept = append(kept, a.messages[0])
	kept = append(kept, types.Message{
		Role:    "user",
		Content: summary,
	})
	kept = append(kept, a.messages[cutIndex:]...)

	discardedCount := cutIndex - 1
	a.messages = kept
	fmt.Fprintf(a.output, "%s[context compacted: %d messages summarized]%s\n", types.ColorYellow, discardedCount, types.ColorReset)
}

// findCutPoint walks backward from the newest messages accumulating character
// estimates. Returns the index at which to cut.
func (a *Agent) findCutPoint() int {
	chars := 0
	n := len(a.messages)

	i := n - 1
	for i > 0 {
		msgChars := len(a.messages[i].Content)
		for _, tc := range a.messages[i].ToolCalls {
			msgChars += len(tc.Function.Arguments)
		}

		if chars+msgChars > types.KeepRecentChars {
			break
		}
		chars += msgChars
		i--
	}

	if i > n-types.MinKeepMessages {
		i = n - types.MinKeepMessages
		if i < 1 {
			i = 1
		}
	}

	for i < n && a.messages[i].Role == "tool" {
		i++
	}

	if i >= n {
		return 0
	}

	return i
}

// extractFileOps scans messages for file read/write/edit operations and
// returns a compact summary of which files were accessed.
func extractFileOps(messages []types.Message) string {
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
		paths := util.MapKeys(reads)
		parts = append(parts, fmt.Sprintf("Files read: %s", strings.Join(paths, ", ")))
	}
	if len(writes) > 0 {
		paths := util.MapKeys(writes)
		parts = append(parts, fmt.Sprintf("Files modified: %s", strings.Join(paths, ", ")))
	}
	return strings.Join(parts, "\n")
}

// extractPathFromArgs extracts a "path" field from JSON arguments.
func extractPathFromArgs(args string) string {
	var parsed struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return ""
	}
	return parsed.Path
}

// summarizeMessages uses the LLM to create a compact summary of discarded messages.
func (a *Agent) summarizeMessages(ctx context.Context, discarded []types.Message, fileOps string) (string, error) {
	var content strings.Builder
	for _, msg := range discarded {
		switch msg.Role {
		case "user":
			fmt.Fprintf(&content, "User: %s\n", util.TruncateOutput(msg.Content, 500))
		case "assistant":
			if msg.Content != "" {
				fmt.Fprintf(&content, "Assistant: %s\n", util.TruncateOutput(msg.Content, 500))
			}
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&content, "Tool call: %s(%s)\n", tc.Function.Name, util.TruncateOutput(tc.Function.Arguments, 200))
			}
		case "tool":
			fmt.Fprintf(&content, "Tool result: %s\n", util.TruncateOutput(msg.Content, 300))
		}
	}

	prompt := fmt.Sprintf(`Summarize this conversation excerpt in 2-4 concise sentences. Focus on what was discussed, what actions were taken, and any important decisions or findings. Do not include tool call details.

%s`, content.String())

	if fileOps != "" {
		prompt += fmt.Sprintf("\n\nFile operations performed:\n%s", fileOps)
	}

	summaryMessages := []types.Message{
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
