package log

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// CombinedOutputWithContext runs cmd and returns its stdout. On error, it
// returns the captured stdout AND a wrapped error embedding the binary path,
// the argv (cmd.Args[1:]), the underlying exit-status (or signal) error, and
// the child's trimmed stderr.
//
// Despite the name, it does NOT merge stdout and stderr: it assigns a private
// bytes.Buffer to cmd.Stderr and calls cmd.Output(), so stdout is returned
// separately and stderr only ever appears inside the wrapped error. This split
// is load-bearing — callers such as state.defaultIdentifyPS discriminate on
// stdout emptiness on the non-zero-exit path, so the captured stdout MUST be
// returned verbatim even when err != nil.
//
// The underlying error is wrapped with %w, so errors.As against *exec.ExitError
// (non-zero exit / signal) or *exec.Error (binary missing on PATH) still
// traverses the returned error. Numeric exit codes are intentionally NOT
// assumed in the wrap: a signal-killed child renders via the *exec.ExitError's
// own String() rather than a hard-coded code, and a *exec.Error (no exit
// status, no stderr) wraps cleanly. When stderr is empty the "(stderr: ...)"
// clause is omitted to avoid dangling noise.
//
// This is Boundary class 1's shared idiom; internal/log takes a *exec.Cmd and
// imports stdlib only, so it carries no dependency on internal/state and cannot
// form an import cycle with its callers.
func CombinedOutputWithContext(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}

	trimmed := strings.TrimSpace(stderr.String())
	if trimmed == "" {
		return out, fmt.Errorf("%s %v: %w", cmd.Path, cmd.Args[1:], err)
	}
	return out, fmt.Errorf("%s %v: %w (stderr: %s)", cmd.Path, cmd.Args[1:], err, trimmed)
}
