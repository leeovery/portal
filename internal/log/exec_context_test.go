package log_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/log"
)

// TestCombinedOutputWithContext_EmbedsArgvAndTrimmedStderrOnNonZeroExit verifies
// that on a non-zero exit the wrapped error carries the binary path, the argv
// (cmd.Args[1:]), the underlying exit-status error (%w of *exec.ExitError), and
// the trimmed child stderr.
func TestCombinedOutputWithContext_EmbedsArgvAndTrimmedStderrOnNonZeroExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo boom >&2; exit 3")

	out, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got nil (out=%q)", out)
	}

	msg := err.Error()
	if !strings.Contains(msg, "boom") {
		t.Errorf("error %q does not contain trimmed stderr %q", msg, "boom")
	}
	if !strings.Contains(msg, "sh") {
		t.Errorf("error %q does not contain binary path", msg)
	}
	if !strings.Contains(msg, "-c") || !strings.Contains(msg, "echo boom") {
		t.Errorf("error %q does not contain argv (cmd.Args[1:])", msg)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("errors.As did not recover *exec.ExitError through the wrap: %v", err)
	}
}

// TestCombinedOutputWithContext_ReturnsCapturedStdoutOnErrorPath verifies the
// defaultIdentifyPS contract: on a non-zero exit the captured stdout is still
// returned alongside the error.
func TestCombinedOutputWithContext_ReturnsCapturedStdoutOnErrorPath(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo hello; exit 3")

	out, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got nil")
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("stdout on error path = %q; want %q", string(out), "hello")
	}
}

// TestCombinedOutputWithContext_ReturnsStdoutNilErrOnSuccess verifies the happy
// path: stdout returned, nil error.
func TestCombinedOutputWithContext_ReturnsStdoutNilErrOnSuccess(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo ok")

	out, err := log.CombinedOutputWithContext(cmd)
	if err != nil {
		t.Fatalf("unexpected error on success: %v", err)
	}
	if strings.TrimSpace(string(out)) != "ok" {
		t.Errorf("stdout = %q; want %q", string(out), "ok")
	}
}

// TestCombinedOutputWithContext_EmptyStderrRendersCleanly verifies a non-zero
// exit with no stderr produces a clean wrap (no stderr noise) that remains
// errors.As-recoverable.
func TestCombinedOutputWithContext_EmptyStderrRendersCleanly(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 4")

	_, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got nil")
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("errors.As did not recover *exec.ExitError through the wrap: %v", err)
	}
}

// TestCombinedOutputWithContext_PathLookupErrorWrapsCleanly verifies a missing
// binary (*exec.Error) wraps cleanly with empty stderr and stays
// errors.As-recoverable.
func TestCombinedOutputWithContext_PathLookupErrorWrapsCleanly(t *testing.T) {
	cmd := exec.Command("portal-no-such-binary-xyz", "arg1", "arg2")

	_, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}

	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Errorf("errors.As did not recover *exec.Error through the wrap: %v", err)
	}
}
