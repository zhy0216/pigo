# pigo

[![CI](https://github.com/user/pigo/actions/workflows/ci.yml/badge.svg)](https://github.com/user/pigo/actions/workflows/ci.yml)

Minimal AI coding assistant in Go, inspired by [nanocode](https://github.com/1rgs/nanocode).

## Features

- **10 tools**: read, write, edit, bash, grep, find, ls, memory_recall, memory_remember, memory_forget
- **OpenAI-compatible API**: Works with OpenAI, OpenRouter, vLLM, or any compatible endpoint
- **Interactive CLI**: Conversation history, streaming output, session persistence
- **Long-term memory**: Persistent memory with vector similarity search and automatic deduplication
- **Skills system**: User and project-level skill definitions via Markdown files
- **Context compaction**: Proactive context management with LLM-generated summaries

## Installation

```bash
# Clone and build
git clone https://github.com/zhy0216/pigo.git
cd pigo
make build

# Or install directly
go install github.com/zhy0216/pigo/cmd/pigo@latest
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
Tools: read, write, edit, bash, grep, find, ls, memory_recall, memory_remember, memory_forget
Commands: /q (quit), /c (clear), /model, /usage, /save, /load, /sessions, /skills, /memory

> Read main.go and explain what it does

[read]
     1  package main
     2  ...

This is the entry point for pigo. It loads config, creates an agent,
and runs an interactive CLI loop...

> Find all test files
[find] Found 15 results
...
```

### Commands

| Command | Description |
|---------|-------------|
| `/q`, `exit`, `quit` | Quit the session |
| `/c`, `clear` | Clear conversation history |
| `/model [name]` | Show or change the current model |
| `/usage` | Show token usage statistics |
| `/save [name]` | Save the current session |
| `/load <name>` | Load a saved session |
| `/sessions` | List saved sessions |
| `/skills` | List available skills |
| `/memory` | Show memory statistics |
| `/memory clear` | Clear all stored memories |
| `/skill:<name>` | Invoke a skill by name |

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | - | API key for authentication |
| `OPENAI_BASE_URL` | No | `https://api.openai.com/v1` | Custom API endpoint |
| `PIGO_MODEL` | No | `gpt-4o` | Model name |
| `OPENAI_API_TYPE` | No | `chat` | API mode: `chat` (Chat Completions) or `responses` (Responses API) |
| `PIGO_EMBED_MODEL` | No | `text-embedding-3-small` | Embedding model for memory search |
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

Execute a shell command with timeout (default 120s). Sensitive environment variables are stripped.

```json
{
  "command": "ls -la",
  "timeout": 120
}
```

### grep

Search file contents with regex. Requires [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`) to be installed.

```json
{
  "pattern": "func\\s+main",
  "path": "/project",
  "include": "*.go",
  "context_lines": 2
}
```

### find

Find files by glob pattern. Uses `fd`/`fdfind` if available, falls back to native Go.

```json
{
  "pattern": "*.go",
  "path": "/project",
  "type": "file"
}
```

### ls

List directory contents with type indicators.

```json
{
  "path": "/project",
  "all": false
}
```

### memory_recall

Search long-term memory by vector similarity and keyword matching.

```json
{
  "query": "user prefers tabs over spaces",
  "category": "preferences",
  "top_k": 5
}
```

### memory_remember

Explicitly save a memory with automatic deduplication.

```json
{
  "category": "preferences",
  "abstract": "User prefers tabs",
  "overview": "User stated preference for tabs over spaces in Go code",
  "content": "During session on 2026-01-15, user explicitly asked to use tabs..."
}
```

### memory_forget

Delete a memory by ID.

```json
{
  "id": "mem_abc123"
}
```

## Architecture

```
pigo/
├── cmd/pigo/
│   └── main.go              # CLI entry point, signal handling
├── pkg/
│   ├── agent/
│   │   ├── agent.go          # Agent struct, config, ProcessInput loop
│   │   └── compaction.go     # Proactive context compaction
│   ├── llm/
│   │   └── client.go         # OpenAI client (chat + responses API, streaming, embeddings)
│   ├── memory/
│   │   ├── store.go          # Persistent JSONL memory store with vector search
│   │   ├── extractor.go      # LLM-based memory extraction from compacted messages
│   │   ├── deduplicator.go   # Vector similarity + LLM deduplication
│   │   └── tools.go          # memory_recall, memory_remember, memory_forget tools
│   ├── ops/
│   │   └── ops.go            # FileOps/ExecOps interfaces for testability
│   ├── session/
│   │   └── session.go        # JSONL session persistence (~/.pigo/sessions/)
│   ├── skills/
│   │   └── skills.go         # Skill loading from ~/.pigo/skills/ and ./.pigo/skills/
│   ├── tools/
│   │   ├── registry.go       # Tool registry (thread-safe)
│   │   ├── read.go           # read tool
│   │   ├── write.go          # write tool
│   │   ├── edit.go           # edit tool
│   │   ├── bash.go           # bash tool
│   │   ├── grep.go           # grep tool
│   │   ├── find.go           # find tool
│   │   └── ls.go             # ls tool
│   ├── types/
│   │   ├── constants.go      # All numeric constants and limits
│   │   ├── colors.go         # ANSI terminal colors
│   │   ├── tool.go           # Tool interface
│   │   ├── result.go         # ToolResult type
│   │   ├── message.go        # Message, ToolCall, ChatResponse types
│   │   └── events.go         # EventEmitter (pub-sub, thread-safe)
│   └── util/
│       └── util.go           # Path validation, formatting, truncation helpers
├── docs/reference/            # Reference TypeScript implementation
├── Makefile
└── go.mod
```

Design patterns from [picoclaw](https://github.com/sipeed/picoclaw):
- `Tool` interface with `Name()`, `Description()`, `Parameters()`, `Execute()`
- `ToolResult` with `ForLLM`, `ForUser`, `Silent`, `IsError`
- `ToolRegistry` for thread-safe tool management

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
