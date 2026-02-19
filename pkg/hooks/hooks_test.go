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
