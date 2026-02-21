# Pre-flight Skill Matching Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Guarantee skill loading by making a lightweight LLM call to match user input against available skills before the main agent loop, injecting matched skill content as system messages.

**Architecture:** New `MatchSkills()` function in `pkg/skills/match.go` takes a `ChatClient` interface (satisfied by `*llm.Client`), the user input, and visible skills. It makes a non-streaming LLM call that returns a JSON array of matching skill names. The agent calls this in `ProcessInput()` before the agent loop and injects matched skill content as system messages. A shared `LoadSkillContent()` helper is extracted to avoid duplication with `use_skill.go`.

**Tech Stack:** Go, OpenAI Chat Completions API (non-streaming)

---

### Task 1: Extract shared LoadSkillContent helper

**Files:**
- Create: `pkg/skills/content.go`
- Test: `pkg/skills/content_test.go`
- Modify: `pkg/tools/use_skill.go:72-83` (use new helper)

**Step 1: Write the failing test**

```go
// pkg/skills/content_test.go
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillContent(t *testing.T) {
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	os.WriteFile(skillFile, []byte("---\nname: test-skill\ndescription: A test\n---\n# Test Skill\n\nSome content here."), 0644)

	skill := Skill{Name: "test-skill", FilePath: skillFile}
	content, err := LoadSkillContent(skill)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "<skill name=\"test-skill\">\n# Test Skill\n\nSome content here.\n</skill>"
	if content != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, content)
	}
}

func TestLoadSkillContentMissingFile(t *testing.T) {
	skill := Skill{Name: "missing", FilePath: "/nonexistent/SKILL.md"}
	_, err := LoadSkillContent(skill)
	if err == nil {
		t.Error("expected error for missing file")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/skills/ -run TestLoadSkillContent -v`
Expected: FAIL with "undefined: LoadSkillContent"

**Step 3: Write minimal implementation**

```go
// pkg/skills/content.go
package skills

import (
	"fmt"
	"os"
	"strings"
)

// LoadSkillContent reads a skill file, strips frontmatter, and wraps the body
// in <skill> XML tags. Returns the formatted content ready for injection.
func LoadSkillContent(skill Skill) (string, error) {
	raw, err := os.ReadFile(skill.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read skill file: %w", err)
	}

	_, body := ParseFrontmatter(string(raw))
	body = strings.TrimSpace(body)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<skill name=%q>\n", skill.Name))
	b.WriteString(body)
	b.WriteString("\n</skill>")
	return b.String(), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/skills/ -run TestLoadSkillContent -v`
Expected: PASS

**Step 5: Refactor use_skill.go to use LoadSkillContent**

In `pkg/tools/use_skill.go`, replace lines 72-83:

```go
// Before (lines 72-83):
content, err := os.ReadFile(skill.FilePath)
if err != nil {
    return types.ErrorResult(fmt.Sprintf("failed to read skill file: %v", err))
}
_, body := skills.ParseFrontmatter(string(content))
body = strings.TrimSpace(body)
var b strings.Builder
b.WriteString(fmt.Sprintf("<skill name=%q>\n", name))
b.WriteString(body)
b.WriteString("\n</skill>")
result := types.NewToolResult(b.String())

// After:
formatted, err := skills.LoadSkillContent(*skill)
if err != nil {
    return types.ErrorResult(err.Error())
}
result := types.NewToolResult(formatted)
```

Remove unused imports from `use_skill.go`: `"os"`, `"strings"`, `"fmt"` (keep `"fmt"` if still used elsewhere — check).

**Step 6: Run all tests to verify refactor**

Run: `go test ./pkg/skills/ ./pkg/tools/ -v`
Expected: All PASS

**Step 7: Commit**

```bash
git add pkg/skills/content.go pkg/skills/content_test.go pkg/tools/use_skill.go
git commit -m "refactor: extract LoadSkillContent helper from use_skill tool"
```

---

### Task 2: Add ChatClient interface and MatchSkills function

**Files:**
- Create: `pkg/skills/match.go`
- Test: `pkg/skills/match_test.go`

**Step 1: Write the failing tests**

```go
// pkg/skills/match_test.go
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
		client := &mockChatClient{response: "Here are the matches: [\"brainstorming\"] hope that helps"}
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/skills/ -run TestMatchSkills -v`
Expected: FAIL with "undefined: MatchSkills"

**Step 3: Write minimal implementation**

```go
// pkg/skills/match.go
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

// MatchSkills calls the LLM to determine which skills apply to user input.
// Returns matched skill names, or nil on error (silent fallback).
func MatchSkills(ctx context.Context, client ChatClient, userInput string, skills []Skill) []string {
	if len(skills) == 0 {
		return nil
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
		return nil
	}

	matched := parseSkillNames(resp.Content)
	if matched == nil {
		return nil
	}

	// Filter to only valid skill names
	var result []string
	for _, name := range matched {
		if validNames[name] {
			result = append(result, name)
		}
	}
	return result
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/skills/ -run TestMatchSkills -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/skills/match.go pkg/skills/match_test.go
git commit -m "feat: add pre-flight skill matching via LLM call"
```

---

### Task 3: Integrate MatchSkills into agent ProcessInput

**Files:**
- Modify: `pkg/agent/agent.go:200-230` (ProcessInput, add pre-flight matching)
- Modify: `pkg/agent/agent.go:22-32` (Agent struct, store visible skills)
- Test: `pkg/agent/agent_test.go` (new test + update existing tool count test)

**Step 1: Write the failing test**

Add to `pkg/agent/agent_test.go`:

```go
func TestProcessInputPreflightSkillMatching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))

		callCount++

		// First call is the pre-flight skill matching (non-streaming)
		if callCount == 1 {
			// Verify it's non-streaming
			if bytes.Contains(body, []byte(`"stream":true`)) || bytes.Contains(body, []byte(`"stream": true`)) {
				t.Error("expected non-streaming request for skill matching")
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "chatcmpl-match", "object": "chat.completion",
				"created": 1677652288, "model": "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": `["test-skill"]`,
						},
						"finish_reason": "stop",
					},
				},
			})
			return
		}

		// Second call is the main agent loop (streaming)
		// Verify skill content was injected into messages
		if !bytes.Contains(body, []byte(`<skill name="test-skill">`)) {
			t.Error("expected skill content to be injected into messages")
		}

		response := map[string]interface{}{
			"id": "chatcmpl-main", "object": "chat.completion",
			"created": 1677652288, "model": "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "I loaded the skill!",
					},
					"finish_reason": "stop",
				},
			},
		}
		mockRespond(w, r, response)
	}))
	defer server.Close()

	// Create a temporary skill file
	dir := t.TempDir()
	skillFile := filepath.Join(dir, "SKILL.md")
	os.WriteFile(skillFile, []byte("---\nname: test-skill\ndescription: Use for testing\n---\n# Test Skill\n\nDo the test thing."), 0644)

	cfg := &config.Config{APIKey: "test-key", BaseURL: server.URL, Model: "gpt-4"}
	agent := NewAgent(cfg)
	agent.output = &bytes.Buffer{}
	agent.visibleSkills = []skills.Skill{
		{Name: "test-skill", Description: "Use for testing", FilePath: skillFile},
	}

	err := agent.ProcessInput(context.Background(), "run the test thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 API calls (match + main), got %d", callCount)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/agent/ -run TestProcessInputPreflightSkillMatching -v`
Expected: FAIL (agent.visibleSkills undefined)

**Step 3: Implement the integration**

In `pkg/agent/agent.go`, add `visibleSkills` field to Agent struct:

```go
type Agent struct {
	client        *llm.Client
	registry      *tools.ToolRegistry
	messages      []types.Message
	output        io.Writer
	Skills        []skills.Skill
	visibleSkills []skills.Skill // skills eligible for pre-flight matching
	events        *types.EventEmitter
	hookMgr       *hooks.HookManager
	usage         types.TokenUsage
}
```

In `NewAgent()`, after loading skills, compute visible skills:

```go
// After line 66: registry.Register(tools.NewUseSkillTool(loadedSkills))
var visibleSkills []skills.Skill
for _, s := range loadedSkills {
    if !s.DisableModelInvocation {
        visibleSkills = append(visibleSkills, s)
    }
}
```

And set it on the agent (add `visibleSkills: visibleSkills,` in the struct literal).

In `ProcessInput()`, after appending the user message (line 209) and before the agent loop (line 227), add:

```go
// Pre-flight skill matching
if len(a.visibleSkills) > 0 {
    matched := skills.MatchSkills(ctx, a.client, input, a.visibleSkills)
    for _, name := range matched {
        for _, s := range a.visibleSkills {
            if s.Name == name {
                content, err := skills.LoadSkillContent(s)
                if err != nil {
                    fmt.Fprintf(a.output, "%s[skill match warning: %v]%s\n", types.ColorYellow, err, types.ColorReset)
                    continue
                }
                a.messages = append(a.messages, types.Message{
                    Role:    "system",
                    Content: content,
                })
                fmt.Fprintf(a.output, "%s[skill: %s]%s\n", types.ColorGray, name, types.ColorReset)
                break
            }
        }
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/agent/ -run TestProcessInputPreflightSkillMatching -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add pkg/agent/agent.go pkg/agent/agent_test.go
git commit -m "feat: integrate pre-flight skill matching into agent loop"
```

---

### Task 4: Verify end-to-end and run full test suite

**Step 1: Run full test suite with race detector**

Run: `make test-race`
Expected: All PASS

**Step 2: Run linter**

Run: `make lint`
Expected: Clean

**Step 3: Build**

Run: `make build`
Expected: Clean build

**Step 4: Commit any fixes if needed**
