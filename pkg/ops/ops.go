package ops

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// FileOps abstracts filesystem operations for testability.
type FileOps interface {
	ReadFile(path string) ([]byte, error)
	Open(path string) (io.ReadCloser, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
	WalkDir(root string, fn fs.WalkDirFunc) error
}

// ExecOps abstracts command execution for testability.
type ExecOps interface {
	// Run executes a command and returns its output.
	// exitCode is the process exit code (0 = success).
	// err is non-nil only for system-level failures (context cancelled, etc.).
	Run(ctx context.Context, name string, args []string, env []string) (stdout, stderr string, exitCode int, err error)
	// LookPath searches for an executable in PATH.
	LookPath(file string) (string, error)
}

// RealFileOps implements FileOps using the real filesystem.
type RealFileOps struct{}

func (r *RealFileOps) ReadFile(path string) ([]byte, error)    { return os.ReadFile(path) }
func (r *RealFileOps) Open(path string) (io.ReadCloser, error) { return os.Open(path) }
func (r *RealFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (r *RealFileOps) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (r *RealFileOps) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (r *RealFileOps) ReadDir(path string) ([]os.DirEntry, error)   { return os.ReadDir(path) }
func (r *RealFileOps) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

// RealExecOps implements ExecOps using os/exec.
type RealExecOps struct{}

func (r *RealExecOps) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r *RealExecOps) Run(ctx context.Context, name string, args []string, env []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = env
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return stdoutBuf.String(), stderrBuf.String(), -1, ctx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), -1, err
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}
