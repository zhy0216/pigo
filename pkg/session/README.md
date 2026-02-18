# pkg/session

JSONL-based persistence for conversation sessions. Sessions are stored as files under `~/.pigo/sessions/`, one file per session, with each line being a JSON-encoded message entry.

## Storage Layout

```
~/.pigo/sessions/<session-id>.jsonl
```

Each line is a `SessionEntry` containing a timestamp and a `types.Message`. System prompt messages are excluded from saves.

## Key Types

| Type | Description |
|---|---|
| `SessionEntry` | A single persisted message with timestamp |
| `SessionInfo` | Metadata about a session: ID, modification time, message count |

## API

| Function | Description |
|---|---|
| `SessionID() string` | Generates a timestamp-based session ID (`YYYYMMDD-HHMMSS`) |
| `SaveSession(id, messages) error` | Writes messages to a JSONL file (skips system messages) |
| `LoadSession(id) ([]Message, error)` | Reads and deserializes messages from a session file |
| `ListSessions() ([]SessionInfo, error)` | Returns all sessions sorted by most recent first |

## Dependencies

- `pkg/types` — for `types.Message` and `types.SessionsDir`
- Standard library only otherwise
