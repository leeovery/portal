package resolver_test

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

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

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}

func TestRealCommandRunner_Run_PathLookupErrorWrapsCleanly(t *testing.T) {
	r := &resolver.RealCommandRunner{}

	_, err := r.Run("portal-no-such-binary-xyz", "arg1")
	if err == nil {
		t.Fatalf("expected error for missing binary, got nil")
	}

	if _, ok := errors.AsType[*exec.Error](err); !ok {
		t.Errorf("errors.AsType[*exec.Error](err) = false; *exec.Error not recovered through the wrap: %v", err)
	}
}
