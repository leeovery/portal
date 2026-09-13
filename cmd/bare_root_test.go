package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// rootCmd declares no Run/RunE, so cobra's execute() short-circuits it to
// flag.ErrHelp before the PersistentPreRunE bootstrap chain, and ExecuteC maps
// that to help-printed plus a nil error.
func TestBarePortalPrintsHelpAndDoesNotLaunchPicker(t *testing.T) {
	runner := &recordingRunner{}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

	tuiLaunched := false
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		tuiLaunched = true
		return nil
	})

	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})

	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("bare portal returned error: %v (want nil — help printed, exit 0)", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Usage:") && !strings.Contains(out, "Available Commands:") {
		t.Errorf("bare portal output missing help/usage text; got:\n%s", out)
	}

	if runner.calls != 0 {
		t.Errorf("bootstrap orchestrator Run count = %d, want 0 (bare portal must not bootstrap)", runner.calls)
	}

	if tuiLaunched {
		t.Error("openTUIFunc was invoked for bare portal; the picker must stay behind `portal open` / `x`")
	}
}

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
