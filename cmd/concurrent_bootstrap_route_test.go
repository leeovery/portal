package cmd

import (
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/spf13/cobra"
)

// pickerRoute is what PersistentPreRunE handed openTUI.
type pickerRoute struct {
	reached       bool
	deferred      bool
	serverStarted bool
}

// runPickerRoute executes open with args and records the route openTUI saw,
// with the backend staged so the zmx lookup never reaches the real PATH.
func runPickerRoute(t *testing.T, runner *recordingRunner, args ...string) pickerRoute {
	t.Helper()
	resetBootstrapOnce(t)
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	if runner != nil {
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
	}

	var route pickerRoute
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, started bool) error {
		if runner != nil && runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the picker path; want deferred", runner.calls)
		}
		route = pickerRoute{reached: true, deferred: deferredBootstrapFromContext(cmd) != nil, serverStarted: started}
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs(append([]string{"open"}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !route.reached {
		t.Fatal("openTUI was not reached")
	}
	return route
}

func TestPersistentPreRunE_PickerWithRunner_DefersBootstrap(t *testing.T) {
	if route := runPickerRoute(t, &recordingRunner{started: true}); !route.deferred {
		t.Error("openTUI did not receive a deferred bootstrap with a runner injected")
	}
}

func TestPersistentPreRunE_SearchFormWithRunner_DefersBootstrap(t *testing.T) {
	withOpenDeps(t, OpenDeps{SearchSessions: &fakeSearchSource{}})

	if route := runPickerRoute(t, &recordingRunner{started: true}, "/port"); !route.deferred {
		t.Error("openTUI did not receive a deferred bootstrap for a search form with a runner injected")
	}
}

func TestPersistentPreRunE_PickerWithoutRunner_OpensStraightOntoList(t *testing.T) {
	route := runPickerRoute(t, nil)

	if route.deferred {
		t.Error("a deferred bootstrap was stashed with no runner; want none, so no loading page")
	}
	if route.serverStarted {
		t.Error("serverStarted = true with no runner; want false (no loading page)")
	}
}

func TestPersistentPreRunE_CLIWithRunner_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("CLI path: orchestrator calls = %d, want 1 (synchronous, not deferred)", runner.calls)
	}
}
