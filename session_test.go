package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionSaveAndLoad(t *testing.T) {
	// Use a temp directory for sessions
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	messages := []Message{
		{Role: "system", Content: "You are a helper."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "4"},
	}

	id := "test-session-001"
	err := SaveSession(id, messages)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Verify file exists
	dir := filepath.Join(tmpDir, sessionsDir)
	path := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("session file not created at %s", path)
	}

	// Load it back
	loaded, err := LoadSession(id)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// System prompt should be excluded
	if len(loaded) != 4 {
		t.Fatalf("expected 4 messages (no system), got %d", len(loaded))
	}

	if loaded[0].Role != "user" || loaded[0].Content != "Hello" {
		t.Errorf("expected first loaded message to be user 'Hello', got %s '%s'", loaded[0].Role, loaded[0].Content)
	}
	if loaded[1].Role != "assistant" || loaded[1].Content != "Hi there!" {
		t.Errorf("expected second message to be assistant 'Hi there!', got %s '%s'", loaded[1].Role, loaded[1].Content)
	}
}

func TestSessionSaveWithToolCalls(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	messages := []Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "list files"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "bash", Arguments: `{"command":"ls"}`},
				},
			},
		},
		{Role: "tool", Content: "file1.go\nfile2.go", ToolCallID: "call_1"},
		{Role: "assistant", Content: "I found 2 files."},
	}

	id := "test-toolcalls"
	err := SaveSession(id, messages)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	loaded, err := LoadSession(id)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	if len(loaded) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(loaded))
	}

	// Verify tool call is preserved
	if len(loaded[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(loaded[1].ToolCalls))
	}
	if loaded[1].ToolCalls[0].Function.Name != "bash" {
		t.Errorf("expected tool call name 'bash', got '%s'", loaded[1].ToolCalls[0].Function.Name)
	}

	// Verify tool result is preserved
	if loaded[2].ToolCallID != "call_1" {
		t.Errorf("expected tool call ID 'call_1', got '%s'", loaded[2].ToolCallID)
	}
}

func TestLoadSessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, err := LoadSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a couple of sessions
	messages := []Message{
		{Role: "user", Content: "hello"},
	}

	if err := SaveSession("session-a", messages); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}
	if err := SaveSession("session-b", messages); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Verify both sessions are present
	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids["session-a"] || !ids["session-b"] {
		t.Errorf("expected both sessions, got: %v", ids)
	}
}

func TestListSessionsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestSessionID(t *testing.T) {
	id := SessionID()
	if len(id) == 0 {
		t.Error("expected non-empty session ID")
	}
	// Should look like "20060102-150405"
	if len(id) != 15 {
		t.Errorf("expected 15 char session ID, got %d: %s", len(id), id)
	}
}

func TestHandleCommandSave(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	app.messages = append(app.messages, Message{Role: "user", Content: "hello"})

	handled, exit := app.HandleCommand("/save test-save")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /save")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Session saved: test-save")) {
		t.Errorf("expected save confirmation, got: %s", buf.String())
	}
}

func TestHandleCommandSaveAutoID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	handled, exit := app.HandleCommand("/save")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /save with no name")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Session saved:")) {
		t.Errorf("expected save confirmation, got: %s", buf.String())
	}
}

func TestHandleCommandLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a session first
	messages := []Message{
		{Role: "user", Content: "saved message"},
		{Role: "assistant", Content: "saved reply"},
	}
	if err := SaveSession("test-load", messages); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	handled, exit := app.HandleCommand("/load test-load")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /load")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Session loaded: test-load")) {
		t.Errorf("expected load confirmation, got: %s", buf.String())
	}

	// Should have system prompt + loaded messages
	if len(app.messages) != 3 {
		t.Errorf("expected 3 messages (system + 2 loaded), got %d", len(app.messages))
	}
	if app.messages[0].Role != "system" {
		t.Error("expected first message to be system prompt")
	}
	if app.messages[1].Content != "saved message" {
		t.Errorf("expected loaded user message, got: %s", app.messages[1].Content)
	}
}

func TestHandleCommandSessions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &Config{APIKey: "test", Model: "gpt-4"}
	app := NewApp(cfg)
	buf := &bytes.Buffer{}
	app.output = buf

	// No sessions
	handled, exit := app.HandleCommand("/sessions")
	if !handled || exit {
		t.Error("expected handled=true, exit=false for /sessions")
	}
	if !bytes.Contains(buf.Bytes(), []byte("No saved sessions")) {
		t.Errorf("expected 'No saved sessions', got: %s", buf.String())
	}

	// Save one and list
	buf.Reset()
	SaveSession("my-session", []Message{{Role: "user", Content: "hi"}})
	app.HandleCommand("/sessions")
	if !bytes.Contains(buf.Bytes(), []byte("my-session")) {
		t.Errorf("expected 'my-session' in listing, got: %s", buf.String())
	}
}
