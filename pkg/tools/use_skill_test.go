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
	if strings.Contains(result.ForLLM, "description: Deploy the app") {
		t.Error("frontmatter should be stripped from output")
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
	if !strings.Contains(result.ForLLM, "name is required") {
		t.Errorf("expected 'name is required' error, got: %s", result.ForLLM)
	}
}

func TestUseSkillTool_EmptyName(t *testing.T) {
	tool := NewUseSkillTool(nil)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"name": "",
	})

	if !result.IsError {
		t.Error("expected error for empty name")
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
