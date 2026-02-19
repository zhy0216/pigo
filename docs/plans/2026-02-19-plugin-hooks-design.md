# Plugin Hooks System Design

## Overview

Lightweight plugin hooks system for pigo. Each plugin defines event hooks that run bash commands with environment variable interpolation. No Go code required — users only edit `config.json`.

## Configuration

Plugins are defined as an array in `~/.pigo/config.json`:

```json
{
  "api_key": "sk-...",
  "model": "gpt-4o",
  "plugins": [
    {
      "name": "safety-gate",
      "enabled": true,
      "hooks": {
        "tool_start": [
          {
            "command": "check-safety.sh",
            "match": { "tool": "bash" },
            "blocking": true,
            "timeout": 5
          }
        ]
      }
    },
    {
      "name": "logging",
      "enabled": true,
      "hooks": {
        "tool_end": [
          { "command": "log-tool.sh", "blocking": false }
        ],
        "agent_end": [
          { "command": "notify-send 'pigo done'" }
        ]
      }
    }
  ]
}
```

### Plugin Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique plugin name |
| `enabled` | bool | no | `true` | Enable/disable the plugin |
| `hooks` | map | yes | — | Map of event name → hook array |

### Hook Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `command` | string | yes | — | Bash command to execute |
| `match` | object | no | match all | Filter condition, e.g. `{"tool": "bash"}` |
| `blocking` | bool | no | `true` | Wait for completion before continuing |
| `timeout` | int | no | `10` | Timeout in seconds (blocking only) |

### Match Rules

- `{"tool": "bash"}` — exact tool name match
- `{"tool": "bash|write"}` — match multiple tools with `|` separator
- No match field → hook runs for all events of that type

## Event Types

| Event | Trigger | Can Intercept |
|-------|---------|---------------|
| `agent_start` | Agent begins processing user input | No |
| `turn_start` | Before each LLM call | No |
| `tool_start` | Before tool execution | Yes (blocking hook exit != 0 cancels) |
| `tool_end` | After tool execution | No |
| `turn_end` | After each turn completes | No |
| `agent_end` | Agent finishes all processing | No |

## Environment Variables

All variables use `PIGO_` prefix. Injected into hook subprocess environment.

### Common (all events)

| Variable | Description |
|----------|-------------|
| `PIGO_EVENT` | Event name (e.g. `tool_start`) |
| `PIGO_WORK_DIR` | Current working directory |
| `PIGO_MODEL` | Current model name |
| `PIGO_USER_MESSAGE` | Last user message content |
| `PIGO_ASSISTANT_MESSAGE` | Last assistant message content |

### tool_start additional

| Variable | Description |
|----------|-------------|
| `PIGO_TOOL_NAME` | Tool name (e.g. `bash`, `read`, `edit`) |
| `PIGO_TOOL_ARGS` | Tool arguments as JSON string |
| `PIGO_TOOL_INPUT` | For bash tool, extracted `command` argument value |

### tool_end additional

| Variable | Description |
|----------|-------------|
| `PIGO_TOOL_NAME` | Tool name |
| `PIGO_TOOL_ARGS` | Tool arguments as JSON |
| `PIGO_TOOL_OUTPUT` | Tool output (truncated to 10K chars) |
| `PIGO_TOOL_ERROR` | `"true"` or `"false"` |

### turn_start / turn_end additional

| Variable | Description |
|----------|-------------|
| `PIGO_TURN_NUMBER` | Current turn number |

## Execution

### Flow

1. Event triggers in agent loop
2. Iterate through plugins array (in order)
3. For each enabled plugin, find hooks for this event
4. For each hook, check match condition
5. If match passes (or no match rule), execute command

### Blocking Behavior

- `blocking: true` (default): Wait for command to complete
  - For `tool_start`: exit 0 → continue, exit != 0 → cancel tool, return stderr to LLM
  - For other events: exit != 0 → log warning, continue
- `blocking: false`: Fire and forget, don't wait

### Timeout

- Kill subprocess after `timeout` seconds
- Timeout treated as failure (same as exit 1)

### Error Handling

- Blocking hook failure: log stderr, cancel tool if `tool_start`
- Non-blocking hook failure: silently log
- Config parse errors: warn at startup, don't block agent

## Code Architecture

### New Package: `pkg/hooks/`

```
pkg/hooks/
  hooks.go        # HookManager, types, load/match/execute logic
  hooks_test.go   # Unit tests
```

### Core Types

```go
type HookConfig struct {
    Command  string     `json:"command"`
    Match    *MatchRule `json:"match,omitempty"`
    Blocking *bool      `json:"blocking,omitempty"` // default true
    Timeout  int        `json:"timeout,omitempty"`  // default 10s
}

type MatchRule struct {
    Tool string `json:"tool,omitempty"`
}

type PluginConfig struct {
    Name    string                  `json:"name"`
    Enabled *bool                   `json:"enabled,omitempty"` // default true
    Hooks   map[string][]HookConfig `json:"hooks"`
}

type HookManager struct {
    plugins []PluginConfig
}

type HookContext struct {
    Event            string
    WorkDir          string
    Model            string
    UserMessage      string
    AssistantMessage string
    ToolName         string
    ToolArgs         string
    ToolInput        string
    ToolOutput       string
    ToolError        bool
    TurnNumber       int
}
```

### Integration Points

1. `pkg/config/config.go` — Add `Plugins []PluginConfig` to `Config` struct
2. `pkg/agent/agent.go` — Add `hookMgr *hooks.HookManager` to `Agent` struct
3. Insert `hookMgr.Run()` calls at each event point in `ProcessInput()`
4. For `tool_start` blocking hooks, check return value to skip tool execution

### No Changes To

- `EventEmitter` — hooks system is independent, called directly from agent loop
- Existing tool implementations
- LLM client
