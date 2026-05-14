package tmux

import (
	"errors"
	"os/exec"
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
	Stderr string
	Err    error
}

// Error renders the wrapped error and any non-empty stderr in a human-readable
// form. The rendered format is not part of the public contract — callers that
// need to discriminate failures should inspect Stderr or use errors.As to
// recover the typed value. The three cases are:
//
//  1. Err != nil and TrimSpace(Stderr) is non-empty: "<Err>: <trimmed Stderr>".
//  2. Err != nil and Stderr is empty or whitespace-only: bare Err.Error().
//  3. Err == nil (defensive — callers should never construct this): trimmed
//     Stderr, or the literal "<no error>" if Stderr is also empty.
func (e *CommandError) Error() string {
	trimmed := strings.TrimSpace(e.Stderr)
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
func WrapCommandError(err error) error {
	if err == nil {
		return nil
	}
	var stderr string
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr = string(exitErr.Stderr)
	}
	return &CommandError{Stderr: stderr, Err: err}
}
