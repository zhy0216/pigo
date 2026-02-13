# pigo Design Document

> Minimal CLI AI coding assistant in Go, reimplementing nanocode-ts

## Overview

| Attribute | Value |
|-----------|-------|
| Project | pigo |
| Location | ~/pigo |
| Go version | 1.22+ |
| API | OpenAI-compatible (via openai-go SDK) |
| Architecture | Single package, multiple files |
| Code style | Follow picoclaw patterns |
| Tools | 4 (read, write, edit, bash) |

### Reference Projects

- **nanocode-ts** (`~/.openclaw/workspace/nanocode-ts`): Feature set and UX
- **picoclaw** (`~/picoclaw`): Go design patterns and code style

## Architecture

```
~/pigo/
├── go.mod              # Module definition
├── go.sum              # Dependency lock
├── main.go             # Entry point, CLI loop
├── client.go           # OpenAI client wrapper
├── tools.go            # Tool interface, registry, result types
├── tool_read.go        # ReadTool implementation
├── tool_write.go       # WriteTool implementation
├── tool_edit.go        # EditTool implementation
├── tool_bash.go        # BashTool implementation
└── utils.go            # Path validation, formatting
```

### Key Interfaces

```go
// Tool interface - matches picoclaw pattern
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, args map[string]interface{}) *ToolResult
}

// ToolResult - matches picoclaw pattern
type ToolResult struct {
    ForLLM  string
    ForUser string
    Silent  bool
    IsError bool
}
```

## Tool Specifications

### read

Read file contents with line numbers.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| path | string | Yes | Absolute file path |
| offset | int | No | Starting line (1-indexed, default: 1) |
| limit | int | No | Max lines to read (default: all) |

### write

Write/overwrite file contents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| path | string | Yes | Absolute file path |
| content | string | Yes | Content to write |

### edit

Replace strings in files.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| path | string | Yes | Absolute file path |
| old_string | string | Yes | Text to find |
| new_string | string | Yes | Replacement text |
| all | bool | No | Replace all occurrences (default: false) |

### bash

Execute shell commands.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| command | string | Yes | Shell command to execute |
| timeout | int | No | Timeout in seconds (default: 120) |

## Dependencies

```go
require (
    github.com/openai/openai-go v0.1.0-alpha
    github.com/chzyer/readline v1.5.1
)
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes | API key |
| `OPENAI_BASE_URL` | No | Custom endpoint (default: api.openai.com) |
| `PIGO_MODEL` | No | Model name (default: gpt-4o) |

## Milestones

| Milestone | Description | Deliverable |
|-----------|-------------|-------------|
| M1 | Project scaffolding | go.mod, main.go skeleton |
| M2 | OpenAI client | client.go with Chat() |
| M3 | Tool framework | tools.go with interface, registry |
| M4 | Core tools | 4 tools implemented and tested |
| M5 | CLI loop | Interactive REPL with history |
| M6 | Integration | End-to-end test, README |

## Implementation Steps

### Phase 1: Foundation (M1-M2)

1. Create `~/pigo` directory and `go.mod`
2. Implement `main.go` with basic CLI skeleton
3. Implement `client.go` with OpenAI client wrapper
4. Test API connectivity

### Phase 2: Tool Framework (M3)

5. Define `Tool` interface in `tools.go`
6. Implement `ToolRegistry`
7. Define `ToolResult` struct
8. Implement tool-to-OpenAI schema conversion

### Phase 3: Tools (M4)

9. Implement `tool_read.go` with unit tests
10. Implement `tool_write.go` with unit tests
11. Implement `tool_edit.go` with unit tests
12. Implement `tool_bash.go` with unit tests
13. Implement `utils.go`

### Phase 4: Integration (M5-M6)

14. Implement CLI loop with readline
15. Wire up tool execution
16. Add conversation history
17. Add `/q` and `/c` commands
18. End-to-end testing
19. Write README.md

## Testing Strategy

### Unit Tests

| File | Coverage |
|------|----------|
| `tool_read_test.go` | Read existing, non-existent, offset/limit |
| `tool_write_test.go` | Create new, overwrite, create dirs |
| `tool_edit_test.go` | Unique match, no match, all=true |
| `tool_bash_test.go` | Success, failure, timeout |
| `client_test.go` | Mock API responses |

### Integration Tests

- Full agent loop with mock LLM
- Read-edit-write workflow
- Bash command execution

### Commands

```bash
go test ./...
go test -v -run Integration
go test -cover
go test -race
```

## Risks & Rollback

| Risk | Mitigation | Rollback |
|------|------------|----------|
| SDK breaking changes | Pin version | Revert commit |
| Tool security | Path validation | Disable bash |
| API rate limits | Backoff | Mock testing |
| File corruption | Atomic write | Backup before edit |

## Acceptance Criteria

### Functional

- [ ] Binary compiles on Linux/macOS
- [ ] Connects to OpenAI-compatible API
- [ ] All 4 tools work correctly
- [ ] Conversation history persists
- [ ] `/q` and `/c` commands work
- [ ] Streaming output works

### Non-Functional

- [ ] Single binary, no runtime deps
- [ ] Startup < 100ms
- [ ] Memory < 50MB idle
- [ ] Follows picoclaw code style

### Testing

- [ ] Coverage > 80%
- [ ] All tests pass
- [ ] No race conditions

### Documentation

- [ ] README with examples
- [ ] Env var documentation
- [ ] Build instructions
