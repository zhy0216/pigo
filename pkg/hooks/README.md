# pkg/hooks

Event-driven plugin hook system. Plugins define hooks that run shell commands in response to agent lifecycle events, enabling tool interception, logging, and custom workflows.

## Key Types

### PluginConfig

A named plugin with a map of event hooks.

| Field | Type | Description |
|---|---|---|
| `Name` | string | Plugin name |
| `Enabled` | *bool | Defaults to true if nil; set to false to disable |
| `Hooks` | map[string][]HookConfig | Event name to hook list |

### HookConfig

A single hook definition.

| Field | Type | Description |
|---|---|---|
| `Command` | string | Shell command to execute |
| `Match` | *MatchRule | Optional conditions for when the hook runs |
| `Blocking` | *bool | Defaults to true (blocking); set to false for async |
| `Timeout` | int | Timeout in seconds (default: 10) |

### MatchRule

| Field | Type | Description |
|---|---|---|
| `Tool` | string | Pipe-separated tool names (e.g., `"bash\|write"`) |

### HookContext

Event context passed to hooks via environment variables.

| Field | Env Variable | When Set |
|---|---|---|
| `Event` | `PIGO_EVENT` | Always |
| `WorkDir` | `PIGO_WORK_DIR` | Always |
| `Model` | `PIGO_MODEL` | Always |
| `UserMessage` | `PIGO_USER_MESSAGE` | Always |
| `AssistantMessage` | `PIGO_ASSISTANT_MESSAGE` | Always |
| `ToolName` | `PIGO_TOOL_NAME` | Tool events |
| `ToolArgs` | `PIGO_TOOL_ARGS` | Tool events |
| `ToolInput` | `PIGO_TOOL_INPUT` | `tool_start` only |
| `ToolOutput` | `PIGO_TOOL_OUTPUT` | `tool_end` only (truncated to 10K chars) |
| `ToolError` | `PIGO_TOOL_ERROR` | `tool_end` only |
| `TurnNumber` | `PIGO_TURN_NUMBER` | `turn_start`, `turn_end` |

## Events

| Event | Description |
|---|---|
| `agent_start` | Agent begins processing user input |
| `agent_end` | Agent finishes processing |
| `turn_start` | Start of each agent loop iteration |
| `turn_end` | End of each iteration |
| `tool_start` | Before tool execution (blocking hooks can cancel the tool) |
| `tool_end` | After tool execution |

## API

| Function / Method | Description |
|---|---|
| `NewHookManager(plugins) *HookManager` | Creates a manager with only enabled plugins |
| `Run(ctx, hctx) error` | Executes matching hooks; returns error only if a blocking `tool_start` hook fails |
| `GetPlugins() []PluginConfig` | Returns the list of active plugins |

## Execution Model

- **Blocking hooks** (default): Run synchronously. On `tool_start`, a failure cancels the tool. On other events, failures are logged to stderr.
- **Async hooks**: Fire-and-forget via `cmd.Start()`.
- **Match rules**: Hooks without a match rule run for all tools. Pipe-separated tool names filter by tool.

## Dependencies

Standard library only (`os/exec`, `context`, `time`).
