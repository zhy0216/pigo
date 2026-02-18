package types

import "testing"

func TestNewToolResult(t *testing.T) {
	result := NewToolResult("content")
	if result.ForLLM != "content" {
		t.Errorf("expected 'content', got '%s'", result.ForLLM)
	}
	if result.Silent || result.IsError {
		t.Error("expected Silent and IsError to be false")
	}
}

func TestSilentResult(t *testing.T) {
	result := SilentResult("content")
	if !result.Silent {
		t.Error("expected Silent to be true")
	}
	if result.ForLLM != "content" {
		t.Errorf("expected 'content', got '%s'", result.ForLLM)
	}
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult("error")
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if result.ForLLM != "error" {
		t.Errorf("expected 'error', got '%s'", result.ForLLM)
	}
}

func TestUserResult(t *testing.T) {
	result := UserResult("content")
	if result.ForLLM != result.ForUser {
		t.Error("expected ForLLM and ForUser to be equal")
	}
	if result.ForLLM != "content" {
		t.Errorf("expected 'content', got '%s'", result.ForLLM)
	}
}
