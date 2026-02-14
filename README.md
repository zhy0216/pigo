# pigo

[![CI](https://github.com/user/pigo/actions/workflows/ci.yml/badge.svg)](https://github.com/user/pigo/actions/workflows/ci.yml)

Minimal AI coding assistant in Go, inspired by [nanocode](https://github.com/1rgs/nanocode).

## Features

- **4 tools**: read, write, edit, bash
- **OpenAI-compatible API**: Works with OpenAI, OpenRouter, vLLM, or any compatible endpoint
- **Interactive CLI**: Conversation history, streaming output
- **Lightweight**: Single binary, ~2MB, no runtime dependencies

## Installation

```bash
# Clone and build
git clone <repo>
cd pigo
go build -o pigo .

# Or install directly
go install github.com/user/pigo@latest
```

## Usage

```bash
# Set API key (required)
export OPENAI_API_KEY="your-api-key"

# Optional: Use OpenRouter or other compatible endpoints
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
export PIGO_MODEL="anthropic/claude-3.5-sonnet"

# Run
./pigo
```

### Example Session

```
pigo - minimal AI coding assistant (model: gpt-4o)
Tools: read, write, edit, bash
Commands: /q (quit), /c (clear)

> Read main.go and explain what it does

[read]
     1  package main
     2  ...

This is the entry point for pigo. It initializes the OpenAI client,
registers tools, and runs an interactive CLI loop...

> Add a comment at the top of main.go

[edit] File edited: main.go

Done! I've added a package comment explaining the purpose of the file.
```

### Commands

| Command | Description |
|---------|-------------|
| `/q` or `exit` | Quit the session |
| `/c` or `clear` | Clear conversation history |

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | - | API key for authentication |
| `OPENAI_BASE_URL` | No | `https://api.openai.com/v1` | Custom API endpoint |
| `PIGO_MODEL` | No | `gpt-4o` | Model name |
| `PIGO_MEMPROFILE` | No | - | File path to write heap profile on exit |

## Tools

### read

Read file contents with line numbers.

```json
{
  "path": "/absolute/path/to/file",
  "offset": 1,
  "limit": 100
}
```

### write

Write content to a file. Creates parent directories if needed.

```json
{
  "path": "/absolute/path/to/file",
  "content": "file contents"
}
```

### edit

Replace text in a file. Fails if old_string is not found or not unique.

```json
{
  "path": "/absolute/path/to/file",
  "old_string": "text to find",
  "new_string": "replacement text",
  "all": false
}
```

### bash

Execute a shell command with timeout.

```json
{
  "command": "ls -la",
  "timeout": 120
}
```

## Architecture

```
pigo/
├── main.go          # CLI entry point
├── client.go        # OpenAI API client
├── tools.go         # Tool interface and registry
├── result.go        # ToolResult type
├── utils.go         # Helper functions
├── tool_read.go     # read tool
├── tool_write.go    # write tool
├── tool_edit.go     # edit tool
└── tool_bash.go     # bash tool
```

Design patterns from [picoclaw](https://github.com/sipeed/picoclaw):
- `Tool` interface with `Name()`, `Description()`, `Parameters()`, `Execute()`
- `ToolResult` with `ForLLM`, `ForUser`, `Silent`, `IsError`
- `ToolRegistry` for tool management

## Development

```bash
make help        # Show all available commands
```

| Command | Description |
|---------|-------------|
| `make build` | Build the binary |
| `make run` | Run from source |
| `make clean` | Remove build artifacts and profiles |
| `make test` | Run tests |
| `make test-race` | Run tests with race detector |
| `make test-cover` | Run tests with coverage report |
| `make lint` | Run all linters (fmt + vet) |
| `make prof-mem` | Run with memory profiling, then open in browser |

### Memory Profiling

```bash
# Option 1: via make (builds, runs, then opens profile in browser)
make prof-mem

# Option 2: manually
PIGO_MEMPROFILE=mem.prof ./pigo
# use pigo, then exit with /q
go tool pprof mem.prof        # interactive CLI
go tool pprof -http=:8080 mem.prof  # browser UI
```

## License

MIT
