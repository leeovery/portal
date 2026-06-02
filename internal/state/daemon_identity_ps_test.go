package state

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestDefaultIdentifyPS_NonExistentPIDReturnsEmptyStdout verifies the
// IdentifyDaemon pid-not-found contract holds through defaultIdentifyPS: a
// virtually-guaranteed-nonexistent pid makes ps exit non-zero with empty
// stdout, so the returned stdout is empty (which IdentifyDaemon classifies as
// IdentifyDead).
func TestDefaultIdentifyPS_NonExistentPIDReturnsEmptyStdout(t *testing.T) {
	out, err := defaultIdentifyPS(0x7FFFFFFE)
	if err == nil {
		t.Fatalf("expected non-zero exit for nonexistent pid, got nil (out=%q)", out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout for nonexistent pid = %q; want empty", out)
	}
}

// TestDefaultIdentifyPS_ErrorEmbedsPSArgv verifies the wrapped error carries the
// ps binary path and argv on non-zero exit, and remains errors.As-recoverable
// to *exec.ExitError.
func TestDefaultIdentifyPS_ErrorEmbedsPSArgv(t *testing.T) {
	_, err := defaultIdentifyPS(0x7FFFFFFE)
	if err == nil {
		t.Fatalf("expected error for nonexistent pid, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ps") {
		t.Errorf("error %q does not contain ps binary path", msg)
	}
	if !strings.Contains(msg, "comm=,args=") {
		t.Errorf("error %q does not contain ps argv", msg)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("errors.As did not recover *exec.ExitError through the wrap: %v", err)
	}
}
