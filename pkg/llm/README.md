# pkg/llm

Thin, opinionated wrapper around the `openai-go` SDK. Supports two OpenAI-compatible API modes (Chat Completions and Responses) and streaming output. Translates between pigo's internal `types.Message` format and the SDK's wire types.

## Key Types

### Client

Holds an `openai.Client` instance, a model name, and an API type (`"chat"` or `"responses"`). All LLM interactions flow through this struct.

## API

| Function / Method | Description |
|---|---|
| `NewClient(apiKey, baseURL, model, apiType) *Client` | Creates a configured client with automatic retries (up to 3) |
| `Chat(ctx, messages, toolDefs) (*ChatResponse, error)` | Non-streaming chat request via Chat Completions or Responses API |
| `ChatStream(ctx, messages, toolDefs, w) (*ChatResponse, error)` | Streaming chat request; text deltas written to `w` as they arrive |
| `GetModel() string` | Returns the current model name |
| `SetModel(model)` | Switches the model for subsequent requests |
| `IsContextOverflow(err) bool` | Detects context-window overflow errors across multiple provider formats |

## API Modes

- **`chat`** — Uses the Chat Completions API (`/v1/chat/completions`)
- **`responses`** — Uses the Responses API, mapping system messages to `instructions` and tool results to `function_call_output` items

Both modes support streaming and non-streaming variants.

## Dependencies

- `github.com/openai/openai-go` — SDK for Chat Completions and Responses API
- `pkg/types` — Shared `Message`, `ChatResponse`, `ToolCall`, `TokenUsage` types
