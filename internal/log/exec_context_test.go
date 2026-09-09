package log_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/log"
)

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

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}

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

func TestCombinedOutputWithContext_EmptyStderrRendersCleanly(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 4")

	_, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error on non-zero exit, got nil")
	}

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}

func TestCombinedOutputWithContext_PathLookupErrorWrapsCleanly(t *testing.T) {
	cmd := exec.Command("portal-no-such-binary-xyz", "arg1", "arg2")

	_, err := log.CombinedOutputWithContext(cmd)
	if err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}

	if _, ok := errors.AsType[*exec.Error](err); !ok {
		t.Errorf("errors.AsType[*exec.Error](err) = false; *exec.Error not recovered through the wrap: %v", err)
	}
}
