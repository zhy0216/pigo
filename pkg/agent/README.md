# pkg/agent

Core runtime for the pigo agent. Wires together the LLM client, tool registry, skills, event emitter, and hook manager, and drives the conversational agent loop.

## Key Types

### Agent

Central application struct. Owns the LLM client, tool registry, message history, loaded skills, event emitter, hook manager, and token usage accumulator.

## API

| Function / Method | Description |
|---|---|
| `NewAgent(cfg *config.Config) *Agent` | Constructs a fully initialized agent: LLM client, tools, skills, hooks, system prompt |
| `HandleCommand(input) (handled, exit bool)` | Processes slash commands (`/q`, `/c`, `/usage`, `/skills`, `/plugins`, `/model`) |
| `ProcessInput(ctx, input) error` | Runs the agent loop for a single user turn (up to 10 iterations) |
| `GetUsage() TokenUsage` | Returns accumulated token usage for the session |
| `GetRegistry() *ToolRegistry` | Returns the tool registry |
| `Events() *EventEmitter` | Returns the event emitter for subscribing to lifecycle events |
| `GetModel() string` | Returns the current model name |

## Agent Loop (`ProcessInput`)

1. Append user message to history.
2. Run proactive context compaction if near the limit.
3. Emit `agent_start` event and run `agent_start` hooks.
4. Loop up to `MaxAgentIterations` (10):
   - Emit `turn_start` event and run `turn_start` hooks.
   - Call the LLM (with overflow retry and truncation fallback).
   - If tool calls are returned:
     - Run `tool_start` hooks (blocking hooks can cancel tools).
     - Execute each tool via the registry.
     - Run `tool_end` hooks with tool output.
     - Append results and continue.
   - If a text response is returned: append and break.
   - Emit `turn_end` event and run `turn_end` hooks.
5. Emit `agent_end` event and run `agent_end` hooks.

## Context Compaction

Proactive compaction triggers at 80% of `MaxContextChars` (200K chars). It keeps recent messages (~80K chars, minimum 10 messages), generates an LLM summary of discarded messages. Falls back to naive truncation if the LLM call fails.
