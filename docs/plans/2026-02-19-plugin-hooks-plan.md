# Plugin Hooks System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a lightweight plugin hooks system where plugins define event hooks that run bash commands with environment variable interpolation.

**Architecture:** New `pkg/hooks/` package contains `HookManager` with types, matching, and execution logic. Config adds `Plugins` field. Agent calls `HookManager.Run()` at each event point in the agent loop. `tool_start` blocking hooks can intercept and cancel tool execution.

**Tech Stack:** Go stdlib only (`os/exec`, `context`, `encoding/json`, `strings`)

---

### Task 1: Hook Types and Config Parsing

**Files:**
- Create: `pkg/hooks/hooks.go`
- Create: `pkg/hooks/hooks_test.go`
- Modify: `pkg/config/config.go:11-24`
- Modify: `pkg/config/config_test.go`

**Step 1: Write the failing test for hook types**

In `pkg/hooks/hooks_test.go`:

```go
package hooks

import (
	"testing"
)

func TestNewHookManager_empty(t *testing.T) {
	mgr := NewHookManager(nil)
	if mgr == nil {
		t.Fatal("expected non-nil HookManager")
	}
}

func TestNewHookManager_skipsDisabled(t *testing.T) {
	disabled := false
	plugins := []PluginConfig{
		{Name: "active", Hooks: map[string][]HookConfig{"agent_start": {{Command: "echo hi"}}}},
		{Name: "off", Enabled: &disabled, Hooks: map[string][]HookConfig{"agent_start": {{Command: "echo no"}}}},
	}
	mgr := NewHookManager(plugins)
	if len(mgr.plugins) != 1 {
		t.Errorf("expected 1 active plugin, got %d", len(mgr.plugins))
	}
	if mgr.plugins[0].Name != "active" {
		t.Errorf("expected plugin 'active', got %q", mgr.plugins[0].Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/hooks/ -run TestNewHookManager -v`
Expected: FAIL — package does not exist

**Step 3: Write minimal implementation**

In `pkg/hooks/hooks.go`:

```go
package hooks

// PluginConfig defines a plugin with its hooks.
type PluginConfig struct {
	Name    string                  `json:"name"`
	Enabled *bool                   `json:"enabled,omitempty"`
	Hooks   map[string][]HookConfig `json:"hooks"`
}

// HookConfig defines a single hook.
type HookConfig struct {
	Command  string     `json:"command"`
	Match    *MatchRule `json:"match,omitempty"`
	Blocking *bool      `json:"blocking,omitempty"`
	Timeout  int        `json:"timeout,omitempty"`
}

// MatchRule defines conditions for when a hook runs.
type MatchRule struct {
	Tool string `json:"tool,omitempty"`
}

// HookContext provides event context for hook execution.
type HookContext struct {
	Event            string
	WorkDir          string
	Model            string
	UserMessage      string
	AssistantMessage string
	ToolName         string
	ToolArgs         string
	ToolInput        string
	ToolOutput       string
	ToolError        bool
	TurnNumber       int
}

// HookManager manages plugins and runs hooks.
type HookManager struct {
	plugins []PluginConfig
}

// NewHookManager creates a HookManager with only enabled plugins.
func NewHookManager(plugins []PluginConfig) *HookManager {
	var active []PluginConfig
	for _, p := range plugins {
		if p.Enabled != nil && !*p.Enabled {
			continue
		}
		active = append(active, p)
	}
	return &HookManager{plugins: active}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/hooks/ -run TestNewHookManager -v`
Expected: PASS

**Step 5: Add plugins field to config**

Add test in `pkg/config/config_test.go`:

```go
func TestLoadWithPlugins(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Setenv("PIGO_HOME", dir)
	t.Setenv("OPENAI_API_KEY", "sk-test")

	configJSON := `{
		"plugins": [
			{
				"name": "test-plugin",
				"hooks": {
					"tool_start": [
						{"command": "echo hello", "blocking": true, "timeout": 5}
					]
				}
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(cfg.Plugins))
	}
	if cfg.Plugins[0].Name != "test-plugin" {
		t.Errorf("plugin name = %q, want %q", cfg.Plugins[0].Name, "test-plugin")
	}
}
```

Then modify `pkg/config/config.go`:
- Import `"github.com/zhy0216/pigo/pkg/hooks"`
- Add `Plugins []hooks.PluginConfig` to `Config` struct
- Add `Plugins []hooks.PluginConfig \`json:"plugins,omitempty"\`` to `fileConfig` struct
- Set `cfg.Plugins = fc.Plugins` in `Load()`

**Step 6: Run all config tests**

Run: `go test ./pkg/config/ -v`
Expected: PASS (all existing + new test)

**Step 7: Commit**

```bash
git add pkg/hooks/hooks.go pkg/hooks/hooks_test.go pkg/config/config.go pkg/config/config_test.go
git commit -m "feat: add hook types, HookManager, and plugins config field"
```

---

### Task 2: Match Logic

**Files:**
- Modify: `pkg/hooks/hooks.go`
- Modify: `pkg/hooks/hooks_test.go`

**Step 1: Write the failing test for match logic**

In `pkg/hooks/hooks_test.go`, append:

```go
func TestMatchRule_matches(t *testing.T) {
	tests := []struct {
		name     string
		rule     *MatchRule
		toolName string
		want     bool
	}{
		{"nil rule matches all", nil, "bash", true},
		{"exact match", &MatchRule{Tool: "bash"}, "bash", true},
		{"exact no match", &MatchRule{Tool: "bash"}, "read", false},
		{"pipe match first", &MatchRule{Tool: "bash|write"}, "bash", true},
		{"pipe match second", &MatchRule{Tool: "bash|write"}, "write", true},
		{"pipe no match", &MatchRule{Tool: "bash|write"}, "read", false},
		{"empty tool field matches all", &MatchRule{Tool: ""}, "bash", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRule(tt.rule, tt.toolName)
			if got != tt.want {
				t.Errorf("matchesRule(%v, %q) = %v, want %v", tt.rule, tt.toolName, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/hooks/ -run TestMatchRule -v`
Expected: FAIL — `matchesRule` undefined

**Step 3: Implement matchesRule**

In `pkg/hooks/hooks.go`, add:

```go
import "strings"

// matchesRule checks if a hook's match rule applies to the given tool name.
func matchesRule(rule *MatchRule, toolName string) bool {
	if rule == nil || rule.Tool == "" {
		return true
	}
	for _, t := range strings.Split(rule.Tool, "|") {
		if t == toolName {
			return true
		}
	}
	return false
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/hooks/ -run TestMatchRule -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/hooks/hooks.go pkg/hooks/hooks_test.go
git commit -m "feat: add hook match rule logic"
```

---

### Task 3: Environment Variable Building

**Files:**
- Modify: `pkg/hooks/hooks.go`
- Modify: `pkg/hooks/hooks_test.go`

**Step 1: Write the failing test**

In `pkg/hooks/hooks_test.go`, append:

```go
func TestBuildEnv_common(t *testing.T) {
	hctx := &HookContext{
		Event:            "agent_start",
		WorkDir:          "/tmp/test",
		Model:            "gpt-4o",
		UserMessage:      "hello",
		AssistantMessage: "hi there",
	}
	env := buildEnv(hctx)
	m := envToMap(env)

	if m["PIGO_EVENT"] != "agent_start" {
		t.Errorf("PIGO_EVENT = %q", m["PIGO_EVENT"])
	}
	if m["PIGO_WORK_DIR"] != "/tmp/test" {
		t.Errorf("PIGO_WORK_DIR = %q", m["PIGO_WORK_DIR"])
	}
	if m["PIGO_MODEL"] != "gpt-4o" {
		t.Errorf("PIGO_MODEL = %q", m["PIGO_MODEL"])
	}
	if m["PIGO_USER_MESSAGE"] != "hello" {
		t.Errorf("PIGO_USER_MESSAGE = %q", m["PIGO_USER_MESSAGE"])
	}
	if m["PIGO_ASSISTANT_MESSAGE"] != "hi there" {
		t.Errorf("PIGO_ASSISTANT_MESSAGE = %q", m["PIGO_ASSISTANT_MESSAGE"])
	}
}

func TestBuildEnv_toolStart(t *testing.T) {
	hctx := &HookContext{
		Event:    "tool_start",
		WorkDir:  "/tmp",
		Model:    "gpt-4o",
		ToolName: "bash",
		ToolArgs: `{"command":"ls -la"}`,
		ToolInput: "ls -la",
	}
	env := buildEnv(hctx)
	m := envToMap(env)

	if m["PIGO_TOOL_NAME"] != "bash" {
		t.Errorf("PIGO_TOOL_NAME = %q", m["PIGO_TOOL_NAME"])
	}
	if m["PIGO_TOOL_ARGS"] != `{"command":"ls -la"}` {
		t.Errorf("PIGO_TOOL_ARGS = %q", m["PIGO_TOOL_ARGS"])
	}
	if m["PIGO_TOOL_INPUT"] != "ls -la" {
		t.Errorf("PIGO_TOOL_INPUT = %q", m["PIGO_TOOL_INPUT"])
	}
}

func TestBuildEnv_toolEnd(t *testing.T) {
	hctx := &HookContext{
		Event:      "tool_end",
		WorkDir:    "/tmp",
		Model:      "gpt-4o",
		ToolName:   "bash",
		ToolArgs:   `{}`,
		ToolOutput: "hello world",
		ToolError:  true,
	}
	env := buildEnv(hctx)
	m := envToMap(env)

	if m["PIGO_TOOL_OUTPUT"] != "hello world" {
		t.Errorf("PIGO_TOOL_OUTPUT = %q", m["PIGO_TOOL_OUTPUT"])
	}
	if m["PIGO_TOOL_ERROR"] != "true" {
		t.Errorf("PIGO_TOOL_ERROR = %q", m["PIGO_TOOL_ERROR"])
	}
}

func TestBuildEnv_turnNumber(t *testing.T) {
	hctx := &HookContext{
		Event:      "turn_start",
		WorkDir:    "/tmp",
		Model:      "gpt-4o",
		TurnNumber: 3,
	}
	env := buildEnv(hctx)
	m := envToMap(env)

	if m["PIGO_TURN_NUMBER"] != "3" {
		t.Errorf("PIGO_TURN_NUMBER = %q", m["PIGO_TURN_NUMBER"])
	}
}

// envToMap converts []string{"K=V",...} to map[string]string.
func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}
```

Add `"strings"` to the test file imports.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/hooks/ -run TestBuildEnv -v`
Expected: FAIL — `buildEnv` undefined

**Step 3: Implement buildEnv**

In `pkg/hooks/hooks.go`, add:

```go
import "fmt"

// buildEnv creates environment variables for a hook subprocess.
func buildEnv(hctx *HookContext) []string {
	env := []string{
		"PIGO_EVENT=" + hctx.Event,
		"PIGO_WORK_DIR=" + hctx.WorkDir,
		"PIGO_MODEL=" + hctx.Model,
		"PIGO_USER_MESSAGE=" + hctx.UserMessage,
		"PIGO_ASSISTANT_MESSAGE=" + hctx.AssistantMessage,
	}

	if hctx.ToolName != "" {
		env = append(env, "PIGO_TOOL_NAME="+hctx.ToolName)
		env = append(env, "PIGO_TOOL_ARGS="+hctx.ToolArgs)
	}

	if hctx.Event == "tool_start" && hctx.ToolInput != "" {
		env = append(env, "PIGO_TOOL_INPUT="+hctx.ToolInput)
	}

	if hctx.Event == "tool_end" {
		env = append(env, "PIGO_TOOL_OUTPUT="+hctx.ToolOutput)
		env = append(env, fmt.Sprintf("PIGO_TOOL_ERROR=%v", hctx.ToolError))
	}

	if hctx.Event == "turn_start" || hctx.Event == "turn_end" {
		env = append(env, fmt.Sprintf("PIGO_TURN_NUMBER=%d", hctx.TurnNumber))
	}

	return env
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/hooks/ -run TestBuildEnv -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/hooks/hooks.go pkg/hooks/hooks_test.go
git commit -m "feat: add hook environment variable builder"
```

---

### Task 4: Hook Execution (Run method)

**Files:**
- Modify: `pkg/hooks/hooks.go`
- Modify: `pkg/hooks/hooks_test.go`

**Step 1: Write the failing test for blocking hook execution**

In `pkg/hooks/hooks_test.go`, append:

```go
import (
	"context"
	"os"
)

func TestRun_blockingSuccess(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name: "test",
		Hooks: map[string][]HookConfig{
			"agent_start": {{Command: "true", Blocking: &blocking}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}

	err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_blockingFailure_toolStart_cancels(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name: "gate",
		Hooks: map[string][]HookConfig{
			"tool_start": {{
				Command:  "echo 'blocked' >&2; exit 1",
				Blocking: &blocking,
			}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}

	err := mgr.Run(context.Background(), hctx)
	if err == nil {
		t.Fatal("expected error for blocked tool_start, got nil")
	}
}

func TestRun_blockingFailure_nonToolStart_continues(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name: "warn",
		Hooks: map[string][]HookConfig{
			"agent_end": {{Command: "exit 1", Blocking: &blocking}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_end", WorkDir: os.TempDir(), Model: "test"}

	err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Errorf("non-tool_start failure should not return error, got: %v", err)
	}
}

func TestRun_matchFiltering(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name: "filter",
		Hooks: map[string][]HookConfig{
			"tool_start": {{
				Command:  "exit 1",
				Match:    &MatchRule{Tool: "write"},
				Blocking: &blocking,
			}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}

	// Should NOT fail because match doesn't apply to "bash"
	err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Errorf("hook should be skipped for non-matching tool, got: %v", err)
	}
}

func TestRun_noHooksForEvent(t *testing.T) {
	plugins := []PluginConfig{{
		Name:  "empty",
		Hooks: map[string][]HookConfig{},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}

	err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_envVarsAvailable(t *testing.T) {
	blocking := true
	// This hook writes PIGO_TOOL_NAME to a temp file so we can verify it was set
	tmpFile := filepath.Join(t.TempDir(), "env-out.txt")
	cmd := fmt.Sprintf("echo $PIGO_TOOL_NAME > %s", tmpFile)
	plugins := []PluginConfig{{
		Name: "env-check",
		Hooks: map[string][]HookConfig{
			"tool_start": {{Command: cmd, Blocking: &blocking}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{
		Event:    "tool_start",
		WorkDir:  os.TempDir(),
		Model:    "test",
		ToolName: "read",
		ToolArgs: `{"path":"/tmp/foo"}`,
	}

	if err := mgr.Run(context.Background(), hctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "read" {
		t.Errorf("PIGO_TOOL_NAME = %q, want %q", got, "read")
	}
}
```

Add `"path/filepath"` and `"fmt"` to test imports.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/hooks/ -run TestRun -v`
Expected: FAIL — `Run` method undefined

**Step 3: Implement Run method**

In `pkg/hooks/hooks.go`, add:

```go
import (
	"context"
	"os"
	"os/exec"
	"time"
)

const defaultTimeout = 10

// Run executes all matching hooks for the given event context.
// For tool_start events, returns an error if a blocking hook fails (cancels tool).
// For other events, logs failures but returns nil.
func (m *HookManager) Run(ctx context.Context, hctx *HookContext) error {
	env := append(os.Environ(), buildEnv(hctx)...)

	for _, plugin := range m.plugins {
		hooks, ok := plugin.Hooks[hctx.Event]
		if !ok {
			continue
		}
		for _, hook := range hooks {
			if !matchesRule(hook.Match, hctx.ToolName) {
				continue
			}

			blocking := hook.Blocking == nil || *hook.Blocking
			timeout := hook.Timeout
			if timeout <= 0 {
				timeout = defaultTimeout
			}

			if blocking {
				if err := runBlocking(ctx, hook.Command, env, hctx.WorkDir, timeout); err != nil {
					if hctx.Event == "tool_start" {
						return fmt.Errorf("hook [%s] blocked tool: %w", plugin.Name, err)
					}
					// Non-tool_start: log and continue
					fmt.Fprintf(os.Stderr, "hook warning [%s]: %v\n", plugin.Name, err)
				}
			} else {
				runAsync(hook.Command, env, hctx.WorkDir)
			}
		}
	}
	return nil
}

// runBlocking executes a command and waits for it to finish with a timeout.
func runBlocking(ctx context.Context, command string, env []string, dir string, timeoutSec int) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = env
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf("%s: %w", msg, err)
		}
		return err
	}
	return nil
}

// runAsync starts a command in the background without waiting.
func runAsync(command string, env []string, dir string) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = env
	cmd.Dir = dir
	_ = cmd.Start()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/hooks/ -run TestRun -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/hooks/hooks.go pkg/hooks/hooks_test.go
git commit -m "feat: add hook execution with blocking, async, and match filtering"
```

---

### Task 5: Timeout Test

**Files:**
- Modify: `pkg/hooks/hooks_test.go`

**Step 1: Write the timeout test**

In `pkg/hooks/hooks_test.go`, append:

```go
func TestRun_timeout(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name: "slow",
		Hooks: map[string][]HookConfig{
			"tool_start": {{
				Command:  "sleep 30",
				Blocking: &blocking,
				Timeout:  1,
			}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}

	start := time.Now()
	err := mgr.Run(context.Background(), hctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}
```

Add `"time"` to test imports.

**Step 2: Run test to verify it passes**

Run: `go test ./pkg/hooks/ -run TestRun_timeout -v -timeout 10s`
Expected: PASS (hook times out in ~1 second)

**Step 3: Commit**

```bash
git add pkg/hooks/hooks_test.go
git commit -m "test: add hook timeout test"
```

---

### Task 6: Integrate HookManager into Agent

**Files:**
- Modify: `pkg/agent/agent.go:22-95` (Agent struct + NewAgent)
- Modify: `pkg/agent/agent.go:177-319` (ProcessInput)

**Step 1: Add HookManager to Agent struct and NewAgent**

In `pkg/agent/agent.go`:

1. Add import: `"github.com/zhy0216/pigo/pkg/hooks"`
2. Add field to Agent struct (after `events`):
   ```go
   hookMgr  *hooks.HookManager
   ```
3. In `NewAgent`, after `events.Subscribe(...)`, add:
   ```go
   hookMgr := hooks.NewHookManager(cfg.Plugins)
   ```
4. Add `hookMgr: hookMgr` to agent struct literal.

**Step 2: Add helper method for building HookContext**

In `pkg/agent/agent.go`, add after `GetModel()`:

```go
// lastMessage returns the last message with the given role, or empty string.
func (a *Agent) lastMessage(role string) string {
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == role {
			return a.messages[i].Content
		}
	}
	return ""
}

// newHookContext creates a base HookContext with common fields.
func (a *Agent) newHookContext(event string) *hooks.HookContext {
	wd, _ := os.Getwd()
	return &hooks.HookContext{
		Event:            event,
		WorkDir:          wd,
		Model:            a.client.GetModel(),
		UserMessage:      a.lastMessage("user"),
		AssistantMessage: a.lastMessage("assistant"),
	}
}
```

**Step 3: Insert hook calls into ProcessInput**

In `ProcessInput()`, make the following changes. Reference the existing line numbers from `pkg/agent/agent.go`:

**agent_start (line 194):** After `a.events.Emit(types.AgentEvent{Type: types.EventAgentStart})`, add:
```go
a.hookMgr.Run(ctx, a.newHookContext("agent_start"))
```

**turn_start (line 200):** After `a.events.Emit(types.AgentEvent{Type: types.EventTurnStart})`, add:
```go
hctxTurn := a.newHookContext("turn_start")
hctxTurn.TurnNumber = iterations + 1
a.hookMgr.Run(ctx, hctxTurn)
```

**tool_start (line 255):** Replace the simple emit with a hook call that can cancel:
```go
a.events.Emit(types.AgentEvent{Type: types.EventToolStart, ToolName: tc.Function.Name})

// Run tool_start hooks — if a blocking hook fails, skip this tool
toolStartCtx := a.newHookContext("tool_start")
toolStartCtx.ToolName = tc.Function.Name
toolStartCtx.ToolArgs = tc.Function.Arguments
if args, ok := extractToolInput(tc.Function.Name, tc.Function.Arguments); ok {
    toolStartCtx.ToolInput = args
}
if hookErr := a.hookMgr.Run(ctx, toolStartCtx); hookErr != nil {
    result := types.ErrorResult(fmt.Sprintf("blocked by hook: %v", hookErr))
    blockedMsg := types.Message{
        Role:       "tool",
        Content:    result.ForLLM,
        ToolCallID: tc.ID,
    }
    a.messages = append(a.messages, blockedMsg)
    turnMessages = append(turnMessages, blockedMsg)
    a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ToolName: tc.Function.Name})
    continue
}
```

Add helper function:
```go
// extractToolInput extracts a human-readable input string for common tools.
func extractToolInput(toolName, argsJSON string) (string, bool) {
	if toolName == "bash" {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsJSON), &args); err == nil {
			if cmd, ok := args["command"].(string); ok {
				return cmd, true
			}
		}
	}
	return "", false
}
```

**tool_end (lines 267, 295):** After each `a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ...})`, add hook call. For the main tool_end at line 295:
```go
a.events.Emit(types.AgentEvent{Type: types.EventToolEnd, ToolName: tc.Function.Name, Content: output})

toolEndCtx := a.newHookContext("tool_end")
toolEndCtx.ToolName = tc.Function.Name
toolEndCtx.ToolArgs = tc.Function.Arguments
toolEndCtx.ToolOutput = result.ForLLM
toolEndCtx.ToolError = result.IsError
a.hookMgr.Run(ctx, toolEndCtx)
```

**turn_end (line 298):** After `a.events.Emit(types.AgentEvent{Type: types.EventTurnEnd})`, add:
```go
hctxTurnEnd := a.newHookContext("turn_end")
hctxTurnEnd.TurnNumber = iterations + 1
a.hookMgr.Run(ctx, hctxTurnEnd)
```

Also after the turn_end at line 309.

**agent_end (line 318):** After `a.events.Emit(types.AgentEvent{Type: types.EventAgentEnd, ...})`, add:
```go
a.hookMgr.Run(ctx, a.newHookContext("agent_end"))
```

**Step 4: Run existing tests to verify no regressions**

Run: `go test ./pkg/agent/ -v`
Run: `go test ./... -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add pkg/agent/agent.go
git commit -m "feat: integrate HookManager into agent loop with tool interception"
```

---

### Task 7: Integration Test

**Files:**
- Modify: `pkg/hooks/hooks_test.go`

**Step 1: Write an integration test that validates the full flow**

In `pkg/hooks/hooks_test.go`, append:

```go
func TestRun_multiplePlugins_ordering(t *testing.T) {
	blocking := true
	outFile := filepath.Join(t.TempDir(), "order.txt")

	plugins := []PluginConfig{
		{
			Name: "first",
			Hooks: map[string][]HookConfig{
				"agent_start": {{
					Command:  fmt.Sprintf("echo first >> %s", outFile),
					Blocking: &blocking,
				}},
			},
		},
		{
			Name: "second",
			Hooks: map[string][]HookConfig{
				"agent_start": {{
					Command:  fmt.Sprintf("echo second >> %s", outFile),
					Blocking: &blocking,
				}},
			},
		},
	}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}

	if err := mgr.Run(context.Background(), hctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	lines := strings.TrimSpace(string(data))
	if lines != "first\nsecond" {
		t.Errorf("execution order = %q, want %q", lines, "first\nsecond")
	}
}

func TestRun_nonBlocking(t *testing.T) {
	nonBlocking := false
	outFile := filepath.Join(t.TempDir(), "async.txt")

	plugins := []PluginConfig{{
		Name: "async",
		Hooks: map[string][]HookConfig{
			"agent_end": {{
				Command:  fmt.Sprintf("echo done > %s", outFile),
				Blocking: &nonBlocking,
			}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_end", WorkDir: os.TempDir(), Model: "test"}

	// Non-blocking should return immediately without error
	if err := mgr.Run(context.Background(), hctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give the async command a moment to finish
	time.Sleep(500 * time.Millisecond)

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("async hook didn't write output: %v", err)
	}
	if strings.TrimSpace(string(data)) != "done" {
		t.Errorf("async output = %q, want %q", string(data), "done")
	}
}
```

**Step 2: Run integration tests**

Run: `go test ./pkg/hooks/ -v -timeout 30s`
Expected: All PASS

**Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add pkg/hooks/hooks_test.go
git commit -m "test: add integration tests for hook ordering and async execution"
```

---

### Task 8: Add `/plugins` Command

**Files:**
- Modify: `pkg/agent/agent.go:98-146` (HandleCommand)

**Step 1: Add `/plugins` command**

In `pkg/agent/agent.go` `HandleCommand()`, add a new case before the `return false, false` at the end of the switch block (after the `/skills` case):

```go
case "/plugins":
    plugins := a.hookMgr.GetPlugins()
    if len(plugins) == 0 {
        fmt.Fprintln(a.output, "No plugins loaded.")
    } else {
        fmt.Fprintln(a.output, "Loaded plugins:")
        for _, p := range plugins {
            hookCount := 0
            for _, hs := range p.Hooks {
                hookCount += len(hs)
            }
            fmt.Fprintf(a.output, "  %s (%d hooks)\n", p.Name, hookCount)
        }
    }
    return true, false
```

Add `GetPlugins()` method to HookManager in `pkg/hooks/hooks.go`:

```go
// GetPlugins returns the list of active plugins.
func (m *HookManager) GetPlugins() []PluginConfig {
	return m.plugins
}
```

**Step 2: Run tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 3: Commit**

```bash
git add pkg/agent/agent.go pkg/hooks/hooks.go
git commit -m "feat: add /plugins command to list loaded plugins"
```

---

### Task 9: Final Verification

**Step 1: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: All PASS, no race conditions

**Step 2: Run linting**

Run: `make lint`
Expected: No formatting issues, no vet warnings

**Step 3: Build**

Run: `make build`
Expected: Build succeeds

**Step 4: Manual smoke test**

Create a test config with a simple logging plugin:

```bash
cat > /tmp/pigo-test-config.json << 'EOF'
{
  "plugins": [
    {
      "name": "test-logger",
      "hooks": {
        "tool_start": [
          {"command": "echo \"Tool: $PIGO_TOOL_NAME Args: $PIGO_TOOL_ARGS\" >> /tmp/pigo-hooks.log"}
        ],
        "agent_end": [
          {"command": "echo 'Agent finished' >> /tmp/pigo-hooks.log"}
        ]
      }
    }
  ]
}
EOF
```

Verify the binary loads the config and hooks fire correctly.

**Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: address issues found during final verification"
```
