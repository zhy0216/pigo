package skills

import (
	"context"
	"fmt"
	"testing"

	"github.com/zhy0216/pigo/pkg/types"
)

// mockChatClient implements ChatClient for testing.
type mockChatClient struct {
	response string
	err      error
}

func (m *mockChatClient) Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}) (*types.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &types.ChatResponse{Content: m.response}, nil
}

func TestMatchSkills(t *testing.T) {
	skills := []Skill{
		{Name: "brainstorming", Description: "Use before any creative work"},
		{Name: "debugging", Description: "Use when encountering bugs"},
	}

	t.Run("matches single skill", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming"]`}
		result := MatchSkills(context.Background(), client, "add a new feature", skills)
		if len(result) != 1 || result[0] != "brainstorming" {
			t.Errorf("expected [brainstorming], got %v", result)
		}
	})

	t.Run("matches multiple skills", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming", "debugging"]`}
		result := MatchSkills(context.Background(), client, "fix and redesign the login", skills)
		if len(result) != 2 {
			t.Errorf("expected 2 skills, got %v", result)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		client := &mockChatClient{response: `[]`}
		result := MatchSkills(context.Background(), client, "hello", skills)
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})

	t.Run("LLM error returns nil", func(t *testing.T) {
		client := &mockChatClient{err: fmt.Errorf("network error")}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result != nil {
			t.Errorf("expected nil on error, got %v", result)
		}
	})

	t.Run("malformed JSON returns nil", func(t *testing.T) {
		client := &mockChatClient{response: "not json"}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result != nil {
			t.Errorf("expected nil on bad JSON, got %v", result)
		}
	})

	t.Run("JSON with surrounding text", func(t *testing.T) {
		client := &mockChatClient{response: `Here are the matches: ["brainstorming"] hope that helps`}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if len(result) != 1 || result[0] != "brainstorming" {
			t.Errorf("expected [brainstorming] from wrapped JSON, got %v", result)
		}
	})

	t.Run("filters unknown skill names", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming", "nonexistent"]`}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if len(result) != 1 || result[0] != "brainstorming" {
			t.Errorf("expected [brainstorming] after filtering, got %v", result)
		}
	})

	t.Run("empty skills list skips call", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming"]`}
		result := MatchSkills(context.Background(), client, "add feature", nil)
		if result != nil {
			t.Errorf("expected nil for empty skills, got %v", result)
		}
	})
}
