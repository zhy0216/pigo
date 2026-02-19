package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
					fmt.Fprintf(os.Stderr, "hook warning [%s]: %v\n", plugin.Name, err)
				}
			} else {
				runAsync(hook.Command, env, hctx.WorkDir)
			}
		}
	}
	return nil
}

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

func runAsync(command string, env []string, dir string) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = env
	cmd.Dir = dir
	_ = cmd.Start()
}
