# pkg/config

Configuration loading and merging. Reads from a JSON config file, environment variables, and built-in defaults, then resolves them in priority order. Bootstraps the workspace directory on first run.

## Config File

Located at `~/.pigo/config.json` (or `$PIGO_HOME/config.json`). All fields are optional.

```json
{
  "api_key": "your-api-key",
  "base_url": "https://openrouter.ai/api/v1",
  "model": "anthropic/claude-3.5-sonnet",
  "api_type": "chat",
  "system_prompt": "You are a helpful coding assistant.",
  "plugins": ["my-plugin"]
}
```

## Priority Order

config file > environment variables > defaults

## Key Types

### Config

| Field | Type | Source (file / env / default) |
|---|---|---|
| `APIKey` | string | `api_key` / `OPENAI_API_KEY` / (required) |
| `BaseURL` | string | `base_url` / `OPENAI_BASE_URL` / `""` |
| `Model` | string | `model` / `PIGO_MODEL` / `gpt-4o` |
| `APIType` | string | `api_type` / `OPENAI_API_TYPE` / `chat` |
| `SystemPrompt` | string | `system_prompt` / — / (built-in default) |
| `Plugins` | `[]hooks.PluginConfig` | `plugins` (string names resolved from `plugins.json`) / — / `nil` |

## Plugin Resolution

The `plugins` field in `config.json` is a string array of plugin names. These names are resolved against `~/.pigo/plugins.json`, which maps names to full `PluginConfig` definitions. Unknown names are skipped with a warning.

## API

| Function | Description |
|---|---|
| `Load() (*Config, error)` | Ensures workspace, reads config file, resolves plugins, merges with env vars and defaults; errors if API key is missing |
| `HomeDir() (string, error)` | Returns `~/.pigo` or `$PIGO_HOME` |
| `EnsureWorkspace() error` | Creates home dir, `skills/` subdir, writes `config.schema.json`, seeds default `config.json` if absent |

## Embedded Schema

`config.schema.json` (JSON Schema Draft 7) is embedded via `//go:embed` and written to `~/.pigo/config.schema.json` on every startup, keeping editor autocompletion up to date.

## Dependencies

- `pkg/hooks` — `PluginConfig` type for plugin definitions
- Standard library only (`encoding/json`, `os`, `path/filepath`)
