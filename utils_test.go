package main

import (
	"testing"
)

func TestFormatWithLineNumbers(t *testing.T) {
	content := "line 1\nline 2\nline 3\nline 4\nline 5"

	t.Run("format all lines", func(t *testing.T) {
		result := formatWithLineNumbers(content, 1, 0)
		if !contains(result, "line 1") || !contains(result, "line 5") {
			t.Errorf("expected all lines, got: %s", result)
		}
	})

	t.Run("format with offset", func(t *testing.T) {
		result := formatWithLineNumbers(content, 3, 0)
		if contains(result, "line 1") || contains(result, "line 2") {
			t.Errorf("should not contain lines 1-2, got: %s", result)
		}
		if !contains(result, "line 3") {
			t.Errorf("should contain line 3, got: %s", result)
		}
	})

	t.Run("format with limit", func(t *testing.T) {
		result := formatWithLineNumbers(content, 1, 2)
		if !contains(result, "line 1") || !contains(result, "line 2") {
			t.Errorf("should contain lines 1-2, got: %s", result)
		}
		if contains(result, "line 3") {
			t.Errorf("should not contain line 3, got: %s", result)
		}
	})
}

func TestTruncateOutput(t *testing.T) {
	t.Run("no truncation needed", func(t *testing.T) {
		result := truncateOutput("short", 100)
		if result != "short" {
			t.Errorf("expected 'short', got '%s'", result)
		}
	})

	t.Run("truncation needed", func(t *testing.T) {
		result := truncateOutput("this is a long string", 10)
		if len(result) <= 10 {
			t.Error("result should be longer than maxLen due to truncation notice")
		}
		if !contains(result, "truncated") {
			t.Errorf("should contain 'truncated', got: %s", result)
		}
	})
}

func TestValidatePath(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := validatePath("")
		if err == nil {
			t.Error("expected error for empty path")
		}
	})

	t.Run("relative path", func(t *testing.T) {
		result, err := validatePath("relative/path")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "relative/path" {
			t.Error("expected absolute path")
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		result, err := validatePath("/absolute/path")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "/absolute/path" {
			t.Errorf("expected '/absolute/path', got '%s'", result)
		}
	})
}

func TestToolResult(t *testing.T) {
	t.Run("NewToolResult", func(t *testing.T) {
		result := NewToolResult("content")
		if result.ForLLM != "content" {
			t.Errorf("expected 'content', got '%s'", result.ForLLM)
		}
		if result.Silent || result.IsError {
			t.Error("expected Silent and IsError to be false")
		}
	})

	t.Run("SilentResult", func(t *testing.T) {
		result := SilentResult("content")
		if !result.Silent {
			t.Error("expected Silent to be true")
		}
	})

	t.Run("ErrorResult", func(t *testing.T) {
		result := ErrorResult("error")
		if !result.IsError {
			t.Error("expected IsError to be true")
		}
	})

	t.Run("UserResult", func(t *testing.T) {
		result := UserResult("content")
		if result.ForLLM != result.ForUser {
			t.Error("expected ForLLM and ForUser to be equal")
		}
	})
}
