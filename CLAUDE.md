# pigo

Minimal AI coding assistant CLI written in Go. Multi-package binary that interfaces with OpenAI-compatible APIs using tool-calling.

## Reference Implementation

`docs/reference/pi-mono-main/` is the reference implementation (TypeScript monorepo). Consult it for design patterns, tool behavior, and agent loop semantics when adding features or resolving ambiguity.

Key reference packages:
- `packages/agent/` — agent runtime and state management
- `packages/coding-agent/` — interactive coding agent (closest analog to pigo)
- `packages/ai/` — multi-provider LLM API layer

## Build & Test

```bash
make build        # Build binary (output: ./pigo) from ./cmd/pigo
make test         # Run tests
make test-race    # Run tests with race detector
make test-cover   # Run tests with coverage report
make lint         # Check formatting (gofmt) + go vet
make run          # Run from source (go run ./cmd/pigo)
make clean        # Remove build artifacts
```

## Architecture

Multi-package layout under `cmd/` and `pkg/`:

- **Entry point**: `cmd/pigo/main.go` — CLI loop, signal handling (Ctrl-C cancels turn, double Ctrl-C exits), skill command expansion
- **Config**: `pkg/config/` — JSON config file loading (`~/.pigo/config.json`), env var merging, priority resolution
- **Agent**: `pkg/agent/` — `Agent` struct, `LoadConfig`, `HandleCommand` (all slash commands), `ProcessInput` (agent loop, max 10 iterations), proactive context compaction
- **LLM client**: `pkg/llm/` — OpenAI client supporting Chat Completions and Responses API modes, streaming
- **Tool framework**: `pkg/tools/` — `ToolRegistry` (thread-safe) + 7 tools: read, write, edit, bash, grep, find, ls
- **Skills**: `pkg/skills/` — Markdown skill files from `~/.pigo/skills/` (user) and `./.pigo/skills/` (project), YAML frontmatter
- **Types**: `pkg/types/` — shared types (`Tool` interface, `ToolResult`, `Message`, `EventEmitter`), constants, ANSI colors
- **Ops**: `pkg/ops/` — `FileOps`/`ExecOps` interfaces for testability
- **Utilities**: `pkg/util/` — path validation, line-number formatting, output truncation, env helpers

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

### Context Compaction

Proactive compaction triggers at 80% of `MaxContextChars` (200K chars). It keeps recent messages (~80K chars, minimum 10 messages), generates an LLM summary of discarded messages. Falls back to naive truncation if the LLM call fails.

## Environment Variables

All variables below can also be set in `~/.pigo/config.json` (config file takes priority over env vars).

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | — | API authentication token |
| `OPENAI_BASE_URL` | No | `https://api.openai.com/v1` | API endpoint |
| `PIGO_MODEL` | No | `gpt-4o` | Model name |
| `OPENAI_API_TYPE` | No | `chat` | `chat` or `responses` |
| `PIGO_MEMPROFILE` | No | — | Write heap profile to this path on exit |

## Code Style

- Go standard formatting (`gofmt`)
- No external dependencies beyond `openai-go` (and its transitive deps)
- Tests use temporary directories; clean up after themselves
- Tool implementations follow the pattern in existing `pkg/tools/tool_*.go` files
- Errors returned via `ErrorResult()`, not panics
- Output truncation: 10,000 chars tail-kept (bash), 500 chars per line (read), 50KB (grep/find)

## Testing

- Unit tests colocated: `foo.go` → `foo_test.go`
- Integration tests in `pkg/agent/integration_test.go` and `pkg/tools/integration_test.go`
- Target: 80%+ coverage (enforced in CI)
- CI runs tests on Go stable + oldstable with race detection
