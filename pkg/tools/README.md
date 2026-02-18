# pkg/tools

Tool framework for the pigo agent. Provides a thread-safe registry and seven concrete tool implementations, each conforming to the `types.Tool` interface.

## Registry

`ToolRegistry` is a thread-safe map of registered tools, keyed by name.

| Method | Description |
|---|---|
| `Register(tool)` | Adds a tool to the registry |
| `Get(name) (Tool, bool)` | Looks up a tool by name |
| `Execute(ctx, name, args) *ToolResult` | Validates args against JSON schema, then executes |
| `GetDefinitions() []map[string]interface{}` | Returns OpenAI-compatible function tool definitions |
| `List() []string` | Returns all registered tool names |
| `ValidateArgs(params, args) error` | Validates args against a JSON Schema (exported for external use) |

## Tools

### read

Reads a file's contents with line numbers.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Absolute path to file |
| `offset` | integer | no | Starting line (1-indexed) |
| `limit` | integer | no | Max lines to read |

Enforces path restriction, rejects directories and files exceeding 10 MB. Lines truncated at 500 chars.

### write

Creates or overwrites a file.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Absolute path to file |
| `content` | string | yes | Content to write |

Creates parent directories as needed. Preserves existing permissions; defaults to `0644`.

### edit

Replaces an exact string in a file.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Absolute path to file |
| `old_string` | string | yes | Text to find |
| `new_string` | string | yes | Replacement text |
| `all` | boolean | no | Replace all occurrences |

Errors if `old_string` is not found or appears multiple times without `all=true`.

### bash

Executes shell commands via `/bin/bash -c`.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `command` | string | yes | Shell command |
| `timeout` | integer | no | Timeout in seconds (default: 120) |

Sanitizes environment to prevent secret leakage. Output truncated to 10,000 chars (tail-kept).

### grep

Searches file contents for a regex pattern.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `pattern` | string | yes | Regex pattern |
| `path` | string | no | Directory or file (default: `.`) |
| `include` | string | no | Glob filter (e.g., `*.go`) |
| `context_lines` | integer | no | Context lines before/after |

Prefers `rg` (ripgrep) when available; falls back to a pure-Go implementation. Output truncated at 50 KB.

### find

Discovers files and directories matching a glob pattern.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `pattern` | string | yes | Glob pattern |
| `path` | string | no | Directory (default: `.`) |
| `type` | string | no | `file`, `directory`, or `both` |

Prefers `fd`/`fdfind` when available; falls back to Go `WalkDir`. Max 1000 results.

### ls

Lists directory contents with type indicators.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Directory path |
| `all` | boolean | no | Show hidden files |

Entries annotated with `[file]`, `[dir]`, or `[link]`. Max 1000 entries.

## Common Patterns

- **Path restriction**: Tools that touch the filesystem enforce an `allowedDir` boundary via `util.ValidatePath`.
- **Dependency injection**: File and exec operations use `ops.FileOps` and `ops.ExecOps` interfaces for testability.
- **Error handling**: Errors returned via `types.ErrorResult()`, not panics.
