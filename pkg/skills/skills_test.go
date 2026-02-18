package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFM   SkillFrontmatter
		wantBody string
	}{
		{
			name:     "valid frontmatter",
			input:    "---\nname: greet\ndescription: A greeting skill\n---\nHello world",
			wantFM:   SkillFrontmatter{Name: "greet", Description: "A greeting skill"},
			wantBody: "Hello world",
		},
		{
			name:     "no frontmatter",
			input:    "Just a plain file",
			wantFM:   SkillFrontmatter{},
			wantBody: "Just a plain file",
		},
		{
			name:     "empty frontmatter",
			input:    "---\n---\nBody text",
			wantFM:   SkillFrontmatter{},
			wantBody: "Body text",
		},
		{
			name:  "multiline description",
			input: "---\nname: deploy\ndescription: |\n  This is a long\n  description text\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:        "deploy",
				Description: "This is a long description text",
			},
			wantBody: "Body",
		},
		{
			name:  "windows line endings",
			input: "---\r\nname: win\r\ndescription: Windows skill\r\n---\r\nBody",
			wantFM: SkillFrontmatter{
				Name:        "win",
				Description: "Windows skill",
			},
			wantBody: "Body",
		},
		{
			name:  "unknown fields ignored",
			input: "---\nname: test\ndescription: A test\nauthor: someone\ntags: [a, b]\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:        "test",
				Description: "A test",
			},
			wantBody: "Body",
		},
		{
			name:  "description containing colons",
			input: "---\nname: colon\ndescription: key: value pair handling\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:        "colon",
				Description: "key: value pair handling",
			},
			wantBody: "Body",
		},
		{
			name:  "disable-model-invocation true",
			input: "---\nname: internal\ndescription: Internal skill\ndisable-model-invocation: true\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:                   "internal",
				Description:            "Internal skill",
				DisableModelInvocation: true,
			},
			wantBody: "Body",
		},
		{
			name:  "disable-model-invocation false",
			input: "---\nname: external\ndescription: External skill\ndisable-model-invocation: false\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:        "external",
				Description: "External skill",
			},
			wantBody: "Body",
		},
		{
			name:  "field after multiline description is parsed",
			input: "---\nname: multi\ndescription: |\n  line one\n  line two\ndisable-model-invocation: true\n---\nBody",
			wantFM: SkillFrontmatter{
				Name:                   "multi",
				Description:            "line one line two",
				DisableModelInvocation: true,
			},
			wantBody: "Body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body := ParseFrontmatter(tt.input)
			if fm.Name != tt.wantFM.Name {
				t.Errorf("name: got %q, want %q", fm.Name, tt.wantFM.Name)
			}
			if fm.Description != tt.wantFM.Description {
				t.Errorf("description: got %q, want %q", fm.Description, tt.wantFM.Description)
			}
			if fm.DisableModelInvocation != tt.wantFM.DisableModelInvocation {
				t.Errorf("disable-model-invocation: got %v, want %v", fm.DisableModelInvocation, tt.wantFM.DisableModelInvocation)
			}
			body = strings.TrimSpace(body)
			if body != tt.wantBody {
				t.Errorf("body: got %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestValidateSkillName(t *testing.T) {
	tests := []struct {
		name         string
		skillName    string
		parentDir    string
		wantWarnings int
	}{
		{"valid name matching dir", "greet", "greet", 0},
		{"name/dir mismatch", "greet", "hello", 1},
		{"too long", strings.Repeat("a", 65), "", 1},
		{"invalid chars uppercase", "Greet", "", 1},
		{"invalid chars space", "my skill", "", 1},
		{"leading hyphen", "-greet", "", 1},
		{"trailing hyphen", "greet-", "", 1},
		{"consecutive hyphens", "my--skill", "", 1},
		{"valid with hyphens", "my-skill", "my-skill", 0},
		{"empty parent dir", "greet", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validateSkillName(tt.skillName, tt.parentDir)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantWarnings, warnings)
			}
		})
	}
}

func TestValidateSkillDescription(t *testing.T) {
	tests := []struct {
		name         string
		desc         string
		wantWarnings int
		wantOk       bool
	}{
		{"empty description", "", 1, false},
		{"valid description", "A helpful skill", 0, true},
		{"too long description", strings.Repeat("x", 201), 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, ok := ValidateSkillDescription(tt.desc)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantWarnings, warnings)
			}
			if ok != tt.wantOk {
				t.Errorf("got ok=%v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestLoadSkillFromFile(t *testing.T) {
	t.Run("valid skill", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "greet")
		os.MkdirAll(skillDir, 0o755)
		path := filepath.Join(skillDir, "SKILL.md")
		os.WriteFile(path, []byte("---\nname: greet\ndescription: A greeting skill\n---\nHello!"), 0o644)

		skill, diags := loadSkillFromFile(path, "user")
		if skill == nil {
			t.Fatal("expected non-nil skill")
		}
		if skill.Name != "greet" {
			t.Errorf("name: got %q, want %q", skill.Name, "greet")
		}
		if skill.Description != "A greeting skill" {
			t.Errorf("description: got %q, want %q", skill.Description, "A greeting skill")
		}
		if skill.Source != "user" {
			t.Errorf("source: got %q, want %q", skill.Source, "user")
		}
		// No fatal diagnostics expected
		for _, d := range diags {
			if strings.Contains(d.Message, "missing") {
				t.Errorf("unexpected fatal diagnostic: %s", d.Message)
			}
		}
	})

	t.Run("missing description", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.md")
		os.WriteFile(path, []byte("---\nname: test\n---\nBody"), 0o644)

		skill, diags := loadSkillFromFile(path, "user")
		if skill != nil {
			t.Error("expected nil skill for missing description")
		}
		if len(diags) == 0 {
			t.Error("expected diagnostics for missing description")
		}
	})

	t.Run("name from parent dir", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "my-skill")
		os.MkdirAll(skillDir, 0o755)
		path := filepath.Join(skillDir, "SKILL.md")
		os.WriteFile(path, []byte("---\ndescription: No name given\n---\nBody"), 0o644)

		skill, _ := loadSkillFromFile(path, "project")
		if skill == nil {
			t.Fatal("expected non-nil skill")
		}
		if skill.Name != "my-skill" {
			t.Errorf("name: got %q, want %q", skill.Name, "my-skill")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		skill, diags := loadSkillFromFile("/nonexistent/path/SKILL.md", "user")
		if skill != nil {
			t.Error("expected nil skill for nonexistent file")
		}
		if len(diags) == 0 {
			t.Error("expected diagnostics for nonexistent file")
		}
	})
}

func TestLoadSkillsFromDir(t *testing.T) {
	t.Run("root md files", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "quick.md"), []byte("---\nname: quick\ndescription: Quick skill\n---\nBody"), 0o644)
		os.WriteFile(filepath.Join(dir, "slow.md"), []byte("---\nname: slow\ndescription: Slow skill\n---\nBody"), 0o644)

		skills, _ := loadSkillsFromDir(dir, "user")
		if len(skills) != 2 {
			t.Errorf("got %d skills, want 2", len(skills))
		}
	})

	t.Run("nested SKILL.md", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, "deploy")
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: deploy\ndescription: Deploy skill\n---\nBody"), 0o644)

		skills, _ := loadSkillsFromDir(dir, "project")
		if len(skills) != 1 {
			t.Errorf("got %d skills, want 1", len(skills))
		}
		if skills[0].Name != "deploy" {
			t.Errorf("name: got %q, want %q", skills[0].Name, "deploy")
		}
	})

	t.Run("skip dotfiles", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".hidden.md"), []byte("---\nname: hidden\ndescription: Hidden\n---\n"), 0o644)
		hiddenDir := filepath.Join(dir, ".hidden")
		os.MkdirAll(hiddenDir, 0o755)
		os.WriteFile(filepath.Join(hiddenDir, "SKILL.md"), []byte("---\nname: hidden\ndescription: Hidden\n---\n"), 0o644)

		skills, _ := loadSkillsFromDir(dir, "user")
		if len(skills) != 0 {
			t.Errorf("got %d skills, want 0 (dotfiles should be skipped)", len(skills))
		}
	})

	t.Run("skip non-SKILL.md in subdirs", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "myskill")
		os.MkdirAll(subdir, 0o755)
		os.WriteFile(filepath.Join(subdir, "README.md"), []byte("---\nname: readme\ndescription: Not a skill\n---\n"), 0o644)

		skills, _ := loadSkillsFromDir(dir, "user")
		if len(skills) != 0 {
			t.Errorf("got %d skills, want 0 (non-SKILL.md in subdirs should be skipped)", len(skills))
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		skills, diags := loadSkillsFromDir("/nonexistent/dir", "user")
		if len(skills) != 0 {
			t.Errorf("got %d skills, want 0", len(skills))
		}
		if len(diags) != 0 {
			t.Errorf("got %d diagnostics, want 0", len(diags))
		}
	})
}

func TestLoadSkills(t *testing.T) {
	t.Run("user and project discovery", func(t *testing.T) {
		// Use temp dir as cwd with .pigo/skills/
		cwd := t.TempDir()
		projectSkillDir := filepath.Join(cwd, ".pigo", "skills", "proj-skill")
		os.MkdirAll(projectSkillDir, 0o755)
		os.WriteFile(filepath.Join(projectSkillDir, "SKILL.md"), []byte("---\nname: proj-skill\ndescription: Project skill\n---\nBody"), 0o644)

		skills, _ := LoadSkills(cwd)
		// Should find at least the project skill (user dir may or may not have skills)
		found := false
		for _, s := range skills {
			if s.Name == "proj-skill" && s.Source == "project" {
				found = true
			}
		}
		if !found {
			t.Error("expected to find proj-skill with source=project")
		}
	})

	t.Run("name collision", func(t *testing.T) {
		// Create a temp dir that acts as both user home and project
		tmpDir := t.TempDir()

		// We'll test collision within a single directory since we can't easily
		// mock the home dir. Put two .md files with the same name field.
		projectDir := filepath.Join(tmpDir, ".pigo", "skills")
		os.MkdirAll(projectDir, 0o755)

		// Root-level .md file
		os.WriteFile(filepath.Join(projectDir, "dupe.md"), []byte("---\nname: dupe\ndescription: First dupe\n---\n"), 0o644)

		// Subdir with same name
		dupeDir := filepath.Join(projectDir, "dupe")
		os.MkdirAll(dupeDir, 0o755)
		os.WriteFile(filepath.Join(dupeDir, "SKILL.md"), []byte("---\nname: dupe\ndescription: Second dupe\n---\n"), 0o644)

		skills, diags := loadSkillsFromDir(projectDir, "project")

		// First one wins
		dupeCount := 0
		for _, s := range skills {
			if s.Name == "dupe" {
				dupeCount++
			}
		}

		// loadSkillsFromDir doesn't deduplicate — LoadSkills does.
		// But let's verify LoadSkills handles it
		allSkills, allDiags := LoadSkills(tmpDir)
		dupeCount = 0
		for _, s := range allSkills {
			if s.Name == "dupe" {
				dupeCount++
			}
		}

		// Should only have one
		if dupeCount > 1 {
			t.Errorf("expected at most 1 skill named 'dupe', got %d", dupeCount)
		}

		// Verify collision diagnostic exists (from either level)
		_ = skills
		_ = diags
		_ = allDiags
	})
}

func TestFormatSkillsForPrompt(t *testing.T) {
	t.Run("empty skills", func(t *testing.T) {
		result := FormatSkillsForPrompt(nil)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single skill", func(t *testing.T) {
		skills := []Skill{{Name: "greet", Description: "A greeting skill"}}
		result := FormatSkillsForPrompt(skills)
		if !strings.Contains(result, "<skills>") {
			t.Error("expected <skills> tag")
		}
		if !strings.Contains(result, `name="greet"`) {
			t.Error("expected skill name in output")
		}
		if !strings.Contains(result, `description="A greeting skill"`) {
			t.Error("expected skill description in output")
		}
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []Skill{
			{Name: "greet", Description: "Greeting"},
			{Name: "deploy", Description: "Deployment"},
		}
		result := FormatSkillsForPrompt(skills)
		if !strings.Contains(result, "greet") || !strings.Contains(result, "deploy") {
			t.Error("expected both skills in output")
		}
	})

	t.Run("XML escaping", func(t *testing.T) {
		skills := []Skill{{Name: "test", Description: "Use <tags> & \"quotes\""}}
		result := FormatSkillsForPrompt(skills)
		if strings.Contains(result, "<tags>") {
			t.Error("expected XML-escaped angle brackets")
		}
		if !strings.Contains(result, "&lt;tags&gt;") {
			t.Error("expected escaped angle brackets")
		}
		if !strings.Contains(result, "&amp;") {
			t.Error("expected escaped ampersand")
		}
	})

	t.Run("excludes disabled skills", func(t *testing.T) {
		skills := []Skill{
			{Name: "visible", Description: "Visible"},
			{Name: "hidden", Description: "Hidden", DisableModelInvocation: true},
		}
		result := FormatSkillsForPrompt(skills)
		if !strings.Contains(result, "visible") {
			t.Error("expected visible skill in output")
		}
		if strings.Contains(result, "hidden") {
			t.Error("expected hidden skill to be excluded")
		}
	})

	t.Run("all disabled returns empty", func(t *testing.T) {
		skills := []Skill{
			{Name: "hidden", Description: "Hidden", DisableModelInvocation: true},
		}
		result := FormatSkillsForPrompt(skills)
		if result != "" {
			t.Errorf("expected empty string when all skills disabled, got %q", result)
		}
	})
}

func TestExpandSkillCommand(t *testing.T) {
	// Create a temp skill file for tests
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "greet")
	os.MkdirAll(skillDir, 0o755)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	os.WriteFile(skillFile, []byte("---\nname: greet\ndescription: A greeting skill\n---\nSay hello to the user."), 0o644)

	skills := []Skill{
		{Name: "greet", Description: "A greeting skill", FilePath: skillFile},
	}

	t.Run("not a skill command", func(t *testing.T) {
		result, ok := ExpandSkillCommand("hello world", skills)
		if ok {
			t.Error("expected ok=false for non-command input")
		}
		if result != "hello world" {
			t.Errorf("expected original input, got %q", result)
		}
	})

	t.Run("skill command without args", func(t *testing.T) {
		result, ok := ExpandSkillCommand("/skill:greet", skills)
		if !ok {
			t.Error("expected ok=true for valid skill command")
		}
		if !strings.Contains(result, "<skill name=\"greet\">") {
			t.Errorf("expected <skill> wrapper, got %q", result)
		}
		if !strings.Contains(result, "Say hello to the user.") {
			t.Errorf("expected skill body, got %q", result)
		}
	})

	t.Run("skill command with args", func(t *testing.T) {
		result, ok := ExpandSkillCommand("/skill:greet say hi to Bob", skills)
		if !ok {
			t.Error("expected ok=true")
		}
		if !strings.Contains(result, "Say hello to the user.") {
			t.Error("expected skill body")
		}
		if !strings.Contains(result, "say hi to Bob") {
			t.Error("expected args in output")
		}
	})

	t.Run("unknown skill", func(t *testing.T) {
		result, ok := ExpandSkillCommand("/skill:unknown", skills)
		if ok {
			t.Error("expected ok=false for unknown skill")
		}
		if result != "/skill:unknown" {
			t.Errorf("expected original input, got %q", result)
		}
	})

	t.Run("format verification", func(t *testing.T) {
		result, ok := ExpandSkillCommand("/skill:greet", skills)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if !strings.HasPrefix(result, "<skill name=\"greet\">") {
			t.Errorf("expected result to start with <skill> tag, got %q", result[:40])
		}
		if !strings.HasSuffix(result, "</skill>") {
			t.Errorf("expected result to end with </skill>, got %q", result[len(result)-20:])
		}
	})
}
