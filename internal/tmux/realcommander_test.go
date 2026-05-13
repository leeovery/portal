package tmux

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestRealCommander_RunWrapsExitError drives a real shell child process that
// exits non-zero with a known stderr marker through the production exec path
// via the unexported runner helper. The behavioural contract: a non-zero exit
// returns a *CommandError whose Stderr carries the child's stderr bytes
// verbatim (no trimming on the Stderr field itself — only Error() trims).
//
// The runs_raw_variant subtest exercises RunRaw against the same shape so the
// two methods stay behaviourally identical on the error path.
func TestRealCommander_RunWrapsExitError(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not available on PATH: %v", err)
	}

	const marker = "synthetic stderr marker"
	script := `echo "` + marker + `" 1>&2; exit 1`

	cases := []struct {
		name string
		run  func() (string, error)
	}{
		{
			name: "run",
			run: func() (string, error) {
				return runCommand("sh", true, "-c", script)
			},
		},
		{
			name: "runs_raw_variant",
			run: func() (string, error) {
				return runCommand("sh", false, "-c", script)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.run()
			if err == nil {
				t.Fatalf("expected non-nil error, got out=%q err=nil", out)
			}
			var cmdErr *CommandError
			if !errors.As(err, &cmdErr) {
				t.Fatalf("errors.As did not extract *CommandError from %v (%T)", err, err)
			}
			if !strings.Contains(cmdErr.Stderr, marker) {
				t.Errorf("CommandError.Stderr = %q, want it to contain %q", cmdErr.Stderr, marker)
			}
			// Underlying error must be preserved via Unwrap so callers using
			// errors.Is against the original *exec.ExitError continue to work.
			var exitErr *exec.ExitError
			if !errors.As(cmdErr.Err, &exitErr) {
				t.Errorf("cmdErr.Err = %v (%T); expected to unwrap to *exec.ExitError", cmdErr.Err, cmdErr.Err)
			}
		})
	}
}

// TestRealCommander_RunWrapsNonExitError invokes a deterministic non-existent
// binary so cmd.Output() returns an *exec.Error (from exec.LookPath), not an
// *exec.ExitError. The wrap must still produce a *CommandError but with an
// empty Stderr — there is no child process whose stderr could be captured.
//
// The non-exit assertion uses errors.As(cmdErr.Err, &exitErr) rather than a
// direct *exec.Error type assertion so the test stays resilient to Go internal
// changes in how exec surfaces "binary not found" errors.
func TestRealCommander_RunWrapsNonExitError(t *testing.T) {
	const missing = "__portal_test_nonexistent_binary__"

	cases := []struct {
		name string
		run  func() (string, error)
	}{
		{
			name: "run",
			run: func() (string, error) {
				return runCommand(missing, true, "arg")
			},
		},
		{
			name: "runs_raw_variant",
			run: func() (string, error) {
				return runCommand(missing, false, "arg")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.run()
			if err == nil {
				t.Fatalf("expected non-nil error invoking %q, got out=%q err=nil", missing, out)
			}
			var cmdErr *CommandError
			if !errors.As(err, &cmdErr) {
				t.Fatalf("errors.As did not extract *CommandError from %v (%T)", err, err)
			}
			if cmdErr.Stderr != "" {
				t.Errorf("CommandError.Stderr = %q, want empty string for non-ExitError failure", cmdErr.Stderr)
			}
			if cmdErr.Err == nil {
				t.Fatal("CommandError.Err is nil; want underlying error preserved")
			}
			var exitErr *exec.ExitError
			if errors.As(cmdErr.Err, &exitErr) {
				t.Errorf("cmdErr.Err unexpectedly unwraps to *exec.ExitError; want non-exit error type")
			}
		})
	}
}
