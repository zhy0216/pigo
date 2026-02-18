# pkg/ops

Abstraction interfaces for filesystem and command execution operations. Exists to enable testability — tests can inject fake implementations without touching the real OS.

## Interfaces

### FileOps

Abstracts filesystem operations.

| Method | Signature |
|---|---|
| `ReadFile` | `(path string) ([]byte, error)` |
| `WriteFile` | `(path string, data []byte, perm os.FileMode) error` |
| `Stat` | `(path string) (os.FileInfo, error)` |
| `MkdirAll` | `(path string, perm os.FileMode) error` |
| `ReadDir` | `(path string) ([]os.DirEntry, error)` |
| `WalkDir` | `(root string, fn fs.WalkDirFunc) error` |

### ExecOps

Abstracts command execution.

| Method | Signature |
|---|---|
| `Run` | `(ctx context.Context, name string, args []string, env []string) (stdout, stderr string, exitCode int, err error)` |
| `LookPath` | `(file string) (string, error)` |

## Implementations

### RealFileOps

Delegates directly to the `os` and `path/filepath` standard library packages. Zero-value usable (`&RealFileOps{}`).

### RealExecOps

Uses `os/exec` to run commands. Zero-value usable (`&RealExecOps{}`).

Error semantics for `Run`:
- Non-zero exit code: `exitCode` is set, `err` is `nil` (normal process failure)
- Context cancelled/deadline exceeded: `exitCode = -1`, `err` is the context error
- Other system failure: `exitCode = -1`, `err` is the underlying error

## Dependencies

Standard library only.
