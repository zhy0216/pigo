# pkg/util

General-purpose helper functions used across the pigo codebase. Covers path validation, output formatting, tool argument extraction, environment variable handling, and string utilities.

## Path Utilities

| Function | Description |
|---|---|
| `ValidatePath(path, allowedDir) (string, error)` | Validates a path is absolute, clean, symlink-safe, and within `allowedDir` if set |
| `RelativizePath(path, basePath) string` | Returns `path` relative to `basePath`; returns `path` unchanged on failure |
| `IsBinaryExtension(name) bool` | Returns true for known binary file extensions (images, archives, executables, etc.) |

## Output Formatting and Truncation

| Function | Description |
|---|---|
| `FormatWithLineNumbers(content, offset, limit) string` | Formats content with `cat -n` style line numbers; truncates long lines at `MaxLineLength` |
| `TruncateOutput(output, maxLen) string` | Head-truncation with a notice of remaining characters |
| `TruncateTail(output, maxLen) string` | Tail-truncation (keeps the end); suited for bash output |

## Tool Argument Extraction

Helpers for extracting typed parameters from `map[string]interface{}` argument maps, handling JSON type coercion.

| Function | Description |
|---|---|
| `ExtractString(args, key) (string, error)` | Required string parameter |
| `ExtractOptionalString(args, key, default) string` | Optional string with default |
| `ExtractInt(args, key, default) int` | Integer parameter (handles JSON `float64`) |
| `ExtractBool(args, key, default) bool` | Boolean parameter with default |

## Environment Helpers

| Function | Description |
|---|---|
| `SanitizeEnv() []string` | Returns `os.Environ()` with sensitive variables stripped (`OPENAI_*`, `API_KEY*`, `SECRET*`, `TOKEN*`, `AWS_SECRET*`) |
| `GetEnvOrDefault(key, default) string` | Returns env var value or default if unset/empty |

## Message Utilities

| Function | Description |
|---|---|
| `EstimateMessageChars(messages) int` | Rough character count across messages (content + tool call args) |
| `StripCodeFence(s) string` | Removes markdown code fences from LLM responses before JSON parsing |

## Generic Utilities

| Function | Description |
|---|---|
| `MapKeys(m) []string` | Returns keys of a `map[string]bool` as a string slice |
| `XmlEscape(s) string` | Escapes XML special characters (`&`, `<`, `>`, `"`) |
