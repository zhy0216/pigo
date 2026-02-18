package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/user/pigo/pkg/util"
)

// Skill represents a loaded skill definition.
type Skill struct {
	Name                   string
	Description            string
	FilePath               string
	BaseDir                string
	Source                 string // "user" or "project"
	DisableModelInvocation bool
}

// SkillFrontmatter holds parsed YAML frontmatter fields.
type SkillFrontmatter struct {
	Name                   string
	Description            string
	DisableModelInvocation bool
}

// SkillDiagnostic represents a warning or error from skill loading.
type SkillDiagnostic struct {
	Message string
	Path    string
}

var skillNameRegexp = regexp.MustCompile(`^[a-z0-9-]+$`)

// ParseFrontmatter extracts YAML frontmatter from a markdown file.
// It handles the three fields we care about: name, description, disable-model-invocation.
// It supports multiline description values using YAML block scalar (|).
func ParseFrontmatter(content string) (SkillFrontmatter, string) {
	var fm SkillFrontmatter

	content = strings.ReplaceAll(content, "\r\n", "\n")

	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return fm, content
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return fm, content
	}

	for i := 1; i < endIdx; i++ {
		line := lines[i]

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "name":
			fm.Name = value
		case "description":
			if value == "|" {
				var descLines []string
				for j := i + 1; j < endIdx; j++ {
					nextLine := lines[j]
					if len(nextLine) > 0 && (nextLine[0] == ' ' || nextLine[0] == '\t') {
						descLines = append(descLines, strings.TrimSpace(nextLine))
						i = j
					} else {
						break
					}
				}
				fm.Description = strings.Join(descLines, " ")
			} else {
				fm.Description = value
			}
		case "disable-model-invocation":
			fm.DisableModelInvocation = value == "true"
		}
	}

	body := strings.Join(lines[endIdx+1:], "\n")
	return fm, body
}

// validateSkillName returns warnings for invalid skill names.
func validateSkillName(name, parentDirName string) []string {
	var warnings []string

	if name != parentDirName && parentDirName != "" {
		warnings = append(warnings, fmt.Sprintf("skill name %q does not match directory name %q", name, parentDirName))
	}

	if len(name) > 64 {
		warnings = append(warnings, fmt.Sprintf("skill name %q exceeds 64 characters", name))
	}

	if !skillNameRegexp.MatchString(name) {
		warnings = append(warnings, fmt.Sprintf("skill name %q contains invalid characters (must be lowercase alphanumeric and hyphens)", name))
	}

	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		warnings = append(warnings, fmt.Sprintf("skill name %q has leading or trailing hyphens", name))
	}

	if strings.Contains(name, "--") {
		warnings = append(warnings, fmt.Sprintf("skill name %q contains consecutive hyphens", name))
	}

	return warnings
}

// ValidateSkillDescription returns warnings and whether the description is acceptable.
// A missing description is the only hard failure (ok=false).
func ValidateSkillDescription(desc string) ([]string, bool) {
	if desc == "" {
		return []string{"skill is missing a description"}, false
	}

	var warnings []string
	if len(desc) > 200 {
		warnings = append(warnings, "skill description exceeds 200 characters")
	}

	return warnings, true
}

// loadSkillFromFile reads and parses a single skill file.
// Returns nil if the skill should not be loaded (e.g., missing description).
func loadSkillFromFile(filePath, source string) (*Skill, []SkillDiagnostic) {
	var diagnostics []SkillDiagnostic

	content, err := os.ReadFile(filePath)
	if err != nil {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Message: fmt.Sprintf("failed to read skill file: %v", err),
			Path:    filePath,
		})
		return nil, diagnostics
	}

	fm, _ := ParseFrontmatter(string(content))

	name := fm.Name
	parentDir := filepath.Base(filepath.Dir(filePath))
	if name == "" {
		if filepath.Base(filePath) == "SKILL.md" {
			name = parentDir
		} else {
			name = strings.TrimSuffix(filepath.Base(filePath), ".md")
		}
	}

	nameWarnings := validateSkillName(name, parentDir)
	for _, w := range nameWarnings {
		diagnostics = append(diagnostics, SkillDiagnostic{Message: w, Path: filePath})
	}

	descWarnings, ok := ValidateSkillDescription(fm.Description)
	for _, w := range descWarnings {
		diagnostics = append(diagnostics, SkillDiagnostic{Message: w, Path: filePath})
	}
	if !ok {
		return nil, diagnostics
	}

	return &Skill{
		Name:                   name,
		Description:            fm.Description,
		FilePath:               filePath,
		BaseDir:                filepath.Dir(filePath),
		Source:                 source,
		DisableModelInvocation: fm.DisableModelInvocation,
	}, diagnostics
}

// loadSkillsFromDir scans a directory for skill files.
// Root level: any .md files. Subdirectories: only SKILL.md.
// Skips dotfiles/dotdirs, node_modules, vendor.
func loadSkillsFromDir(dir, source string) ([]Skill, []SkillDiagnostic) {
	var skills []Skill
	var diagnostics []SkillDiagnostic

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}

	for _, entry := range entries {
		name := entry.Name()

		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}

		fullPath := filepath.Join(dir, name)

		if entry.IsDir() {
			skillFile := filepath.Join(fullPath, "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				skill, diags := loadSkillFromFile(skillFile, source)
				diagnostics = append(diagnostics, diags...)
				if skill != nil {
					skills = append(skills, *skill)
				}
			}
		} else if strings.HasSuffix(name, ".md") {
			skill, diags := loadSkillFromFile(fullPath, source)
			diagnostics = append(diagnostics, diags...)
			if skill != nil {
				skills = append(skills, *skill)
			}
		}
	}

	return skills, diagnostics
}

// LoadSkills discovers skills from user (~/.pigo/skills/) and project (.pigo/skills/) directories.
// User skills are loaded first; on name collision, the first one wins.
func LoadSkills(cwd string) ([]Skill, []SkillDiagnostic) {
	var allSkills []Skill
	var diagnostics []SkillDiagnostic
	seen := make(map[string]bool)

	homeDir, err := os.UserHomeDir()
	if err == nil {
		userDir := filepath.Join(homeDir, ".pigo", "skills")
		skills, diags := loadSkillsFromDir(userDir, "user")
		diagnostics = append(diagnostics, diags...)
		for _, s := range skills {
			if !seen[s.Name] {
				seen[s.Name] = true
				allSkills = append(allSkills, s)
			}
		}
	}

	projectDir := filepath.Join(cwd, ".pigo", "skills")
	skills, diags := loadSkillsFromDir(projectDir, "project")
	diagnostics = append(diagnostics, diags...)
	for _, s := range skills {
		if seen[s.Name] {
			diagnostics = append(diagnostics, SkillDiagnostic{
				Message: fmt.Sprintf("skill %q already loaded (name collision), skipping", s.Name),
				Path:    s.FilePath,
			})
			continue
		}
		seen[s.Name] = true
		allSkills = append(allSkills, s)
	}

	return allSkills, diagnostics
}

// FormatSkillsForPrompt generates an XML block listing available skills for the system prompt.
// Skills with DisableModelInvocation=true are excluded.
func FormatSkillsForPrompt(skills []Skill) string {
	var visible []Skill
	for _, s := range skills {
		if !s.DisableModelInvocation {
			visible = append(visible, s)
		}
	}

	if len(visible) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n<skills>\n")
	for _, s := range visible {
		b.WriteString(fmt.Sprintf("  <skill name=%q description=%q />\n",
			util.XmlEscape(s.Name), util.XmlEscape(s.Description)))
	}
	b.WriteString("</skills>")
	return b.String()
}

// ExpandSkillCommand detects /skill:name [args] input, reads the skill file,
// strips frontmatter, and wraps content in a <skill> XML block.
// Returns (expanded, true) if a skill command was matched and expanded,
// or (original, false) otherwise.
func ExpandSkillCommand(input string, skills []Skill) (string, bool) {
	if !strings.HasPrefix(input, "/skill:") {
		return input, false
	}

	rest := input[len("/skill:"):]
	parts := strings.SplitN(rest, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	var skill *Skill
	for i := range skills {
		if skills[i].Name == name {
			skill = &skills[i]
			break
		}
	}
	if skill == nil {
		return input, false
	}

	content, err := os.ReadFile(skill.FilePath)
	if err != nil {
		return input, false
	}

	_, body := ParseFrontmatter(string(content))
	body = strings.TrimSpace(body)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<skill name=%q>\n", name))
	b.WriteString(body)
	b.WriteString("\n</skill>")
	if args != "" {
		b.WriteString("\n\n")
		b.WriteString(args)
	}

	return b.String(), true
}
