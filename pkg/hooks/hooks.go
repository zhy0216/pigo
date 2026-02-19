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
