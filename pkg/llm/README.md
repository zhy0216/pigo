# pkg/llm

Thin, opinionated wrapper around the `openai-go` SDK. Supports two OpenAI-compatible API modes (Chat Completions and Responses), streaming output, and text embeddings. Translates between pigo's internal `types.Message` format and the SDK's wire types.

## Key Types

### Client

Holds an `openai.Client` instance, a model name, and an API type (`"chat"` or `"responses"`). All LLM interactions flow through this struct.

## API

| Function / Method | Description |
|---|---|
| `NewClient(apiKey, baseURL, model, apiType) *Client` | Creates a configured client with automatic retries (up to 3) |
| `Chat(ctx, messages, toolDefs) (*ChatResponse, error)` | Non-streaming chat request via Chat Completions or Responses API |
| `ChatStream(ctx, messages, toolDefs, w) (*ChatResponse, error)` | Streaming chat request; text deltas written to `w` as they arrive |
| `Embed(ctx, text) ([]float64, error)` | Generates an embedding vector using the configured embedding model |
| `GetModel() string` | Returns the current model name |
| `SetModel(model)` | Switches the model for subsequent requests |
| `IsContextOverflow(err) bool` | Detects context-window overflow errors across multiple provider formats |

## API Modes

- **`chat`** — Uses the Chat Completions API (`/v1/chat/completions`)
- **`responses`** — Uses the Responses API, mapping system messages to `instructions` and tool results to `function_call_output` items

Both modes support streaming and non-streaming variants.

## Configuration

| Env Var | Default | Used By |
|---|---|---|
| `PIGO_EMBED_MODEL` | `text-embedding-3-small` | `Embed()` |

## Dependencies

- `github.com/openai/openai-go` — SDK for Chat Completions, Responses API, and Embeddings
- `pkg/types` — Shared `Message`, `ChatResponse`, `ToolCall`, `TokenUsage` types
- `pkg/util` — `GetEnvOrDefault` helper
