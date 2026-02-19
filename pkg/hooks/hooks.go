package hooks

import (
	"fmt"
	"strings"
)

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
