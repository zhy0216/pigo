package main

import (
	"context"
	"fmt"
	"time"
)

// BashTool executes shell commands.
type BashTool struct {
	execOps ExecOps
}

// NewBashTool creates a new BashTool.
func NewBashTool(execOps ExecOps) *BashTool {
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

func (t *BashTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	command, err := extractString(args, "command")
	if err != nil {
		return ErrorResult(err.Error())
	}

	timeout := extractInt(args, "timeout", 120)
	if timeout <= 0 {
		return ErrorResult("timeout must be a positive number")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	stdout, stderr, exitCode, runErr := t.execOps.Run(cmdCtx, "/bin/bash", []string{"-c", command}, sanitizeEnv())

	output := stdout
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += "STDERR:\n" + stderr
	}

	if runErr != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("Command timed out after %d seconds", timeout))
		}
		output += fmt.Sprintf("\nError: %v", runErr)
	} else if exitCode != 0 {
		output += fmt.Sprintf("\nExit code: exit status %d", exitCode)
	}

	if output == "" {
		output = "(no output)"
	}

	// Truncate long output — keep tail for bash (errors/exit status are at the end)
	output = truncateTail(output, 10000)

	if runErr != nil || exitCode != 0 {
		return &ToolResult{
			ForLLM:  output,
			ForUser: output,
			IsError: true,
		}
	}

	return UserResult(output)
}
