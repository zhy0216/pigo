# `use_skill` Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow the AI model to proactively invoke skills during the agent loop via a new `use_skill` tool.

**Architecture:** A single `use_skill` tool registered in the tool registry, accepting a skill name and returning the skill's markdown content wrapped in XML. Filters out `DisableModelInvocation` skills at construction time.

**Tech Stack:** Go, existing `pkg/tools` + `pkg/skills` packages.

---

### Task 1: Write failing tests for `use_skill` tool

**Files:**
- Create: `pkg/tools/use_skill_test.go`

**Step 1: Write the test file**

```go
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhy0216/pigo/pkg/skills"
)

func createSkillFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUseSkillTool_Name(t *testing.T) {
	tool := NewUseSkillTool(nil)
	if tool.Name() != "use_skill" {
		t.Errorf("expected 'use_skill', got '%s'", tool.Name())
	}
}

func TestUseSkillTool_Parameters(t *testing.T) {
	tool := NewUseSkillTool(nil)
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Error("expected type 'object'")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok || props["name"] == nil {
		t.Error("expected 'name' property")
	}
	required, ok := params["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "name" {
		t.Error("expected 'name' to be required")
	}
}

func TestUseSkillTool_Success(t *testing.T) {
	tmpDir := t.TempDir()
	path := createSkillFile(t, tmpDir, "deploy", "---\nname: deploy\ndescription: Deploy the app\n---\n# Deploy\nRun deploy steps.")

	tool := NewUseSkillTool([]skills.Skill{
		{Name: "deploy", Description: "Deploy the app", FilePath: path},
	})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "deploy",
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "<skill name=\"deploy\">") {
		t.Errorf("expected <skill> XML wrapper, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "# Deploy") {
		t.Errorf("expected skill body content, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Run deploy steps.") {
		t.Errorf("expected skill body content, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "</skill>") {
		t.Errorf("expected closing </skill> tag, got: %s", result.ForLLM)
	}
}

func TestUseSkillTool_NotFound(t *testing.T) {
	tool := NewUseSkillTool([]skills.Skill{
		{Name: "deploy", Description: "Deploy the app", FilePath: "/nonexistent"},
	})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "unknown",
	})

	if !result.IsError {
		t.Error("expected error for unknown skill")
	}
	if !strings.Contains(result.ForLLM, "skill not found") {
		t.Errorf("expected 'skill not found' error, got: %s", result.ForLLM)
	}
}

func TestUseSkillTool_MissingName(t *testing.T) {
	tool := NewUseSkillTool(nil)

	result := tool.Execute(context.Background(), map[string]interface{}{})

	if !result.IsError {
		t.Error("expected error for missing name")
	}
}

func TestUseSkillTool_FiltersDisabledSkills(t *testing.T) {
	tool := NewUseSkillTool([]skills.Skill{
		{Name: "visible", Description: "Visible", FilePath: "/a", DisableModelInvocation: false},
		{Name: "hidden", Description: "Hidden", FilePath: "/b", DisableModelInvocation: true},
	})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "hidden",
	})

	if !result.IsError {
		t.Error("expected error for disabled skill")
	}
	if !strings.Contains(result.ForLLM, "skill not found") {
		t.Errorf("expected 'skill not found', got: %s", result.ForLLM)
	}
}

func TestUseSkillTool_FileReadError(t *testing.T) {
	tool := NewUseSkillTool([]skills.Skill{
		{Name: "broken", Description: "Broken", FilePath: "/nonexistent/path.md"},
	})

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "broken",
	})

	if !result.IsError {
		t.Error("expected error for unreadable file")
	}
}

func TestUseSkillTool_NoSkills(t *testing.T) {
	tool := NewUseSkillTool(nil)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "anything",
	})

	if !result.IsError {
		t.Error("expected error when no skills loaded")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/tools/ -run TestUseSkillTool -v`
Expected: FAIL — `NewUseSkillTool` undefined

---

### Task 2: Implement `use_skill` tool

**Files:**
- Create: `pkg/tools/use_skill.go`

**Step 1: Write the implementation**

```go
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zhy0216/pigo/pkg/skills"
	"github.com/zhy0216/pigo/pkg/types"
)

// UseSkillTool loads skill content by name for the AI model.
type UseSkillTool struct {
	skills []skills.Skill
}

// NewUseSkillTool creates a new UseSkillTool. Skills with DisableModelInvocation
// are filtered out at construction time.
func NewUseSkillTool(allSkills []skills.Skill) *UseSkillTool {
	var filtered []skills.Skill
	for _, s := range allSkills {
		if !s.DisableModelInvocation {
			filtered = append(filtered, s)
		}
	}
	return &UseSkillTool{skills: filtered}
}

func (t *UseSkillTool) Name() string {
	return "use_skill"
}

func (t *UseSkillTool) Description() string {
	return "Load a skill by name. Skills provide specialized instructions for specific tasks. Available skills are listed in the system prompt."
}

func (t *UseSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The skill name to load",
			},
		},
		"required": []string{"name"},
	}
}

func (t *UseSkillTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return types.ErrorResult("missing required parameter: name")
	}

	var skill *skills.Skill
	for i := range t.skills {
		if t.skills[i].Name == name {
			skill = &t.skills[i]
			break
		}
	}
	if skill == nil {
		return types.ErrorResult(fmt.Sprintf("skill not found: %s", name))
	}

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

	return types.NewToolResult(b.String())
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./pkg/tools/ -run TestUseSkillTool -v`
Expected: PASS — all 7 tests pass

**Step 3: Run full test suite**

Run: `make test`
Expected: PASS — no regressions

**Step 4: Commit**

```bash
git add pkg/tools/use_skill.go pkg/tools/use_skill_test.go
git commit -m "feat: add use_skill tool for AI-driven skill invocation"
```

---

### Task 3: Register `use_skill` tool in agent

**Files:**
- Modify: `pkg/agent/agent.go:60` (after `registry.Register(tools.NewLsTool(...))`)

**Step 1: Add tool registration**

Add one line after the existing tool registrations (after line 59):

```go
registry.Register(tools.NewUseSkillTool(loadedSkills))
```

Note: `loadedSkills` is already available at this point (line 61). Move the skill loading block (lines 61-64) above the tool registrations so `loadedSkills` is available, then register the tool.

Reorder `pkg/agent/agent.go` NewAgent function so:
1. Load skills (lines 61-64 moved up, before tool registrations)
2. Register all tools including `use_skill` with `loadedSkills`
3. Build system prompt with skills

**Step 2: Run full test suite**

Run: `make test`
Expected: PASS

**Step 3: Run lint**

Run: `make lint`
Expected: PASS

**Step 4: Commit**

```bash
git add pkg/agent/agent.go
git commit -m "feat: register use_skill tool in agent"
```

---

### Task 4: Manual smoke test

**Step 1: Build and run**

Run: `make build && ./pigo`

**Step 2: Verify tool appears in tool list**

The startup banner should show `use_skill` in the Tools list.

**Step 3: Verify `/skills` still works**

Type `/skills` — should list loaded skills as before.

**Step 4: Verify `/skill:name` still works**

Type `/skill:<any-loaded-skill>` — should expand and work as before.
