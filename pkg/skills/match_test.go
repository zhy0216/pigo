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

func (m *mockChatClient) Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}, opts ...types.ChatOption) (*types.ChatResponse, error) {
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
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 1 || result.Names[0] != "brainstorming" {
			t.Errorf("expected [brainstorming], got %v", result.Names)
		}
		if result.RawResponse != `["brainstorming"]` {
			t.Errorf("expected raw response preserved, got %q", result.RawResponse)
		}
	})

	t.Run("matches multiple skills", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming", "debugging"]`}
		result := MatchSkills(context.Background(), client, "fix and redesign the login", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 2 {
			t.Errorf("expected 2 skills, got %v", result.Names)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		client := &mockChatClient{response: `[]`}
		result := MatchSkills(context.Background(), client, "hello", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 0 {
			t.Errorf("expected empty, got %v", result.Names)
		}
		if result.RawResponse != `[]` {
			t.Errorf("expected raw response preserved, got %q", result.RawResponse)
		}
	})

	t.Run("LLM error populates Err", func(t *testing.T) {
		client := &mockChatClient{err: fmt.Errorf("network error")}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result.Err == nil {
			t.Error("expected error, got nil")
		}
		if len(result.Names) != 0 {
			t.Errorf("expected no names on error, got %v", result.Names)
		}
	})

	t.Run("malformed JSON populates Err", func(t *testing.T) {
		client := &mockChatClient{response: "not json"}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result.Err == nil {
			t.Error("expected error for bad JSON, got nil")
		}
		if result.RawResponse != "not json" {
			t.Errorf("expected raw response preserved on parse error, got %q", result.RawResponse)
		}
	})

	t.Run("JSON with surrounding text", func(t *testing.T) {
		client := &mockChatClient{response: `Here are the matches: ["brainstorming"] hope that helps`}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 1 || result.Names[0] != "brainstorming" {
			t.Errorf("expected [brainstorming] from wrapped JSON, got %v", result.Names)
		}
	})

	t.Run("JSON in markdown code fence", func(t *testing.T) {
		client := &mockChatClient{response: "```json\n[\"brainstorming\"]\n```"}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 1 || result.Names[0] != "brainstorming" {
			t.Errorf("expected [brainstorming] from fenced JSON, got %v", result.Names)
		}
	})

	t.Run("JSON in bare code fence", func(t *testing.T) {
		client := &mockChatClient{response: "```\n[\"debugging\"]\n```"}
		result := MatchSkills(context.Background(), client, "fix a bug", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 1 || result.Names[0] != "debugging" {
			t.Errorf("expected [debugging] from bare fenced JSON, got %v", result.Names)
		}
	})

	t.Run("filters unknown skill names", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming", "nonexistent"]`}
		result := MatchSkills(context.Background(), client, "add feature", skills)
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if len(result.Names) != 1 || result.Names[0] != "brainstorming" {
			t.Errorf("expected [brainstorming] after filtering, got %v", result.Names)
		}
	})

	t.Run("empty skills list returns empty result", func(t *testing.T) {
		client := &mockChatClient{response: `["brainstorming"]`}
		result := MatchSkills(context.Background(), client, "add feature", nil)
		if result.Err != nil {
			t.Errorf("expected no error for empty skills, got %v", result.Err)
		}
		if len(result.Names) != 0 {
			t.Errorf("expected no names for empty skills, got %v", result.Names)
		}
	})
}
