package state

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDefaultIdentifyPS_NonExistentPIDReturnsEmptyStdout(t *testing.T) {
	out, err := defaultIdentifyPS(0x7FFFFFFE)
	if err == nil {
		t.Fatalf("expected non-zero exit for nonexistent pid, got nil (out=%q)", out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout for nonexistent pid = %q; want empty", out)
	}
}

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

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}
