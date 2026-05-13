package tmux

import (
	"errors"
	"os/exec"
	"testing"
)

// TestWrapCommandError pins the shared wrap recipe extracted from runCommand
// and tmuxtest.socketCommander.wrapErr. The three covered branches mirror the
// behavioural contract the inline implementations carried:
//
//  1. nil input returns nil — the helper is safe to call unconditionally on the
//     error returned by cmd.Output() / exec.Cmd.Output().
//  2. *exec.ExitError input returns a *CommandError whose Stderr is the bytes
//     auto-populated by exec when cmd.Stderr was left nil, and whose Err is
//     the original error preserved verbatim for Unwrap traversal.
//  3. non-exec error input still returns a *CommandError so callers can
//     uniformly errors.As to recover the wrap, but Stderr is empty because
//     there is no child whose stderr could be captured.
func TestWrapCommandError(t *testing.T) {
	t.Run("nil_input_returns_nil", func(t *testing.T) {
		if got := WrapCommandError(nil); got != nil {
			t.Errorf("WrapCommandError(nil) = %v, want nil", got)
		}
	})

	t.Run("exec_exit_error_populates_stderr", func(t *testing.T) {
		// Drive a real child process so the *exec.ExitError carries its
		// auto-populated Stderr bytes (the same path exercised by
		// cmd.Output() when cmd.Stderr is nil). This keeps the test
		// honest about the wrap contract rather than synthesising an
		// *exec.ExitError by hand.
		if _, err := exec.LookPath("sh"); err != nil {
			t.Skipf("sh not available on PATH: %v", err)
		}
		const marker = "wrap-helper stderr marker"
		cmd := exec.Command("sh", "-c", `echo "`+marker+`" 1>&2; exit 1`)
		// cmd.Stderr deliberately left nil — see WrapCommandError godoc.
		_, runErr := cmd.Output()
		if runErr == nil {
			t.Fatal("expected non-nil error from sh exit 1")
		}

		wrapped := WrapCommandError(runErr)
		if wrapped == nil {
			t.Fatal("WrapCommandError returned nil for non-nil exec error")
		}
		var cmdErr *CommandError
		if !errors.As(wrapped, &cmdErr) {
			t.Fatalf("errors.As did not extract *CommandError from %v (%T)", wrapped, wrapped)
		}
		// Stderr must carry the child's stderr bytes verbatim — the
		// *exec.ExitError.Stderr field auto-populated by cmd.Output().
		if cmdErr.Stderr == "" {
			t.Errorf("CommandError.Stderr is empty; want bytes containing %q", marker)
		}
		// Err must preserve the original error so errors.As to
		// *exec.ExitError on the wrapped chain still works.
		var exitErr *exec.ExitError
		if !errors.As(cmdErr.Err, &exitErr) {
			t.Errorf("cmdErr.Err = %v (%T); expected to unwrap to *exec.ExitError", cmdErr.Err, cmdErr.Err)
		}
	})

	t.Run("non_exec_error_empty_stderr", func(t *testing.T) {
		// A plain sentinel error stands in for the exec.LookPath /
		// *exec.Error path: cmd.Output() can fail before a child is
		// spawned, in which case no stderr bytes exist to capture.
		// WrapCommandError must still return a *CommandError so callers'
		// errors.As traversal is uniform across both failure modes.
		sentinel := errors.New("plain non-exec error")

		wrapped := WrapCommandError(sentinel)
		if wrapped == nil {
			t.Fatal("WrapCommandError returned nil for non-nil non-exec error")
		}
		var cmdErr *CommandError
		if !errors.As(wrapped, &cmdErr) {
			t.Fatalf("errors.As did not extract *CommandError from %v (%T)", wrapped, wrapped)
		}
		if cmdErr.Stderr != "" {
			t.Errorf("CommandError.Stderr = %q, want empty string for non-exec error", cmdErr.Stderr)
		}
		if !errors.Is(cmdErr.Err, sentinel) {
			t.Errorf("cmdErr.Err = %v; want original sentinel preserved", cmdErr.Err)
		}
	})
}
