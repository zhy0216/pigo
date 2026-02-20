# Superpowers Workspace Example

## Goal

Create a reference example workspace in `docs/examples/superpowers-workspace/` that demonstrates how to set up a pigo project with skills and a log plugin. Users can copy the `.pigo/` directory into their own projects.

## Structure

```
docs/examples/superpowers-workspace/
├── README.md
├── .pigo/
│   ├── config.json
│   ├── plugins.json
│   └── skills/
│       ├── brainstorming/SKILL.md
│       ├── test-driven-development/SKILL.md
│       ├── systematic-debugging/SKILL.md
│       ├── writing-plans/SKILL.md
│       └── verification-before-completion/SKILL.md
└── scripts/
    └── log-conversation.sh
```

## Components

### config.json

Minimal config referencing the conversation-logger plugin.

### plugins.json

Defines `conversation-logger` plugin with non-blocking hooks on `turn_start`, `turn_end`, `tool_start`, and `tool_end` events. All hooks call `./scripts/log-conversation.sh`.

### log-conversation.sh

Shell script that reads PIGO_* environment variables and appends JSONL entries to `.pigo/logs/conversation.jsonl`. Creates log directory on first run.

### Skills (curated subset of 5)

Copied from `docs/reference/superpowers/skills/`:

1. **brainstorming** — Design before implementation
2. **writing-plans** — Plan before coding
3. **test-driven-development** — Test before implementation
4. **systematic-debugging** — Debug methodically
5. **verification-before-completion** — Verify before claiming done

### README.md

Explains the workspace, how to use it, what each component does, and how to customize.
