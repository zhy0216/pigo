package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sessionsDir = ".pigo/sessions"
)

// SessionEntry represents a single message persisted to a session file.
type SessionEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Message   Message   `json:"message"`
}

// sessionDir returns the directory for storing session files.
// It creates the directory if it doesn't exist.
func sessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, sessionsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create sessions directory: %w", err)
	}
	return dir, nil
}

// SessionID generates a timestamp-based session ID.
func SessionID() string {
	return time.Now().Format("20060102-150405")
}

// SaveSession writes all messages (excluding the system prompt) to a JSONL file.
func SaveSession(id string, messages []Message) error {
	dir, err := sessionDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, id+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("cannot create session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	now := time.Now()
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		entry := SessionEntry{
			Timestamp: now,
			Message:   msg,
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("cannot write session entry: %w", err)
		}
	}

	return nil
}

// LoadSession reads messages from a JSONL session file.
func LoadSession(id string) ([]Message, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, id+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open session file: %w", err)
	}
	defer f.Close()

	var messages []Message
	scanner := bufio.NewScanner(f)
	// Allow large lines (up to 1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry SessionEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, entry.Message)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading session file: %w", err)
	}

	return messages, nil
}

// SessionInfo holds metadata about a saved session.
type SessionInfo struct {
	ID       string
	ModTime  time.Time
	Messages int
}

// ListSessions returns available sessions sorted by modification time (newest first).
func ListSessions() ([]SessionInfo, error) {
	dir, err := sessionDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read sessions directory: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Count messages by counting lines
		path := filepath.Join(dir, entry.Name())
		msgCount := countLines(path)

		sessions = append(sessions, SessionInfo{
			ID:       id,
			ModTime:  info.ModTime(),
			Messages: msgCount,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

// countLines counts the number of lines in a file.
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}
