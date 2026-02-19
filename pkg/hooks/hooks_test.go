package hooks

import (
	"strings"
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
