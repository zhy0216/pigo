package main

// ToolResult represents the structured return value from tool execution.
// It separates content intended for the LLM from content shown to the user,
// allowing tools to control visibility and error reporting independently.
type ToolResult struct {
	ForLLM  string `json:"for_llm"`
	ForUser string `json:"for_user,omitempty"`
	Silent  bool   `json:"silent"`
	IsError bool   `json:"is_error"`
}

// NewToolResult creates a ToolResult with content only for the LLM.
// The result is not shown to the user (ForUser is empty).
// Use for tool outputs the LLM needs but the user doesn't (e.g., file contents).
func NewToolResult(forLLM string) *ToolResult {
	return &ToolResult{ForLLM: forLLM}
}

// SilentResult creates a ToolResult with an LLM confirmation message and
// no user output. Use for operations where the user already sees the effect
// (e.g., write/edit confirmations).
func SilentResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
		Silent: true,
	}
}

// ErrorResult creates a ToolResult representing an error, shown to both
// the LLM and the user.
func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		IsError: true,
	}
}

// UserResult creates a ToolResult with the same content for both the LLM
// and the user. Use when the output is relevant to both (e.g., bash output).
func UserResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
	}
}
