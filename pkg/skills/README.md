# pkg/skills

Skill discovery, parsing, validation, and expansion. Skills are Markdown files with YAML frontmatter that define named, reusable instruction sets invokable via `/skill:<name>` commands or surfaced to the model via the system prompt.

## Skill File Formats

- Single `.md` files at the root of a skills directory
- `SKILL.md` inside a named subdirectory

## Skill Directories

Scanned in priority order (user skills take precedence):

1. `~/.pigo/skills/` — user skills
2. `<cwd>/.pigo/skills/` — project skills

## Key Types

| Type | Description |
|---|---|
| `Skill` | A fully loaded skill: name, description, file path, source, model visibility flag |
| `SkillFrontmatter` | Parsed YAML frontmatter: name, description, disable-model-invocation |
| `SkillDiagnostic` | Non-fatal warning or error from skill loading |

## API

| Function | Description |
|---|---|
| `LoadSkills(cwd) ([]Skill, []SkillDiagnostic)` | Discovers and loads skills from user and project directories |
| `ParseFrontmatter(content) (SkillFrontmatter, body)` | Parses YAML frontmatter from a Markdown string |
| `ValidateSkillDescription(desc) (warnings, ok)` | Validates a skill's description (empty = hard fail, >200 chars = warning) |
| `FormatSkillsForPrompt(skills) string` | Generates an XML block of skills for the system prompt |
| `ExpandSkillCommand(input, skills) (expanded, ok)` | Expands `/skill:<name> [args]` commands by injecting skill file content |

## Frontmatter Keys

| Key | Type | Description |
|---|---|---|
| `name` | string | Skill name (overrides directory/filename) |
| `description` | string | Human/model-readable description (required) |
| `disable-model-invocation` | bool | If true, hidden from LLM prompt but still invokable via `/skill:` |

## Name Resolution Order

1. Frontmatter `name:` field
2. Parent directory name (for `SKILL.md`)
3. Filename without `.md`

## Name Rules

- Must match `^[a-z0-9-]+$`
- Maximum 64 characters
- No leading, trailing, or consecutive hyphens
