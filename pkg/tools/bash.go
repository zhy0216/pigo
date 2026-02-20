package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/zhy0216/pigo/pkg/ops"
	"github.com/zhy0216/pigo/pkg/types"
	"github.com/zhy0216/pigo/pkg/util"
)

// BashTool executes shell commands.
type BashTool struct {
	execOps ops.ExecOps
}

// NewBashTool creates a new BashTool.
func NewBashTool(execOps ops.ExecOps) *BashTool {
	return &BashTool{execOps: execOps}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return "Execute a shell command and return its output. Use with caution."
}

func (t *BashTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default: 120)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]interface{}) *types.ToolResult {
	command, err := util.ExtractString(args, "command")
	if err != nil {
		return types.ErrorResult(err.Error())
	}

	timeout := util.ExtractInt(args, "timeout", types.BashDefaultTimeout)
	if timeout <= 0 {
		return types.ErrorResult("timeout must be a positive number")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	stdout, stderr, exitCode, runErr := t.execOps.Run(cmdCtx, "/bin/bash", []string{"-c", command}, util.SanitizeEnv())

	output := stdout
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += "STDERR:\n" + stderr
	}

	if runErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return types.ErrorResult(fmt.Sprintf("Command timed out after %d seconds", timeout))
		}
		output += fmt.Sprintf("\nError: %v", runErr)
	} else if exitCode != 0 {
		output += fmt.Sprintf("\nExit code: exit status %d", exitCode)
	}

	if output == "" {
		output = "(no output)"
	}

	output = util.TruncateTail(output, types.BashMaxOutput)

	if runErr != nil || exitCode != 0 {
		return &types.ToolResult{
			ForLLM:  output,
			ForUser: output,
			IsError: true,
		}
	}

	return types.UserResult(output)
}
