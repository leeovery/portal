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
			// runCommand threads its argv into CommandError.Args (the tmux
			// subcommand + flags, NOT the binary name). The fake invocation
			// is runCommand("sh", trim, "-c", script), so the argv is
			// ["-c", script].
			if len(cmdErr.Args) != 2 || cmdErr.Args[0] != "-c" || cmdErr.Args[1] != script {
				t.Errorf("CommandError.Args = %v, want [-c %q]", cmdErr.Args, script)
			}
			// The rendered error must carry argv + exit code + trimmed stderr.
			rendered := err.Error()
			for _, want := range []string{"-c", "exit 1", marker} {
				if !strings.Contains(rendered, want) {
					t.Errorf("Error() = %q, want it to contain %q", rendered, want)
				}
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

// TestWrapNoSuchSession_ArgvChainRemainsRecoverable is the boundary-class-2
// regression lock: after argv is threaded into *CommandError, the multi-%w
// sentinel chain produced by wrapNoSuchSession over the production wrap (a
// *CommandError WITH Args, the shape runCommand now emits) must STILL recover
// both the sentinel via errors.Is AND the *CommandError (carrying Args + Stderr)
// via errors.As on the same value. The argv field touches neither Stderr nor
// Unwrap, so the contract holds — this test pins that it cannot silently regress.
func TestWrapNoSuchSession_ArgvChainRemainsRecoverable(t *testing.T) {
	// Synthesise the exact shape runCommand now produces on a "no such session"
	// failure: WrapCommandError(err, args...) → *CommandError{Stderr, Err, Args}.
	cmdErr := &CommandError{
		Stderr: "no such session: missing",
		Err:    errors.New("exit status 1"),
		Args:   []string{"show-environment", "-t", "=missing"},
	}
	chain := wrapNoSuchSession(cmdErr)

	if !errors.Is(chain, ErrNoSuchSession) {
		t.Errorf("errors.Is(chain, ErrNoSuchSession) = false, want true; chain = %v", chain)
	}
	var recovered *CommandError
	if !errors.As(chain, &recovered) {
		t.Fatalf("errors.As did not recover *CommandError from %v (%T)", chain, chain)
	}
	if recovered.Stderr != "no such session: missing" {
		t.Errorf("recovered Stderr = %q, want %q", recovered.Stderr, "no such session: missing")
	}
	wantArgs := []string{"show-environment", "-t", "=missing"}
	if len(recovered.Args) != len(wantArgs) {
		t.Fatalf("recovered Args = %v, want %v", recovered.Args, wantArgs)
	}
	for i := range wantArgs {
		if recovered.Args[i] != wantArgs[i] {
			t.Fatalf("recovered Args = %v, want %v", recovered.Args, wantArgs)
		}
	}
}

// TestRunCommand_RunRawVerbatimOnSuccess pins that the success path of RunRaw
// (trim=false through the shared runCommand seam) returns the child's stdout
// byte-identical — the argv-threading change touches only the error path.
func TestRunCommand_RunRawVerbatimOnSuccess(t *testing.T) {
	if _, err := exec.LookPath("printf"); err != nil {
		t.Skipf("printf not available on PATH: %v", err)
	}
	// Emit output with leading/trailing whitespace so a trim regression would
	// be observable. printf does not append a trailing newline.
	const want = "  line1\nline2  \n"
	out, err := runCommand("printf", false, "%s", want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != want {
		t.Errorf("RunRaw output = %q, want verbatim %q", out, want)
	}
	// Sibling: Run (trim=true) over the same output trims surrounding space.
	trimmed, err := runCommand("printf", true, "%s", want)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trimmed != strings.TrimSpace(want) {
		t.Errorf("Run output = %q, want trimmed %q", trimmed, strings.TrimSpace(want))
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
