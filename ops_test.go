package main

import (
	"context"
	"io/fs"
	"strings"
	"testing"
)

// mockExecOps is a test implementation of ExecOps.
type mockExecOps struct {
	stdout   string
	stderr   string
	exitCode int
	err      error

	lookPathResult string
	lookPathErr    error

	lastArgs []string
}

func (m *mockExecOps) Run(_ context.Context, name string, args []string, _ []string) (string, string, int, error) {
	m.lastArgs = append([]string{name}, args...)
	return m.stdout, m.stderr, m.exitCode, m.err
}

func (m *mockExecOps) LookPath(file string) (string, error) {
	return m.lookPathResult, m.lookPathErr
}

func TestBashTool_WithMockExecOps(t *testing.T) {
	mock := &mockExecOps{stdout: "mocked output\n"}
	tool := NewBashTool(mock)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"command": "anything",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "mocked output") {
		t.Errorf("expected mocked output, got: %s", result.ForLLM)
	}
	// Verify the command was passed through
	if len(mock.lastArgs) < 3 || mock.lastArgs[2] != "anything" {
		t.Errorf("expected command 'anything' in args, got: %v", mock.lastArgs)
	}
}

func TestBashTool_MockNonZeroExit(t *testing.T) {
	mock := &mockExecOps{exitCode: 1, stdout: "some output\n"}
	tool := NewBashTool(mock)

	result := tool.Execute(context.Background(), map[string]interface{}{
		"command": "failing-cmd",
	})
	if !result.IsError {
		t.Error("expected error for non-zero exit code")
	}
	if !strings.Contains(result.ForLLM, "exit status 1") {
		t.Errorf("expected exit status message, got: %s", result.ForLLM)
	}
}

func TestRealFileOps(t *testing.T) {
	ops := &RealFileOps{}
	tmpDir := t.TempDir()

	// Test MkdirAll + WriteFile + ReadFile + Stat
	dir := tmpDir + "/a/b"
	if err := ops.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	path := dir + "/test.txt"
	if err := ops.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := ops.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("expected size 5, got %d", info.Size())
	}

	data, err := ops.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(data))
	}

	// Test ReadDir
	entries, err := ops.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "test.txt" {
		t.Errorf("expected [test.txt], got %v", entries)
	}

	// Test WalkDir
	var walked []string
	if err := ops.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		walked = append(walked, d.Name())
		return nil
	}); err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}
	// Should visit tmpDir's base, "a", "b", "test.txt"
	if len(walked) < 3 {
		t.Errorf("expected at least 3 walked entries, got %d: %v", len(walked), walked)
	}
}

func TestRealExecOps(t *testing.T) {
	ops := &RealExecOps{}

	t.Run("successful command", func(t *testing.T) {
		stdout, stderr, exitCode, err := ops.Run(context.Background(), "/bin/echo", []string{"hello"}, nil)
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}
		if strings.TrimSpace(stdout) != "hello" {
			t.Errorf("expected stdout 'hello', got '%s'", stdout)
		}
		if stderr != "" {
			t.Errorf("expected empty stderr, got '%s'", stderr)
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		_, _, exitCode, err := ops.Run(context.Background(), "/bin/bash", []string{"-c", "exit 42"}, nil)
		if err != nil {
			t.Fatalf("unexpected system error: %v", err)
		}
		if exitCode != 42 {
			t.Errorf("expected exit code 42, got %d", exitCode)
		}
	})

	t.Run("LookPath finds echo", func(t *testing.T) {
		path, err := ops.LookPath("echo")
		if err != nil {
			t.Fatalf("LookPath failed: %v", err)
		}
		if path == "" {
			t.Error("expected non-empty path for echo")
		}
	})

	t.Run("LookPath missing binary", func(t *testing.T) {
		_, err := ops.LookPath("nonexistent_binary_xyz_123")
		if err == nil {
			t.Error("expected error for missing binary")
		}
	})
}
