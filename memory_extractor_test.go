package main

import (
	"strings"
	"testing"
)

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no fence", `[{"category": "profile"}]`, `[{"category": "profile"}]`},
		{"json fence", "```json\n[{\"category\": \"profile\"}]\n```", `[{"category": "profile"}]`},
		{"plain fence", "```\n[{\"category\": \"profile\"}]\n```", `[{"category": "profile"}]`},
		{"with whitespace", "  ```json\n[]\n```  ", `[]`},
		{"no closing fence", "```json\n[]", `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFence(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestFormatMessagesForExtraction(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there"},
		{Role: "assistant", ToolCalls: []ToolCall{{Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "read", Arguments: `{"path": "/tmp/test.go"}`}}}},
		{Role: "tool", Content: "file contents here"},
	}

	result := formatMessagesForExtraction(messages)
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !strings.Contains(result, "User: Hello world") {
		t.Error("should contain user message")
	}
	if !strings.Contains(result, "Assistant: Hi there") {
		t.Error("should contain assistant message")
	}
	if !strings.Contains(result, "Tool call: read") {
		t.Error("should contain tool call")
	}
	if !strings.Contains(result, "Tool result: file contents here") {
		t.Error("should contain tool result")
	}
}

func TestFormatMessagesForExtractionEmpty(t *testing.T) {
	result := formatMessagesForExtraction(nil)
	if result != "" {
		t.Errorf("expected empty result for nil messages, got %q", result)
	}
}
