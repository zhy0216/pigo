# Pre-flight Skill Matching Design

## Problem

When skills are available, pigo relies on the LLM voluntarily calling `use_skill` before responding. The system prompt instructs it to do so, but models (especially GPT-4o) frequently ignore this and respond directly. This means skills like `brainstorming` never get loaded even when they clearly apply.

## Solution

Add a client-side pre-flight LLM call that matches user input against available skills before the main agent loop. Matched skills are injected as system messages so the LLM has them in context regardless of whether it would have called `use_skill` itself.

## Flow

```
User sends message
  │
  ├─ Any visible skills?
  │    No → proceed to agent loop as normal
  │    Yes ↓
  │
  ├─ Pre-flight LLM call:
  │    "Given these skills and this user message, which skills apply?"
  │    Returns: JSON array of skill names (or [])
  │
  ├─ For each matched skill:
  │    Read file, strip frontmatter, inject as system message
  │
  └─ Agent loop runs (skill content already in context)
```

## Pre-flight Prompt

System prompt for the matching call:

```
You are a skill matcher. Given a user message and a list of available skills, determine which skills should be loaded.

Rules:
- Return a JSON array of skill names that apply to the user's task
- Return [] if no skills apply
- A skill applies if the user's task matches the skill's description
- When in doubt, include the skill (false positives are acceptable)
- Return ONLY the JSON array, no other text
```

User message format:

```
User message: <the user input>

Available skills:
- name: "brainstorming", description: "Use before any creative work..."
- name: "test-driven-development", description: "Use when implementing..."
```

## Implementation Details

### New function: `pkg/skills/match.go`

```go
// SkillMatcher calls the LLM to determine which skills apply to user input.
type SkillMatcher struct {
    client ChatClient // interface for LLM calls (non-streaming)
}

// Match returns skill names that apply to the given user input.
// Returns nil on error (silent fallback).
func (m *SkillMatcher) Match(ctx context.Context, userInput string, skills []Skill) []string
```

### Interface for LLM calls

To avoid circular imports (`skills` depending on `llm`), define a minimal interface:

```go
// ChatClient is the minimal interface needed for skill matching.
type ChatClient interface {
    Chat(ctx context.Context, messages []types.Message, toolDefs []map[string]interface{}) (*types.ChatResponse, error)
}
```

This interface is satisfied by `*llm.Client` already.

### Integration in `agent.ProcessInput()`

After appending the user message but before the agent loop:

```go
// Pre-flight skill matching
if len(visibleSkills) > 0 {
    matched := skills.MatchSkills(ctx, a.client, input, visibleSkills)
    for _, name := range matched {
        content := loadSkillContent(name) // read file, strip frontmatter
        a.messages = append(a.messages, types.Message{
            Role:    "system",
            Content: fmt.Sprintf("<skill name=%q>\n%s\n</skill>", name, content),
        })
    }
}
```

### Key behaviors

- **Model:** Uses the same configured model. The prompt is ~200 tokens so cost is negligible.
- **No streaming:** The matching call uses `Chat()` not `ChatStream()`.
- **No tools sent:** The matching call sends no tool definitions (pure text response).
- **Timeout:** Uses the same context; if cancelled, matching is skipped.
- **Fallback:** If the LLM call fails or returns unparseable JSON, proceed silently. The `use_skill` tool still exists as a manual fallback.
- **`use_skill` tool remains:** The LLM can still load additional skills during conversation.
- **Only model-invocable skills:** Skills with `disable-model-invocation: true` are excluded from matching, same as current behavior.

### Skill content injection format

Same format as `use_skill` tool output:

```xml
<skill name="brainstorming">
# Brainstorming Ideas Into Designs
...skill body...
</skill>
```

Injected as a system message so it appears as authoritative context, not as a previous tool call.

## What stays the same

- `use_skill` tool registration and functionality
- `FormatSkillsForPrompt()` system prompt section
- Skill loading, frontmatter parsing
- `/skill:name` command expansion
- All existing tests

## Testing

- Unit test for `Match()` with mock LLM client returning known JSON
- Unit test for JSON parsing edge cases (empty array, malformed, extra text around JSON)
- Integration test verifying skill content appears in messages before agent loop
