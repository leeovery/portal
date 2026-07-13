package spawn

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// runArgvCombined is the shared exec boundary behind both deliberately-separate
// production runner seams (execOsascriptRunner and execRecipeRunner): it execs
// argv through log.CombinedOutputWithContext (the stderr-preserving boundary
// helper) and derives exitCode from an *exec.ExitError. A clean run returns
// (stdout, 0, nil); a non-zero (or signal) exit returns the combined output
// plus the exit code with a nil err (it ran but failed); a non-exit failure
// (binary missing on PATH — no exit status) surfaces as err so the caller's
// mapping folds it to spawn-failed. Only this identical plumbing is shared —
// the two runner interfaces and their Adapters stay fully separate.
func runArgvCombined(argv []string) (out string, exitCode int, err error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	combined, runErr := log.CombinedOutputWithContext(cmd)
	if runErr == nil {
		return string(combined), 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return combineOutput(combined, runErr), exitErr.ExitCode(), nil
	}
	return string(combined), 0, runErr
}

// combineOutput folds captured stdout and the boundary helper's wrapped error
// (which embeds the child's stderr) into one opaque combined-output string —
// honouring the runner seam's out = stdout+stderr contract, since
// CombinedOutputWithContext keeps stderr inside the wrapped error rather than
// merging it into stdout.
func combineOutput(stdout []byte, wrapErr error) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(string(stdout)); s != "" {
		parts = append(parts, s)
	}
	if wrapErr != nil {
		parts = append(parts, wrapErr.Error())
	}
	return strings.Join(parts, "\n")
}

// execFailureDetail is the shared opaque-Detail formatter for a non-clean exit
// across both spawn adapters: the combined output and/or execution-error text,
// falling back to fallbackLabel (a "%d" format string for the exit code) so
// Detail is never empty. The two failure-detail wrappers differ only in that
// label, so a fix or behaviour tweak lands here once.
func execFailureDetail(out string, exitCode int, err error, fallbackLabel string) string {
	detail := strings.TrimSpace(out)
	if err != nil {
		if detail == "" {
			return err.Error()
		}
		return detail + ": " + err.Error()
	}
	if detail == "" {
		return fmt.Sprintf(fallbackLabel, exitCode)
	}
	return detail
}
