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
