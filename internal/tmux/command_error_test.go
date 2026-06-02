package tmux

import (
	"errors"
	"os/exec"
	"strings"
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

	t.Run("variadic_args_populate_CommandError_Args", func(t *testing.T) {
		// The variadic argv must thread into CommandError.Args so a log site
		// can recover which tmux invocation failed via errors.As without
		// re-parsing the rendered string.
		sentinel := errors.New("boom")
		wrapped := WrapCommandError(sentinel, "list-panes", "-t", "=missing")

		var cmdErr *CommandError
		if !errors.As(wrapped, &cmdErr) {
			t.Fatalf("errors.As did not extract *CommandError from %v (%T)", wrapped, wrapped)
		}
		want := []string{"list-panes", "-t", "=missing"}
		if len(cmdErr.Args) != len(want) {
			t.Fatalf("Args = %v, want %v", cmdErr.Args, want)
		}
		for i := range want {
			if cmdErr.Args[i] != want[i] {
				t.Fatalf("Args = %v, want %v", cmdErr.Args, want)
			}
		}
	})

	t.Run("nil_input_returns_nil_even_with_args", func(t *testing.T) {
		// nil input must still short-circuit to nil so callers can invoke the
		// wrap unconditionally on the exec result even when passing argv.
		if got := WrapCommandError(nil, "list-panes", "-t", "x"); got != nil {
			t.Errorf("WrapCommandError(nil, args...) = %v, want nil", got)
		}
	})

	t.Run("no_args_leaves_Args_nil_for_legacy_literal_parity", func(t *testing.T) {
		// Argv-less construction (the legacy WrapCommandError(err) call shape)
		// must leave Args nil so plain-struct-literal mocks stay byte-identical.
		wrapped := WrapCommandError(errors.New("boom"))
		var cmdErr *CommandError
		if !errors.As(wrapped, &cmdErr) {
			t.Fatalf("errors.As did not extract *CommandError")
		}
		if cmdErr.Args != nil {
			t.Errorf("Args = %v, want nil for argv-less wrap", cmdErr.Args)
		}
	})
}

// TestCommandError_ErrorRendering_WithArgs covers the argv-aware Error()
// rendering branch added by boundary class 2. The rendered format is NOT a
// public contract; these assertions pin the human-readable shape (argv + exit
// code + trimmed stderr) so a log line carries the failing tmux invocation.
func TestCommandError_ErrorRendering_WithArgs(t *testing.T) {
	t.Run("exit_error_renders_argv_exit_code_and_trimmed_stderr", func(t *testing.T) {
		if _, err := exec.LookPath("sh"); err != nil {
			t.Skipf("sh not available on PATH: %v", err)
		}
		cmd := exec.Command("sh", "-c", `echo "  nope  " 1>&2; exit 2`)
		// cmd.Stderr deliberately left nil — see WrapCommandError godoc.
		_, runErr := cmd.Output()
		if runErr == nil {
			t.Fatal("expected non-nil error from sh exit 2")
		}
		wrapped := WrapCommandError(runErr, "kill-session", "-t", "=foo")

		got := wrapped.Error()
		for _, want := range []string{"tmux", "kill-session", "-t", "=foo", "exit 2", "nope"} {
			if !strings.Contains(got, want) {
				t.Errorf("Error() = %q, want it to contain %q", got, want)
			}
		}
		// Stderr fragment must be trimmed in the rendered output.
		if strings.Contains(got, "  nope  ") {
			t.Errorf("Error() = %q, expected trimmed stderr (no surrounding spaces)", got)
		}
	})

	t.Run("path_lookup_error_renders_argv_with_no_exit_fragment_and_empty_stderr", func(t *testing.T) {
		// A missing binary yields an *exec.Error (no child, no exit code). The
		// rendering must show argv but omit any "exit N" fragment and carry no
		// dangling stderr noise.
		cmd := exec.Command("__portal_test_nonexistent_binary__", "arg")
		_, runErr := cmd.Output()
		if runErr == nil {
			t.Fatal("expected non-nil error invoking nonexistent binary")
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			t.Fatalf("expected non-ExitError failure, got *exec.ExitError")
		}
		wrapped := WrapCommandError(runErr, "list-sessions")

		got := wrapped.Error()
		if !strings.Contains(got, "list-sessions") {
			t.Errorf("Error() = %q, want it to contain argv 'list-sessions'", got)
		}
		if strings.Contains(got, "exit ") {
			t.Errorf("Error() = %q, want no 'exit N' fragment for *exec.Error", got)
		}
	})

	t.Run("argv_with_spaces_and_quotes_renders_intact", func(t *testing.T) {
		// Argv tokens are data — spaces, quotes, and metacharacters inside a
		// single token must render verbatim (not re-parsed or escaped).
		ce := &CommandError{
			Args:   []string{"send-keys", `echo "hello world"`, ";"},
			Err:    errors.New("exit status 1"),
			Stderr: "boom",
		}
		got := ce.Error()
		if !strings.Contains(got, `echo "hello world"`) {
			t.Errorf("Error() = %q, want it to contain the quoted/spaced token intact", got)
		}
		if !strings.Contains(got, ";") {
			t.Errorf("Error() = %q, want it to contain the ';' metacharacter token", got)
		}
	})

	t.Run("empty_args_falls_back_to_legacy_rendering", func(t *testing.T) {
		// Argv-less construction (legacy literal mocks) must render exactly as
		// before: "<Err>: <trimmed stderr>".
		ce := &CommandError{Err: errors.New("exit status 1"), Stderr: "  boom  "}
		if got, want := ce.Error(), "exit status 1: boom"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("args_present_but_empty_stderr_omits_trailing_stderr_fragment", func(t *testing.T) {
		ce := &CommandError{Args: []string{"list-panes"}, Err: errors.New("exit status 1")}
		got := ce.Error()
		if !strings.Contains(got, "tmux list-panes") {
			t.Errorf("Error() = %q, want it to contain 'tmux list-panes'", got)
		}
		// No exit fragment (plain errors.New, not *exec.ExitError) and no
		// trailing ": " dangling separator from empty stderr.
		if strings.HasSuffix(got, ": ") || strings.HasSuffix(got, ":") {
			t.Errorf("Error() = %q, want no dangling trailing separator", got)
		}
	})
}
