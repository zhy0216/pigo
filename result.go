package main

// ToolResult represents the structured return value from tool execution.
type ToolResult struct {
	ForLLM  string `json:"for_llm"`
	ForUser string `json:"for_user,omitempty"`
	Silent  bool   `json:"silent"`
	IsError bool   `json:"is_error"`
}

// NewToolResult creates a basic ToolResult with content for the LLM.
func NewToolResult(forLLM string) *ToolResult {
	return &ToolResult{ForLLM: forLLM}
}

// SilentResult creates a ToolResult that is silent (no user message).
func SilentResult(forLLM string) *ToolResult {
	return &ToolResult{
		ForLLM: forLLM,
		Silent: true,
	}
}

// ErrorResult creates a ToolResult representing an error.
func ErrorResult(message string) *ToolResult {
	return &ToolResult{
		ForLLM:  message,
		IsError: true,
	}
}

// UserResult creates a ToolResult with content for both LLM and user.
func UserResult(content string) *ToolResult {
	return &ToolResult{
		ForLLM:  content,
		ForUser: content,
	}
}
