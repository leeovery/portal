package resolver_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// TestRealCommandRunner_Run_Success verifies the happy path returns stdout
// verbatim with a nil error.
func TestRealCommandRunner_Run_Success(t *testing.T) {
	r := &resolver.RealCommandRunner{}

	out, err := r.Run("sh", "-c", "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("stdout = %q; want %q", out, "hello")
	}
}

// TestRealCommandRunner_Run_EmbedsArgvAndStderrOnNonZeroExit verifies the
// wrapped error carries the argv and the child's trimmed stderr on a non-zero
// exit, and remains errors.As-recoverable to *exec.ExitError.
func TestRealCommandRunner_Run_EmbedsArgvAndStderrOnNonZeroExit(t *testing.T) {
	r := &resolver.RealCommandRunner{}

	out, err := r.Run("sh", "-c", "echo gitfail >&2; exit 1")
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got nil (out=%q)", out)
	}
	if out != "" {
		t.Errorf("stdout on error path = %q; want empty", out)
	}

	msg := err.Error()
	if !strings.Contains(msg, "gitfail") {
		t.Errorf("error %q does not contain trimmed stderr", msg)
	}
	if !strings.Contains(msg, "-c") {
		t.Errorf("error %q does not contain argv", msg)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("errors.As did not recover *exec.ExitError through the wrap: %v", err)
	}
}

// TestRealCommandRunner_Run_PathLookupErrorWrapsCleanly verifies a missing
// binary wraps cleanly (empty stderr) and stays errors.As-recoverable to
// *exec.Error.
func TestRealCommandRunner_Run_PathLookupErrorWrapsCleanly(t *testing.T) {
	r := &resolver.RealCommandRunner{}

	_, err := r.Run("portal-no-such-binary-xyz", "arg1")
	if err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}

	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Errorf("errors.As did not recover *exec.Error through the wrap: %v", err)
	}
}
