# Pigo Improvement Tasks

Improvements identified by comparing pigo against the reference implementation (`docs/reference/pi-mono-main/`).

## High Priority

### 1. Add `grep` tool (wrapping `rg`)

**File:** `tool_grep.go`

Without a dedicated grep tool, the LLM uses `bash` for searches, wasting tokens on shell formatting and producing unstructured output. The reference implementation wraps ripgrep with streaming JSON output.

- Shell out to `rg --json` with pattern and path arguments
- Parameters: `pattern` (required), `path` (optional, defaults to cwd), `include` (glob filter), `context_lines` (before/after)
- Apply match limit (default 100) and byte limit (50KB) — whichever hits first
- Truncate individual match lines to 500 chars
- Kill `rg` process when match limit is reached
- Return structured output: file paths with line numbers and matched content
- Follow existing `tool_*.go` pattern, respect `allowedDir` boundary

### 2. Add `find` tool (wrapping `fd`)

**File:** `tool_find.go`

A dedicated file discovery tool gives the LLM structured directory exploration without bash overhead. The reference wraps `fd` with glob support.

- Shell out to `fd` with glob pattern arguments
- Parameters: `pattern` (required glob), `path` (optional, defaults to cwd), `type` (file/directory/both)
- Apply result limit (default 1000) and byte limit (50KB)
- Respect `.gitignore` exclusions (fd does this by default)
- Return relative paths, one per line
- Follow existing `tool_*.go` pattern, respect `allowedDir` boundary

### 3. Streaming responses

**Files:** `client.go`, `main.go`

Pigo waits for the entire LLM response before displaying anything. Users see no output during long generations. The reference streams text/thinking/toolcall deltas to the UI in real-time.

- Use `client.Chat.Completions.NewStreaming()` from the openai-go library
- Print text deltas to stdout as they arrive
- Accumulate tool call deltas and dispatch when complete
- Add a `ChatStream` method alongside existing `Chat`
- Update `ProcessInput` to use streaming by default
- For Responses API: use the equivalent streaming endpoint

### 4. Fix bash output truncation (keep tail, not head)

**File:** `tool_bash.go`, `utils.go`

Pigo's `truncateOutput` cuts from the end, discarding the final lines. For bash commands (builds, tests), the most important information (errors, exit status) is at the end. The reference uses `truncateTail` for bash output.

- Add `truncateTail(output string, maxChars int) string` to `utils.go` — keeps last N chars
- Keep existing `truncateOutput` (head truncation) for file reads
- Change `tool_bash.go:86` to use `truncateTail` instead of `truncateOutput`
- Include truncation metadata in the result: `[output truncated: showing last 10000 of N chars]`

### 5. Ctrl+C aborts turn, not process

**File:** `main.go`

Current signal handling terminates the entire process on SIGINT. The reference aborts only the current agent turn and returns to the prompt. Users should be able to interrupt a long-running turn without losing their session.

- Replace single SIGINT handler with a stateful one
- First SIGINT: cancel the current `context.Context` for `ProcessInput`, return to prompt
- Second SIGINT (within ~1s): exit the process
- Create a per-turn `context.WithCancel` inside the REPL loop
- Cancel that context on first SIGINT, reset for next prompt

## Medium Priority

### 6. Smarter context compaction

**Files:** `main.go` (new `compaction.go`)

Current truncation (`main.go:142-168`) drops messages blindly, keeping only the last 10. The reference walks backward with a token budget, summarizes discarded history via LLM, and preserves file operation metadata.

- Walk backward from newest messages, accumulating char estimates until hitting `keepRecentChars` budget (~80K chars)
- Find valid cut points (never mid-tool-result sequence)
- Call the LLM to summarize discarded messages into a compact summary
- Insert summary as a user message after the system prompt
- Track which files were read/modified in discarded messages
- On subsequent compactions, update existing summary incrementally
- Fallback to current naive truncation if LLM summarization fails

### 7. Tool argument validation

**File:** `tools.go`

The reference validates tool arguments against JSON schema before execution. Pigo relies on each tool to validate its own args, producing inconsistent error messages.

- Add `ValidateArgs(params map[string]interface{}, args map[string]interface{}) error` to `tools.go`
- Check `required` fields are present
- Check basic type matching (string, integer, boolean)
- Call validation in `ToolRegistry.Execute` before dispatching to the tool
- Return a structured error message on validation failure

### 8. Sequential tool execution with interrupt checks

**File:** `main.go`

The reference executes tools sequentially, checking for user interrupts between each. Pigo runs all tools concurrently with no interrupt path.

- Execute tool calls sequentially instead of concurrently
- Between each tool execution, check if the context was cancelled (user pressed Ctrl+C)
- If cancelled, skip remaining tool calls with `"Skipped due to user interrupt"` result
- Still send all tool results (including skipped) to the LLM for consistency

### 9. Event system

**File:** new `events.go`

The reference emits granular events (`agent_start`, `turn_start`, `tool_execution_start`, etc.) with pub-sub. Pigo uses inline `fmt.Fprintf`. An event system enables future UI improvements, testing, and extensibility.

- Define `AgentEvent` interface with event types: `AgentStart`, `TurnStart`, `TurnEnd`, `MessageStart`, `MessageEnd`, `ToolStart`, `ToolEnd`, `AgentEnd`
- Add `EventEmitter` with `Subscribe(fn func(AgentEvent))` and `Emit(AgentEvent)`
- Wire into `ProcessInput` loop
- Move current `fmt.Fprintf` output calls to a default subscriber
- This decouples the agent loop from output formatting

### 10. `agentLoopContinue` for retries

**File:** `main.go`

The reference has two loop entry points: new prompt and retry-from-context. When context overflow occurs, it compacts and retries. Pigo has no retry path.

- Detect context overflow errors from the API (HTTP 400 with token limit message)
- On overflow: trigger compaction, then retry the same turn without re-adding the user message
- Add a `continueLoop` path in `ProcessInput` that skips the user message append
- Max retry attempts: 2

## Lower Priority

### 11. Token/cost tracking

**Files:** `client.go`, `main.go`

The reference tracks usage (input/output/cache tokens + cost) on every response. Pigo doesn't track any usage.

- Parse `usage` field from API responses (both Chat Completions and Responses API)
- Accumulate per-session totals in `App`
- Display running totals on `/q` or a new `/usage` command
- Track: prompt tokens, completion tokens, total tokens

### 12. Session persistence

**Files:** new `session.go`

The reference saves sessions to files with branching. Pigo loses all history on restart.

- Save messages to `~/.pigo/sessions/<id>.jsonl` (append-only)
- Each line: JSON-encoded message with timestamp
- On startup: option to resume last session or list recent sessions
- Commands: `/save`, `/load`, `/sessions`
- Session ID: timestamp-based or user-named

### 13. Dynamic model switching

**File:** `main.go`, `client.go`

The reference allows changing model mid-session. Pigo's model is fixed at startup.

- Add `/model <name>` command
- Update `Client` to accept model changes: `SetModel(model string)`
- Display current model in prompt or on `/model` with no args

### 14. Add `ls` tool

**File:** `tool_ls.go`

A lightweight directory listing tool gives the LLM structured output without bash overhead.

- Parameters: `path` (required), `all` (show hidden, default false)
- Return entries with type indicators (file/dir/symlink)
- Respect `allowedDir` boundary
- Limit output to reasonable size (e.g., 1000 entries)

### 15. Pluggable tool operations

**Files:** all `tool_*.go`

The reference passes operation interfaces to tools (readFile, exec functions). This enables mock testing and future remote execution.

- Define `FileOps` interface: `ReadFile`, `WriteFile`, `Stat`, `MkdirAll`
- Define `ExecOps` interface: `Run(ctx, command) (stdout, stderr, error)`
- Pass these to tool constructors instead of calling `os` directly
- Default implementations use real filesystem/exec
- Test implementations use in-memory or temp filesystems
