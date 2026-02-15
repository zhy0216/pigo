# pigo

Minimal AI coding assistant CLI written in Go. Single-package binary that interfaces with OpenAI-compatible APIs using tool-calling.

## Reference Implementation

`docs/reference/pi-mono-main/` is the reference implementation (TypeScript monorepo). Consult it for design patterns, tool behavior, and agent loop semantics when adding features or resolving ambiguity.

Key reference packages:
- `packages/agent/` — agent runtime and state management
- `packages/coding-agent/` — interactive coding agent (closest analog to pigo)
- `packages/ai/` — multi-provider LLM API layer

## Build & Test

```bash
make build        # Build binary (output: ./pigo)
make test         # Run tests
make test-race    # Run tests with race detector
make test-cover   # Run tests with coverage report
make lint         # Check formatting (gofmt) + go vet
make run          # Run from source
make clean        # Remove build artifacts
```

## Architecture

Single Go package (`package main`), flat file structure:

- **Entry point**: `main.go` — CLI loop, config loading, agent loop (`ProcessInput` with max 10 iterations)
- **API client**: `client.go` — OpenAI client supporting both Chat Completions and Responses API modes
- **Tool framework**: `tools.go` — `Tool` interface + `ToolRegistry` (thread-safe)
- **Tool results**: `result.go` — `ToolResult` with separate `ForLLM`/`ForUser` fields
- **Tools**: `tool_read.go`, `tool_write.go`, `tool_edit.go`, `tool_bash.go`
- **Utilities**: `utils.go` — path validation, line-number formatting, output truncation

### Tool Interface

All tools implement:
```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, args map[string]interface{}) *ToolResult
}
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | — | API authentication token |
| `OPENAI_BASE_URL` | No | `https://api.openai.com/v1` | API endpoint |
| `PIGO_MODEL` | No | `gpt-4o` | Model name |
| `OPENAI_API_TYPE` | No | `chat` | `chat` or `responses` |

## Code Style

- Go standard formatting (`gofmt`)
- No external dependencies beyond `openai-go` (and its transitive deps)
- Tests use temporary directories; clean up after themselves
- Tool implementations follow the pattern in existing `tool_*.go` files
- Errors returned via `ErrorResult()`, not panics
- Output truncation at 10,000 chars (bash), 500 chars per line (read)

## Testing

- Unit tests colocated: `foo.go` → `foo_test.go`
- Integration tests: `integration_test.go` (full agent loop with mock LLM)
- Target: 80%+ coverage
- CI runs tests on Go stable + oldstable with race detection
