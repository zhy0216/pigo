package agent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhy0216/pigo/pkg/types"
)

// autoHealTimeout is the maximum time to wait for a build/lint check.
const autoHealTimeout = 15 * time.Second

// fileModifyingTools are tool names that can modify files.
var fileModifyingTools = map[string]bool{
	"edit":  true,
	"write": true,
}

// buildCheckByExt maps file extensions to build/lint commands.
var buildCheckByExt = map[string][]string{
	".go": {"go", "build", "./..."},
}

// autoHeal checks if any file-modifying tool calls affected source files and
// runs appropriate build checks. Returns error messages to feed back to the agent.
func (a *Agent) autoHeal(ctx context.Context, toolCalls []types.ToolCall, results []toolCallResult) string {
	// Collect modified file paths
	var modifiedFiles []string
	for i, tc := range toolCalls {
		if !fileModifyingTools[tc.Function.Name] {
			continue
		}
		if results[i].result != nil && results[i].result.IsError {
			continue // skip tools that already failed
		}
		path := extractPathFromArgs(tc.Function.Arguments)
		if path != "" {
			modifiedFiles = append(modifiedFiles, path)
		}
	}

	if len(modifiedFiles) == 0 {
		return ""
	}

	// Determine which checks to run based on file extensions
	checksToRun := map[string][]string{}
	for _, f := range modifiedFiles {
		ext := filepath.Ext(f)
		if cmd, ok := buildCheckByExt[ext]; ok {
			key := strings.Join(cmd, " ")
			checksToRun[key] = cmd
		}
	}

	if len(checksToRun) == 0 {
		return ""
	}

	// Run each check
	var errors []string
	for _, cmd := range checksToRun {
		checkCtx, cancel := context.WithTimeout(ctx, autoHealTimeout)
		c := exec.CommandContext(checkCtx, cmd[0], cmd[1:]...)
		output, err := c.CombinedOutput()
		cancel()

		if err != nil && len(output) > 0 {
			// Truncate large output
			out := string(output)
			if len(out) > 3000 {
				out = out[:3000] + "\n... (truncated)"
			}
			errors = append(errors, fmt.Sprintf("[auto-heal] `%s` failed:\n%s", strings.Join(cmd, " "), out))
		}
	}

	if len(errors) == 0 {
		return ""
	}

	return strings.Join(errors, "\n\n")
}
