package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhy0216/pigo/pkg/types"
)

// ChatClient is the minimal interface needed for skill matching.
// Satisfied by *llm.Client.
type ChatClient interface {
	Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}) (*types.ChatResponse, error)
}

const skillMatchSystemPrompt = `You are a skill matcher. Given a user message and a list of available skills, determine which skills should be loaded.

Rules:
- Return a JSON array of skill names that apply to the user's task
- Return [] if no skills apply
- A skill applies if the user's task matches the skill's description
- When in doubt, include the skill (false positives are acceptable)
- Return ONLY the JSON array, no other text`

// MatchResult holds the outcome of a pre-flight skill matching call,
// including debug information about the LLM response.
type MatchResult struct {
	Names       []string // matched skill names (filtered to valid)
	RawResponse string   // raw LLM response text (empty if call failed)
	Err         error    // error from LLM call or JSON parsing, nil on success
}

// MatchSkills calls the LLM to determine which skills apply to user input.
// Always returns a non-nil MatchResult with debug information.
func MatchSkills(ctx context.Context, client ChatClient, userInput string, skills []Skill) *MatchResult {
	if len(skills) == 0 {
		return &MatchResult{}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("User message: %s\n\nAvailable skills:\n", userInput))
	validNames := make(map[string]bool)
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("- name: %q, description: %q\n", s.Name, s.Description))
		validNames[s.Name] = true
	}

	messages := []types.Message{
		{Role: "system", Content: skillMatchSystemPrompt},
		{Role: "user", Content: b.String()},
	}

	resp, err := client.Chat(ctx, messages, nil)
	if err != nil {
		return &MatchResult{Err: fmt.Errorf("LLM call failed: %w", err)}
	}

	raw := resp.Content
	matched := parseSkillNames(raw)
	if matched == nil {
		return &MatchResult{RawResponse: raw, Err: fmt.Errorf("failed to parse response as JSON array")}
	}

	// Filter to only valid skill names
	var result []string
	for _, name := range matched {
		if validNames[name] {
			result = append(result, name)
		}
	}
	return &MatchResult{Names: result, RawResponse: raw}
}

// parseSkillNames extracts a JSON string array from LLM output.
// Handles cases where the JSON is surrounded by extra text.
func parseSkillNames(content string) []string {
	content = strings.TrimSpace(content)

	// Try direct parse first
	var names []string
	if err := json.Unmarshal([]byte(content), &names); err == nil {
		return names
	}

	// Try extracting JSON array from surrounding text
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(content[start:end+1]), &names); err == nil {
			return names
		}
	}

	return nil
}
