package types

// Limit and threshold constants used across the codebase.
// Color constants and enum-style constants remain with their respective types.
const (
	// --- Context window management ---

	// MaxContextChars is the approximate character budget for the message history.
	MaxContextChars = 200000

	// ProactiveCompactThreshold is the fraction of MaxContextChars at which
	// proactive compaction is triggered (before hitting the hard limit).
	ProactiveCompactThreshold = 0.8

	// KeepRecentChars is the approximate character budget for recent messages
	// to preserve during compaction (~80K chars).
	KeepRecentChars = 80000

	// MinKeepMessages is the minimum number of recent messages to preserve
	// during truncation.
	MinKeepMessages = 10

	// MaxOverflowRetries is the number of times to retry after context overflow.
	MaxOverflowRetries = 2

	// MaxAgentIterations is the maximum number of agent loop turns per input.
	MaxAgentIterations = 10

	// --- File operations ---

	// MaxReadFileSize is the maximum file size that ReadTool will load (10 MB).
	MaxReadFileSize = 10 * 1024 * 1024

	// MaxLineLength is the per-line truncation limit for file reads.
	MaxLineLength = 500

	// --- Tool output limits ---

	// BashMaxOutput is the maximum output length for bash tool results.
	BashMaxOutput = 10000

	// BashDefaultTimeout is the default timeout in seconds for bash commands.
	BashDefaultTimeout = 120

	// GrepMaxMatches is the maximum number of matches the grep tool returns.
	GrepMaxMatches = 100

	// GrepMaxBytes is the maximum output size in bytes for grep results.
	GrepMaxBytes = 50 * 1024 // 50KB

	// GrepMaxLine is the per-line truncation limit for grep output.
	GrepMaxLine = 500

	// FindMaxResults is the maximum number of results the find tool returns.
	FindMaxResults = 1000

	// FindMaxBytes is the maximum output size in bytes for find results.
	FindMaxBytes = 50 * 1024 // 50KB

	// LsMaxEntries is the maximum number of entries the ls tool returns.
	LsMaxEntries = 1000

	// --- Session storage ---

	// SessionsDir is the directory name for storing session files.
	SessionsDir = ".pigo/sessions"
)
