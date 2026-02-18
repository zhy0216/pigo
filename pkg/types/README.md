# pkg/types

Shared data types, interfaces, constants, and utilities used throughout the pigo codebase. Single source of truth for cross-cutting concerns.

## Messages (`message.go`)

| Type | Description |
|---|---|
| `Message` | Chat message with `Role`, `Content`, optional `ToolCalls`, and `ToolCallID` |
| `ToolCall` | Model-issued tool invocation: `ID`, `Type`, and `Function` (name + arguments JSON) |
| `ChatResponse` | Parsed LLM response: `Content`, `ToolCalls`, `FinishReason`, `Usage` |
| `TokenUsage` | Token accounting: `PromptTokens`, `CompletionTokens`, `TotalTokens` |

## Tool Interface (`tool.go`)

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{}
    Execute(ctx context.Context, args map[string]interface{}) *ToolResult
}
```

## Tool Results (`result.go`)

| Constructor | Description |
|---|---|
| `NewToolResult(forLLM)` | Visible only to the LLM |
| `SilentResult(forLLM)` | LLM confirmation, no user output |
| `ErrorResult(message)` | Error shown to both LLM and user |
| `UserResult(content)` | Same content for both LLM and user |

`ToolResult` separates LLM and user output so tools can send rich context to the model without cluttering the terminal.

## Events (`events.go`)

Lightweight, thread-safe pub-sub event emitter for agent lifecycle events.

| Event | When |
|---|---|
| `EventAgentStart` | Agent begins a session |
| `EventTurnStart` | Start of each agent loop turn |
| `EventTurnEnd` | End of each turn |
| `EventMessageStart` | LLM starts generating |
| `EventMessageEnd` | LLM finishes generating |
| `EventToolStart` | Before tool execution |
| `EventToolEnd` | After tool execution |
| `EventAgentEnd` | Agent session ends |

`EventEmitter` uses swap-with-last-element on unsubscribe for O(1) removal. Emit operates under a read lock for concurrent delivery.

## Constants (`constants.go`)

| Constant | Value | Description |
|---|---|---|
| `MaxContextChars` | 200,000 | Character budget for message history |
| `ProactiveCompactThreshold` | 0.8 | Compaction trigger threshold |
| `KeepRecentChars` | 80,000 | Recent chars preserved during compaction |
| `MinKeepMessages` | 10 | Minimum messages kept during truncation |
| `MaxOverflowRetries` | 2 | Retries after context-overflow errors |
| `MaxAgentIterations` | 10 | Max tool-calling iterations per turn |
| `MaxReadFileSize` | 10 MB | File size ceiling for read tool |
| `MaxLineLength` | 500 | Per-line truncation for file reads |
| `BashMaxOutput` | 10,000 | Max chars for bash output |
| `BashDefaultTimeout` | 120 | Default bash timeout (seconds) |
| `GrepMaxMatches` | 100 | Max grep matches |
| `GrepMaxBytes` | 50 KB | Max grep output size |
| `GrepMaxLine` | 500 | Per-line truncation for grep |
| `FindMaxResults` | 1,000 | Max find results |
| `FindMaxBytes` | 50 KB | Max find output size |
| `LsMaxEntries` | 1,000 | Max ls entries |
| `SessionsDir` | `.pigo/sessions` | Session storage path |

## Colors (`colors.go`)

ANSI escape code constants: `ColorReset`, `ColorGreen`, `ColorYellow`, `ColorBlue`, `ColorGray`.
