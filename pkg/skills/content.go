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
