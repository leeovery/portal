package tmux

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// CommandError wraps an error returned by Commander.Run / Commander.RunRaw and
// carries the captured stderr from the underlying process. Stderr is empty when
// no stderr was captured.
//
// The type is exported and constructable as a plain struct literal so test
// mocks outside the package can return synthetic stderr without coupling to
// os/exec. There is intentionally no NewCommandError factory — callers and
// mocks compose the struct directly.
type CommandError struct {
	// Stderr is the child process's stderr captured by exec (empty when no
	// child stderr was captured — e.g. a PATH-lookup *exec.Error).
	Stderr string
	// Err is the underlying error returned by exec.Cmd.Output(). It is the
	// chain root for errors.Is / errors.As traversal, and the source of the
	// exit code (when it unwraps to *exec.ExitError).
	Err error
	// Args carries the child argv — the tmux subcommand and its flags (NOT the
	// "tmux" binary name). The zero value (nil) is benign: argv-less
	// construction (plain-struct-literal mocks, the legacy WrapCommandError(err)
	// call shape) leaves it nil and Error() falls back to the pre-argv
	// rendering. A populated Args lets a log site recover which tmux invocation
	// failed via errors.As without re-parsing the rendered string.
	Args []string
}

// Error renders the wrapped error in a human-readable form. The rendered format
// is NOT part of the public contract — callers that need to discriminate
// failures should inspect Stderr / Args or use errors.As to recover the typed
// value.
//
// When Args is non-empty (the production commander path) the rendering is
// "tmux <space-joined Args>[: exit <N>][: <trimmed Stderr>]", where:
//   - the "exit <N>" fragment is included only when Err unwraps to
//     *exec.ExitError (a PATH-lookup *exec.Error has no exit code, so the
//     fragment is omitted and no dangling noise is rendered);
//   - the trimmed-stderr fragment is appended only when it is non-empty.
//
// When Args is empty (argv-less construction) it falls back to the original
// pre-argv rendering so legacy literal mocks stay byte-identical:
//
//  1. Err != nil and TrimSpace(Stderr) is non-empty: "<Err>: <trimmed Stderr>".
//  2. Err != nil and Stderr is empty or whitespace-only: bare Err.Error().
//  3. Err == nil (defensive — callers should never construct this): trimmed
//     Stderr, or the literal "<no error>" if Stderr is also empty.
func (e *CommandError) Error() string {
	trimmed := strings.TrimSpace(e.Stderr)
	if len(e.Args) > 0 {
		return e.renderWithArgs(trimmed)
	}
	if e.Err == nil {
		if trimmed == "" {
			return "<no error>"
		}
		return trimmed
	}
	if trimmed == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + ": " + trimmed
}

// renderWithArgs produces the argv-aware rendering. The exit code is extracted
// only behind errors.As(*exec.ExitError) so a signal-killed child or a
// PATH-lookup *exec.Error (no numeric code) does not synthesise a bogus "exit
// N" fragment.
func (e *CommandError) renderWithArgs(trimmedStderr string) string {
	var b strings.Builder
	b.WriteString("tmux ")
	b.WriteString(strings.Join(e.Args, " "))
	var exitErr *exec.ExitError
	if errors.As(e.Err, &exitErr) {
		b.WriteString(": exit ")
		b.WriteString(strconv.Itoa(exitErr.ExitCode()))
	}
	if trimmedStderr != "" {
		b.WriteString(": ")
		b.WriteString(trimmedStderr)
	}
	return b.String()
}

// Unwrap returns the embedded error so errors.Is / errors.As chains traverse
// through *CommandError to the underlying cause.
func (e *CommandError) Unwrap() error {
	return e.Err
}

// WrapCommandError converts an error returned by exec.Cmd.Output() into the
// canonical *CommandError shape used across portal. A nil input returns nil
// so callers can invoke it unconditionally on the exec result.
//
// When err unwraps to *exec.ExitError via errors.As, the returned
// *CommandError.Stderr is populated from (*exec.ExitError).Stderr — the bytes
// exec captures from the child's stderr. For any other error (e.g.
// *exec.Error from a failed PATH lookup) Stderr is empty; the original error
// is still wrapped so callers' errors.As traversal is uniform across both
// failure modes.
//
// Precondition: the *exec.Cmd whose Output() produced err must have left
// cmd.Stderr == nil. exec.Cmd auto-populates (*exec.ExitError).Stderr only
// under that condition — assigning cmd.Stderr (e.g. to tee output) silently
// zeroes exitErr.Stderr and defeats the wrap. Callers needing stderr piped
// elsewhere must capture it explicitly (e.g. via cmd.StderrPipe()) and feed
// the captured bytes into a *CommandError struct directly.
//
// This helper is the single source of truth for the production wrap shape.
// Both internal/tmux.runCommand and internal/tmuxtest.socketCommander call
// through it so production discriminators (errors.As against *CommandError)
// behave identically against the test commander.
//
// args is the child argv (the tmux subcommand and its flags, NOT the binary
// name); it is threaded into CommandError.Args so a downstream log site can
// recover which tmux invocation failed. It is variadic so the legacy argv-less
// call shape WrapCommandError(err) keeps compiling (Args stays nil) while the
// commander call sites pass WrapCommandError(err, args...).
func WrapCommandError(err error, args ...string) error {
	if err == nil {
		return nil
	}
	var stderr string
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr = string(exitErr.Stderr)
	}
	return &CommandError{Stderr: stderr, Err: err, Args: args}
}
