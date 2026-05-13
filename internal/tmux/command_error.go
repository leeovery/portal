package tmux

import "strings"

// CommandError wraps an error returned by Commander.Run / Commander.RunRaw and
// carries the captured stderr from the underlying process. Stderr is empty when
// the failure was not an *exec.ExitError (e.g., executable not found).
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
