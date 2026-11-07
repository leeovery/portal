package state

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// The test binary runs from an absolute path well past 16 bytes, as the saver
// runs portal, so a comm cut short or read as a path would fail here.
func TestDefaultIdentifyPS_NamesTheExecutedFileWhateverItsPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if len(exe) <= 16 {
		t.Skipf("the test binary's path %q is too short to tell", exe)
	}

	out, err := defaultIdentifyPS(os.Getpid())
	if err != nil {
		t.Fatalf("ps on this process: %v", err)
	}
	comm, argv, ok := splitCommAndArgv(strings.TrimSpace(out))
	if !ok {
		t.Fatalf("ps output %q is not comm then argv", out)
	}

	if want := filepath.Base(exe); comm != want {
		t.Errorf("comm = %q, want the executable's name %q, not its path", comm, want)
	}
	if !strings.HasPrefix(argv, os.Args[0]) {
		t.Errorf("argv = %q, want this process's, from %q", argv, os.Args[0])
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
	if !strings.Contains(msg, "ucomm=,args=") {
		t.Errorf("error %q does not contain ps argv", msg)
	}

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}
