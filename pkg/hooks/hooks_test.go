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
