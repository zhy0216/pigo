# pkg/config

Configuration loading and merging. Reads from a JSON config file, environment variables, and built-in defaults, then resolves them in priority order.

## Config File

Located at `~/.pigo/config.json` (or `$PIGO_HOME/config.json`). All fields are optional.

```json
{
  "api_key": "your-api-key",
  "base_url": "https://openrouter.ai/api/v1",
  "model": "anthropic/claude-3.5-sonnet",
  "api_type": "chat",
  "plugins": [
    {
      "name": "my-plugin",
      "hooks": {
        "tool_start": [{ "command": "echo $PIGO_TOOL_NAME" }]
      }
    }
  ]
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
| `Plugins` | `[]hooks.PluginConfig` | `plugins` / — / `nil` |

## API

| Function | Description |
|---|---|
| `Load() (*Config, error)` | Reads config file, merges with env vars and defaults; errors if API key is missing |

## Dependencies

- `pkg/hooks` — `PluginConfig` type for plugin definitions
- Standard library only (`encoding/json`, `os`, `path/filepath`)
