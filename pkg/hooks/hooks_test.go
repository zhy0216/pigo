package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// envToMap converts []string{"K=V",...} to map[string]string.
func envToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func TestBuildEnv_common(t *testing.T) {
	hctx := &HookContext{
		Event: "agent_start", WorkDir: "/tmp/test", Model: "gpt-4o",
		UserMessage: "hello", AssistantMessage: "hi there",
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
		Event: "tool_start", WorkDir: "/tmp", Model: "gpt-4o",
		ToolName: "bash", ToolArgs: `{"command":"ls -la"}`, ToolInput: "ls -la",
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
		Event: "tool_end", WorkDir: "/tmp", Model: "gpt-4o",
		ToolName: "bash", ToolArgs: `{}`, ToolOutput: "hello world", ToolError: true,
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
	hctx := &HookContext{Event: "turn_start", WorkDir: "/tmp", Model: "gpt-4o", TurnNumber: 3}
	env := buildEnv(hctx)
	m := envToMap(env)
	if m["PIGO_TURN_NUMBER"] != "3" {
		t.Errorf("PIGO_TURN_NUMBER = %q", m["PIGO_TURN_NUMBER"])
	}
}

func TestRun_blockingSuccess(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name:  "test",
		Hooks: map[string][]HookConfig{"agent_start": {{Command: "true", Blocking: &blocking}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	if _, err := mgr.Run(context.Background(), hctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_blockingFailure_toolStart_cancels(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name:  "gate",
		Hooks: map[string][]HookConfig{"tool_start": {{Command: "echo 'blocked' >&2; exit 1", Blocking: &blocking}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}
	if _, err := mgr.Run(context.Background(), hctx); err == nil {
		t.Fatal("expected error for blocked tool_start, got nil")
	}
}

func TestRun_blockingFailure_nonToolStart_continues(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name:  "warn",
		Hooks: map[string][]HookConfig{"agent_end": {{Command: "exit 1", Blocking: &blocking}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_end", WorkDir: os.TempDir(), Model: "test"}
	if _, err := mgr.Run(context.Background(), hctx); err != nil {
		t.Errorf("non-tool_start failure should not return error, got: %v", err)
	}
}

func TestRun_matchFiltering(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name:  "filter",
		Hooks: map[string][]HookConfig{"tool_start": {{Command: "exit 1", Match: &MatchRule{Tool: "write"}, Blocking: &blocking}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}
	if _, err := mgr.Run(context.Background(), hctx); err != nil {
		t.Errorf("hook should be skipped for non-matching tool, got: %v", err)
	}
}

func TestRun_noHooksForEvent(t *testing.T) {
	plugins := []PluginConfig{{Name: "empty", Hooks: map[string][]HookConfig{}}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	if _, err := mgr.Run(context.Background(), hctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_envVarsAvailable(t *testing.T) {
	blocking := true
	tmpFile := filepath.Join(t.TempDir(), "env-out.txt")
	cmd := fmt.Sprintf("echo $PIGO_TOOL_NAME > %s", tmpFile)
	plugins := []PluginConfig{{
		Name:  "env-check",
		Hooks: map[string][]HookConfig{"tool_start": {{Command: cmd, Blocking: &blocking}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "read", ToolArgs: `{"path":"/tmp/foo"}`}
	if _, err := mgr.Run(context.Background(), hctx); err != nil {
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

func TestRun_timeout(t *testing.T) {
	blocking := true
	plugins := []PluginConfig{{
		Name:  "slow",
		Hooks: map[string][]HookConfig{"tool_start": {{Command: "sleep 30", Blocking: &blocking, Timeout: 1}}},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "tool_start", WorkDir: os.TempDir(), Model: "test", ToolName: "bash"}
	start := time.Now()
	_, err := mgr.Run(context.Background(), hctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

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

	if _, err := mgr.Run(context.Background(), hctx); err != nil {
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

	if _, err := mgr.Run(context.Background(), hctx); err != nil {
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

func TestRun_contextHook_capturesStdout(t *testing.T) {
	plugins := []PluginConfig{{
		Name: "git-context",
		Hooks: map[string][]HookConfig{
			"agent_start": {{Command: "echo 'Branch: main'", Type: "context"}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	contexts, err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("expected 1 context entry, got %d", len(contexts))
	}
	if contexts[0].Plugin != "git-context" {
		t.Errorf("plugin = %q, want %q", contexts[0].Plugin, "git-context")
	}
	if contexts[0].Content != "Branch: main" {
		t.Errorf("content = %q, want %q", contexts[0].Content, "Branch: main")
	}
}

func TestRun_contextHook_emptyStdout_skipped(t *testing.T) {
	plugins := []PluginConfig{{
		Name: "empty-ctx",
		Hooks: map[string][]HookConfig{
			"agent_start": {{Command: "true", Type: "context"}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	contexts, err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected 0 context entries for empty stdout, got %d", len(contexts))
	}
}

func TestRun_contextHook_failure_skipped(t *testing.T) {
	plugins := []PluginConfig{{
		Name: "fail-ctx",
		Hooks: map[string][]HookConfig{
			"agent_start": {{Command: "exit 1", Type: "context"}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	contexts, err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Fatalf("expected no error for failed context hook, got: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected 0 context entries for failed hook, got %d", len(contexts))
	}
}

func TestRun_contextHook_mixedWithRegular(t *testing.T) {
	blocking := true
	outFile := filepath.Join(t.TempDir(), "mixed.txt")
	plugins := []PluginConfig{{
		Name: "mixed",
		Hooks: map[string][]HookConfig{
			"agent_start": {
				{Command: fmt.Sprintf("echo regular >> %s", outFile), Blocking: &blocking},
				{Command: "echo 'context output'", Type: "context"},
			},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	contexts, err := mgr.Run(context.Background(), hctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Regular hook should have written to file
	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("regular hook didn't write output: %v", readErr)
	}
	if strings.TrimSpace(string(data)) != "regular" {
		t.Errorf("regular hook output = %q, want %q", strings.TrimSpace(string(data)), "regular")
	}
	// Context hook should have returned entry
	if len(contexts) != 1 {
		t.Fatalf("expected 1 context entry, got %d", len(contexts))
	}
	if contexts[0].Content != "context output" {
		t.Errorf("context content = %q, want %q", contexts[0].Content, "context output")
	}
}

func TestRun_contextHook_timeout(t *testing.T) {
	plugins := []PluginConfig{{
		Name: "slow-ctx",
		Hooks: map[string][]HookConfig{
			"agent_start": {{Command: "sleep 30", Type: "context", Timeout: 1}},
		},
	}}
	mgr := NewHookManager(plugins)
	hctx := &HookContext{Event: "agent_start", WorkDir: os.TempDir(), Model: "test"}
	start := time.Now()
	contexts, err := mgr.Run(context.Background(), hctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected no error for timed out context hook, got: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected 0 context entries for timed out hook, got %d", len(contexts))
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}
