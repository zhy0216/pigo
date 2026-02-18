package main

// Limit and threshold constants used across the codebase.
// Color constants and enum-style constants remain with their respective types.
const (
	// --- Context window management ---

	// maxContextChars is the approximate character budget for the message history.
	maxContextChars = 200000

	// proactiveCompactThreshold is the fraction of maxContextChars at which
	// proactive compaction is triggered (before hitting the hard limit).
	proactiveCompactThreshold = 0.8

	// keepRecentChars is the approximate character budget for recent messages
	// to preserve during compaction (~80K chars).
	keepRecentChars = 80000

	// minKeepMessages is the minimum number of recent messages to preserve
	// during truncation.
	minKeepMessages = 10

	// maxOverflowRetries is the number of times to retry after context overflow.
	maxOverflowRetries = 2

	// maxAgentIterations is the maximum number of agent loop turns per input.
	maxAgentIterations = 10

	// --- File operations ---

	// maxReadFileSize is the maximum file size that ReadTool will load (10 MB).
	maxReadFileSize = 10 * 1024 * 1024

	// maxLineLength is the per-line truncation limit for file reads.
	maxLineLength = 500

	// --- Tool output limits ---

	// bashMaxOutput is the maximum output length for bash tool results.
	bashMaxOutput = 10000

	// bashDefaultTimeout is the default timeout in seconds for bash commands.
	bashDefaultTimeout = 120

	// grepMaxMatches is the maximum number of matches the grep tool returns.
	grepMaxMatches = 100

	// grepMaxBytes is the maximum output size in bytes for grep results.
	grepMaxBytes = 50 * 1024 // 50KB

	// grepMaxLine is the per-line truncation limit for grep output.
	grepMaxLine = 500

	// findMaxResults is the maximum number of results the find tool returns.
	findMaxResults = 1000

	// findMaxBytes is the maximum output size in bytes for find results.
	findMaxBytes = 50 * 1024 // 50KB

	// --- Session storage ---

	// sessionsDir is the directory name for storing session files.
	sessionsDir = ".pigo/sessions"
)
