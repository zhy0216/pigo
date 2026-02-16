package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestTruncateTail(t *testing.T) {
	t.Run("no truncation needed", func(t *testing.T) {
		result := truncateTail("short", 100)
		if result != "short" {
			t.Errorf("expected 'short', got '%s'", result)
		}
	})

	t.Run("keeps tail not head", func(t *testing.T) {
		input := "AAAA" + strings.Repeat("x", 100) + "ZZZZ"
		result := truncateTail(input, 10)
		if !strings.HasSuffix(result, "ZZZZ") {
			t.Errorf("expected tail to be preserved, got: %s", result)
		}
		if strings.Contains(result, "AAAA") {
			t.Error("expected head to be truncated")
		}
	})

	t.Run("includes metadata", func(t *testing.T) {
		input := strings.Repeat("x", 200)
		result := truncateTail(input, 50)
		if !strings.Contains(result, "showing last 50 of 200") {
			t.Errorf("expected truncation metadata, got: %s", result)
		}
	})
}

func TestValidatePath(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		_, err := validatePath("", "")
		if err == nil {
			t.Error("expected error for empty path")
		}
	})

	t.Run("relative path", func(t *testing.T) {
		result, err := validatePath("relative/path", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == "relative/path" {
			t.Error("expected absolute path")
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		result, err := validatePath("/absolute/path", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "/absolute/path" {
			t.Errorf("expected '/absolute/path', got '%s'", result)
		}
	})

	t.Run("resolves symlinks", func(t *testing.T) {
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "real.txt")
		os.WriteFile(realFile, []byte("content"), 0644)

		linkFile := filepath.Join(tmpDir, "link.txt")
		os.Symlink(realFile, linkFile)

		result, err := validatePath(linkFile, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedPath, _ := filepath.EvalSymlinks(realFile)
		if result != expectedPath {
			t.Errorf("expected resolved path '%s', got '%s'", expectedPath, result)
		}
	})

	t.Run("nonexistent path returns cleaned path", func(t *testing.T) {
		result, err := validatePath("/tmp/nonexistent/file.txt", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "/tmp/nonexistent/file.txt" {
			t.Errorf("expected '/tmp/nonexistent/file.txt', got '%s'", result)
		}
	})

	t.Run("path within allowed directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
		testFile := filepath.Join(resolvedDir, "file.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		result, err := validatePath(testFile, resolvedDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != testFile {
			t.Errorf("expected '%s', got '%s'", testFile, result)
		}
	})

	t.Run("path outside allowed directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		resolvedDir, _ := filepath.EvalSymlinks(tmpDir)

		_, err := validatePath("/etc/passwd", resolvedDir)
		if err == nil {
			t.Error("expected error for path outside allowed directory")
		}
		if !contains(err.Error(), "outside the allowed directory") {
			t.Errorf("expected boundary error, got: %v", err)
		}
	})

	t.Run("dotdot traversal blocked", func(t *testing.T) {
		tmpDir := t.TempDir()
		resolvedDir, _ := filepath.EvalSymlinks(tmpDir)
		subDir := filepath.Join(resolvedDir, "sub")
		os.MkdirAll(subDir, 0755)

		// Try to escape via ../
		_, err := validatePath(filepath.Join(subDir, "..", "..", "etc", "passwd"), resolvedDir)
		if err == nil {
			t.Error("expected error for .. traversal outside allowed directory")
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

func TestExtractString(t *testing.T) {
	t.Run("present string", func(t *testing.T) {
		args := map[string]interface{}{"key": "value"}
		v, err := extractString(args, "key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "value" {
			t.Errorf("expected 'value', got '%s'", v)
		}
	})

	t.Run("empty string is valid", func(t *testing.T) {
		args := map[string]interface{}{"key": ""}
		v, err := extractString(args, "key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "" {
			t.Errorf("expected empty string, got '%s'", v)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		args := map[string]interface{}{}
		_, err := extractString(args, "key")
		if err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		args := map[string]interface{}{"key": 42}
		_, err := extractString(args, "key")
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})
}

func TestExtractInt(t *testing.T) {
	t.Run("present float64", func(t *testing.T) {
		args := map[string]interface{}{"count": float64(42)}
		v := extractInt(args, "count", 0)
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	})

	t.Run("missing key returns default", func(t *testing.T) {
		args := map[string]interface{}{}
		v := extractInt(args, "count", 10)
		if v != 10 {
			t.Errorf("expected default 10, got %d", v)
		}
	})

	t.Run("wrong type returns default", func(t *testing.T) {
		args := map[string]interface{}{"count": "not a number"}
		v := extractInt(args, "count", 5)
		if v != 5 {
			t.Errorf("expected default 5, got %d", v)
		}
	})
}

func TestExtractBool(t *testing.T) {
	t.Run("present true", func(t *testing.T) {
		args := map[string]interface{}{"flag": true}
		v := extractBool(args, "flag", false)
		if !v {
			t.Error("expected true")
		}
	})

	t.Run("present false", func(t *testing.T) {
		args := map[string]interface{}{"flag": false}
		v := extractBool(args, "flag", true)
		if v {
			t.Error("expected false")
		}
	})

	t.Run("missing returns default", func(t *testing.T) {
		args := map[string]interface{}{}
		v := extractBool(args, "flag", true)
		if !v {
			t.Error("expected default true")
		}
	})

	t.Run("wrong type returns default", func(t *testing.T) {
		args := map[string]interface{}{"flag": "yes"}
		v := extractBool(args, "flag", false)
		if v {
			t.Error("expected default false")
		}
	})
}

func TestSanitizeEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret-key")
	t.Setenv("OPENAI_BASE_URL", "https://secret.api.com")
	t.Setenv("SECRET_TOKEN", "shh")
	t.Setenv("PIGO_TEST_SAFE", "visible")

	env := sanitizeEnv()

	for _, entry := range env {
		upper := strings.ToUpper(entry)
		if strings.HasPrefix(upper, "OPENAI_") {
			t.Errorf("sensitive var leaked: %s", entry)
		}
		if strings.HasPrefix(upper, "SECRET") {
			t.Errorf("sensitive var leaked: %s", entry)
		}
	}

	found := false
	for _, entry := range env {
		if strings.HasPrefix(entry, "PIGO_TEST_SAFE=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PIGO_TEST_SAFE to be preserved")
	}
}

func TestEstimateMessageChars(t *testing.T) {
	messages := []Message{
		{Content: "hello"},
		{Content: "world", ToolCalls: []ToolCall{
			{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Arguments: "12345"}},
		}},
	}
	got := estimateMessageChars(messages)
	// "hello" (5) + "world" (5) + "12345" (5) = 15
	if got != 15 {
		t.Errorf("expected 15, got %d", got)
	}
}
