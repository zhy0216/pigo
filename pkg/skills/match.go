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
	Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, opts ...types.ChatOption) (*types.ChatResponse, error)
}

const skillMatchSystemPrompt = `You classify user messages against a skill list. Do NOT respond to the user. Do NOT explain anything. Output ONLY a json object.

Output format: {"skills": ["name1", "name2"]}
If no skills match: {"skills": []}

Example:
User message: "fix the login bug"
Skills: "brainstorming" (creative work), "debugging" (encountering bugs)
Output: {"skills": ["debugging"]}

Example:
User message: "hello"
Skills: "brainstorming" (creative work), "debugging" (encountering bugs)
Output: {"skills": []}`

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
	b.WriteString("\nRespond with a json object.")

	messages := []types.Message{
		{Role: "system", Content: skillMatchSystemPrompt},
		{Role: "user", Content: b.String()},
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skills": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		"required":             []string{"skills"},
		"additionalProperties": false,
	}

	resp, err := client.Chat(ctx, messages, nil, types.WithJSONSchema("skill_match", schema))
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

// parseSkillNames extracts skill names from LLM output.
// Accepts {"skills": [...]} objects, plain JSON arrays, markdown fences, or
// arrays embedded in surrounding text.
func parseSkillNames(content string) []string {
	content = strings.TrimSpace(content)

	// Strip markdown code fences if present
	if stripped := stripCodeFences(content); stripped != content {
		content = stripped
	}

	// Try parsing as JSON object — look for {"skills": [...]}
	// If it's a valid JSON object without a "skills" key, the model
	// didn't follow the format but we treat it as no matches.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &obj); err == nil {
		if raw, ok := obj["skills"]; ok {
			var names []string
			if err := json.Unmarshal(raw, &names); err == nil {
				return names
			}
		}
		// Valid JSON object but no "skills" key — no matches
		return []string{}
	}

	// Try direct array parse
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

// stripCodeFences removes markdown code fences (```json ... ``` or ``` ... ```)
// and returns the inner content trimmed.
func stripCodeFences(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(first, "```") && last == "```" {
		inner := strings.Join(lines[1:len(lines)-1], "\n")
		return strings.TrimSpace(inner)
	}
	return s
}
