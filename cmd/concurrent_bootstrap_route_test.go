package cmd

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/spf13/cobra"
)

// stageGate stages runner as the bootstrap, owed as the gate answers.
func stageGate(t *testing.T, runner *recordingRunner, owed bool) {
	t.Helper()
	resetBootstrapOnce(t)
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	withBootstrapDeps(t, BootstrapDeps{
		Owed:         func(context.Context) bool { return owed },
		Orchestrator: runner,
	})
}

// runPickerRoute executes open with args under a gate answering owed, and
// reports whether openTUI was handed a bootstrap to run behind its loading page.
func runPickerRoute(t *testing.T, owed bool, args ...string) (deferred bool) {
	t.Helper()
	runner := &recordingRunner{}
	stageGate(t, runner, owed)

	reached := false
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string) error {
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the picker path; want deferred", runner.calls)
		}
		reached, deferred = true, deferredBootstrapFromContext(cmd) != nil
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs(append([]string{"open"}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reached {
		t.Fatal("openTUI was not reached")
	}
	return deferred
}

func TestPersistentPreRunE_PickerOwedABootstrap_DefersIt(t *testing.T) {
	if !runPickerRoute(t, true) {
		t.Error("openTUI did not receive a deferred bootstrap with one owed")
	}
}

func TestPersistentPreRunE_SearchFormOwedABootstrap_DefersIt(t *testing.T) {
	if !runPickerRoute(t, true, "/port") {
		t.Error("openTUI did not receive a deferred bootstrap for a search form with one owed")
	}
}

func TestPersistentPreRunE_PickerOwedNone_OpensStraightOntoList(t *testing.T) {
	if runPickerRoute(t, false) {
		t.Error("a deferred bootstrap was stashed with none owed; want none, so no loading page")
	}
}

func TestPersistentPreRunE_CLI_RunsAnOwedBootstrapSynchronously(t *testing.T) {
	for _, owed := range []bool{true, false} {
		t.Run(map[bool]string{true: "owed", false: "not owed"}[owed], func(t *testing.T) {
			runner := &recordingRunner{}
			stageGate(t, runner, owed)

			resetRootCmd()
			rootCmd.SetArgs([]string{"list"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want := map[bool]int{true: 1, false: 0}[owed]; runner.calls != want {
				t.Errorf("CLI path: orchestrator calls = %d, want %d", runner.calls, want)
			}
		})
	}
}
