# Config File Support

## Summary

Add a JSON config file at `~/.pigo/config.json` that mirrors existing environment variables. Config file values take priority over env vars, which take priority over hardcoded defaults.

## Config File

**Location**: `~/.pigo/config.json`

**Format**:
```json
{
  "api_key": "sk-...",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o",
  "api_type": "chat",
  "embed_model": "text-embedding-3-small"
}
```

All fields are optional. Missing fields fall through to env vars, then defaults.

`PIGO_MEMPROFILE` is excluded — it's a debugging flag used only in `cmd/pigo/main.go` and doesn't belong in persistent config.

## Priority Order

```
config file > env var > hardcoded default
```

## Fields Mapping

| JSON key      | Env var            | Default                        |
|---------------|--------------------|--------------------------------|
| `api_key`     | `OPENAI_API_KEY`   | (required)                     |
| `base_url`    | `OPENAI_BASE_URL`  | `""`                           |
| `model`       | `PIGO_MODEL`       | `"gpt-4o"`                     |
| `api_type`    | `OPENAI_API_TYPE`  | `"chat"`                       |
| `embed_model` | `PIGO_EMBED_MODEL` | `"text-embedding-3-small"`     |

## Architecture

New package `pkg/config/` with:

- `Config` struct (moved from `pkg/agent/`)
- `Load() (*Config, error)` — reads config file, merges with env vars and defaults
- `configPath()` — returns `~/.pigo/config.json`

### Changes to existing code

- `pkg/agent/`: Remove `Config` struct and `LoadConfig()`. Import from `pkg/config/` instead.
- `pkg/llm/client.go`: `Embed()` reads `EmbedModel` from `Config` instead of calling `os.Getenv("PIGO_EMBED_MODEL")` directly. This requires threading the embed model through `NewClient` or `Config`.
- `cmd/pigo/main.go`: Call `config.Load()` instead of `agent.LoadConfig()`.

### No CLI command

Config is edited manually. No `/config` command needed.
