# Design: Proactive Skill Invocation via `use_skill` Tool

## Problem

Skills are currently only invocable by users via `/skill:name` slash commands. The AI model sees skill names and descriptions in the system prompt but has no mechanism to invoke them during the agent loop.

## Decision

Register a single `use_skill` tool that the AI model can call to load skill content by name. This is Approach A (single tool with `name` parameter), chosen over per-skill tool registration (Approach B) for simplicity and scalability.

## Design

### New Tool: `use_skill`

**File**: `pkg/tools/use_skill.go`

```go
type UseSkillTool struct {
    skills []skills.Skill // only model-invokable skills
}
```

**Tool definition:**
- **Name**: `use_skill`
- **Description**: "Load and execute a skill by name. Skills provide specialized instructions for specific tasks. Available skills are listed in the system prompt."
- **Parameters**: `{ "name": string (required) }` — the skill name to invoke

**Execute logic:**
1. Look up skill by `name` in internal list
2. Not found → `ErrorResult("skill not found: <name>")`
3. Read file from `skill.FilePath`
4. Strip YAML frontmatter via `ParseFrontmatter()`
5. Wrap body in `<skill name="...">...</skill>` XML
6. Return as `ToolResult`

### Registration

In `pkg/agent/agent.go`, after existing tool registrations:

```go
registry.Register(tools.NewUseSkillTool(loadedSkills))
```

### Filtering

Skills with `DisableModelInvocation=true` are excluded from the tool's internal list at construction time, consistent with their exclusion from the system prompt.

### Unchanged

- `FormatSkillsForPrompt` — still lists skill names/descriptions in system prompt
- User `/skill:name` invocation — unchanged in `cmd/pigo/main.go`
- Hook system — `use_skill` calls trigger `tool_start`/`tool_end` hooks automatically

### Scope

- One new file: `pkg/tools/use_skill.go`
- One new test file: `pkg/tools/use_skill_test.go`
- One line added in `pkg/agent/agent.go`
