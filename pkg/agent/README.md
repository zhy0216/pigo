# pkg/agent

Core runtime for the pigo agent. Wires together the LLM client, tool registry, memory system, session management, and skills, and drives the conversational agent loop.

## Key Types

### Config

Application configuration loaded from environment variables via `LoadConfig()`.

| Field | Source | Default |
|-------|--------|---------|
| `APIKey` | `OPENAI_API_KEY` | (required) |
| `BaseURL` | `OPENAI_BASE_URL` | `https://api.openai.com/v1` |
| `Model` | `PIGO_MODEL` | `gpt-4o` |
| `APIType` | `OPENAI_API_TYPE` | `chat` |

### Agent

Central application struct. Owns the LLM client, tool registry, message history, loaded skills, event emitter, token usage accumulator, and memory subsystem.

## API

| Function / Method | Description |
|---|---|
| `LoadConfig() (*Config, error)` | Reads configuration from environment variables |
| `NewAgent(cfg) *Agent` | Constructs a fully initialized agent: LLM client, tools, skills, memory, system prompt |
| `HandleCommand(input) (handled, exit bool)` | Processes slash commands (`/q`, `/clear`, `/usage`, `/sessions`, `/skills`, `/memory`, `/save`, `/load`, `/model`) |
| `ProcessInput(ctx, input) error` | Runs the agent loop for a single user turn (up to 10 iterations) |
| `GetUsage() TokenUsage` | Returns accumulated token usage for the session |
| `GetRegistry() *ToolRegistry` | Returns the tool registry |
| `Events() *EventEmitter` | Returns the event emitter for subscribing to lifecycle events |
| `GetModel() string` | Returns the current model name |

## Agent Loop (`ProcessInput`)

1. Append user message to history.
2. Run proactive context compaction if near the limit.
3. Loop up to `MaxAgentIterations` (10):
   - Call the LLM (with overflow retry and truncation fallback).
   - If tool calls are returned: execute each via the registry, append results, continue.
   - If a text response is returned: append and break.
4. Emit lifecycle events throughout (`EventAgentStart`, `EventTurnStart`, `EventToolStart`, `EventToolEnd`, `EventTurnEnd`, `EventMessageEnd`, `EventAgentEnd`).

## Context Compaction

Proactive compaction triggers at 80% of `MaxContextChars` (200K chars). It keeps recent messages (~80K chars, minimum 10 messages), generates an LLM summary of discarded messages, extracts file-operation metadata, and saves memories from discarded content. Falls back to naive truncation if the LLM call fails.
