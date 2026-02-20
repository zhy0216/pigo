package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zhy0216/pigo/pkg/skills"
	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
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
	name, err := util.ExtractString(args, "name")
	if err != nil {
		return types.ErrorResult(err.Error())
	}
	if name == "" {
		return types.ErrorResult("name must not be empty")
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
