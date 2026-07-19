package cmd

// Tests in this file mutate package-level state (bootstrapDeps, openTUIFunc)
// and MUST NOT use t.Parallel.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestBarePortalPrintsHelpAndDoesNotLaunchPicker is the control-plane root
// guard for the CLI verb-surface redesign (spec § "Bare portal (no
// subcommand)"): bare `portal` is the management plane — it prints help/usage
// and exits 0, and it must NOT bootstrap tmux or launch the TUI picker. The
// picker has exactly two doors — `portal open` (no-arg openCmd) and `x` — and
// bare portal is neither.
//
// Mechanism (cobra v1.10.2, github.com/spf13/cobra@v1.10.2/command.go):
// rootCmd declares a PersistentPreRunE but NO Run/RunE, so Runnable()
// (command.go:1596 — `c.Run != nil || c.RunE != nil`) is false. In execute()
// the guard `if !c.Runnable() { return flag.ErrHelp }` (command.go:955) fires
// BEFORE c.preRun() and the PersistentPreRunE loop (command.go:983), so the
// tmux-bootstrap chain never runs. ExecuteC then catches errors.Is(err,
// flag.ErrHelp) (command.go:1152), calls HelpFunc(), and returns nil — so bare
// portal prints help and exits 0.
//
// VERIFICATION-AND-GUARD: this passes against the current tree with ZERO
// production change. A future accidental Run/RunE on rootCmd would flip
// Runnable() true, reach PersistentPreRunE (bootstrapping) and — absent an
// explicit help print — regress bare portal into launching work; the
// assertions below (and the structural TestRootCmdIsNotRunnable) fail loudly.
func TestBarePortalPrintsHelpAndDoesNotLaunchPicker(t *testing.T) {
	// Inject a recordingRunner so we can prove (c) the orchestrator never runs.
	runner := &recordingRunner{}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	// (d) sentinel: bare portal must never launch the picker.
	tuiLaunched := false
	origTUI := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, _ bool) error {
		tuiLaunched = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origTUI })

	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})

	err := rootCmd.Execute()

	// (a) no error — ExecuteC maps flag.ErrHelp → help + nil (exit 0).
	if err != nil {
		t.Fatalf("bare portal returned error: %v (want nil — help printed, exit 0)", err)
	}

	// (b) help/usage text written to the command's output.
	out := buf.String()
	if !strings.Contains(out, "Usage:") && !strings.Contains(out, "Available Commands:") {
		t.Errorf("bare portal output missing help/usage text; got:\n%s", out)
	}

	// (c) the bootstrap orchestrator's Run never fired (no tmux bootstrap).
	if runner.calls != 0 {
		t.Errorf("bootstrap orchestrator Run count = %d, want 0 (bare portal must not bootstrap)", runner.calls)
	}

	// (d) the picker never launched.
	if tuiLaunched {
		t.Error("openTUIFunc was invoked for bare portal; the picker must stay behind `portal open` / `x`")
	}
}

// TestRootCmdIsNotRunnable pins the structural invariant that makes bare
// portal help-only: rootCmd declares neither Run nor RunE, so cobra's
// Runnable() is false and execute() short-circuits to flag.ErrHelp BEFORE the
// PersistentPreRunE bootstrap chain (see the mechanism comment on
// TestBarePortalPrintsHelpAndDoesNotLaunchPicker). A future accidental
// Run/RunE on the root would flip Runnable() true and regress bare portal into
// bootstrapping / launching work — this guard fails loudly if that happens.
func TestRootCmdIsNotRunnable(t *testing.T) {
	if rootCmd.Run != nil {
		t.Error("rootCmd.Run must be nil (bare portal is help-only, not runnable)")
	}
	if rootCmd.RunE != nil {
		t.Error("rootCmd.RunE must be nil (bare portal is help-only, not runnable)")
	}
	if rootCmd.Runnable() {
		t.Error("rootCmd.Runnable() must be false so execute() returns flag.ErrHelp before PersistentPreRunE")
	}
}
