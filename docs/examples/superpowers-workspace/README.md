# Superpowers Workspace Example

A reference pigo workspace with curated development skills and a conversation logging plugin.

## Quick Start

Copy the `.pigo/` directory and `scripts/` into your project root:

```bash
cp -r .pigo/ /path/to/your/project/.pigo/
cp -r scripts/ /path/to/your/project/scripts/
```

Set your API key in `.pigo/config.json` or via environment variable:

```bash
export OPENAI_API_KEY="sk-..."
```

Run pigo from your project root:

```bash
pigo
```

## What's Included

### Config (`.pigo/config.json`)

Minimal configuration pointing to the OpenAI API with the `conversation-logger` plugin enabled. Edit `api_key`, `base_url`, and `model` to match your setup.

### Log Plugin (`.pigo/plugins.json` + `scripts/log-conversation.sh`)

The `conversation-logger` plugin hooks into four events and writes JSONL to `.pigo/logs/conversation.jsonl`:

| Event | What's Logged |
|-------|---------------|
| `turn_start` | User message |
| `turn_end` | Assistant response |
| `tool_start` | Tool name and input |
| `tool_end` | Tool name, output, and errors |

All hooks are non-blocking so they don't slow down the agent loop. Log entries include UTC timestamps.

**Viewing logs:**

```bash
# Follow live
tail -f .pigo/logs/conversation.jsonl

# Pretty-print with jq
cat .pigo/logs/conversation.jsonl | jq .

# Filter by event type
cat .pigo/logs/conversation.jsonl | jq 'select(.event == "tool_end")'
```

### Skills (`.pigo/skills/`)

Five curated skills covering the full development lifecycle:

| Skill | When to Use |
|-------|-------------|
| **brainstorming** | Before any creative work — explores intent, requirements, and design |
| **writing-plans** | When you have requirements for a multi-step task, before coding |
| **test-driven-development** | When implementing any feature or bugfix |
| **systematic-debugging** | When encountering any bug, test failure, or unexpected behavior |
| **verification-before-completion** | Before claiming work is complete or creating PRs |

Invoke skills with:

```
/skill:brainstorming
/skill:test-driven-development
```

Or let the model invoke them automatically via the `use_skill` tool.

## Customizing

### Add more skills

Drop `.md` files or `<name>/SKILL.md` directories into `.pigo/skills/`. See `docs/reference/superpowers/skills/` for the full set of available superpowers skills.

### Modify the log plugin

Edit `scripts/log-conversation.sh` to change log format, destination, or filtering. Edit `.pigo/plugins.json` to add hook conditions:

```json
{
  "command": "./scripts/log-conversation.sh",
  "match": { "tool": "bash|write" },
  "blocking": false
}
```

### Add new plugins

Define new plugins in `.pigo/plugins.json` and reference them in `config.json`'s `plugins` array. See `pkg/hooks/hooks.go` for the full hook API.

## Directory Structure

```
.pigo/
├── config.json              # API config + enabled plugins
├── plugins.json             # Plugin definitions with hooks
└── skills/
    ├── brainstorming/SKILL.md
    ├── systematic-debugging/SKILL.md
    ├── test-driven-development/SKILL.md
    ├── verification-before-completion/SKILL.md
    └── writing-plans/SKILL.md
scripts/
└── log-conversation.sh      # Logging script called by plugin hooks
```
