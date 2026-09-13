package cmd

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

func coldCommander() *commandertest.Scripted {
	return commandertest.Quiet(
		commandertest.Fails(context.DeadlineExceeded, "info"),
	)
}

func TestPersistentPreRunE_ColdTUI_DefersBootstrap(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(coldCommander())
	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var deferredSeen bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the cold/TUI path; want deferred", runner.calls)
		}
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deferredSeen {
		t.Error("openTUI did not receive a deferred bootstrap on the cold/TUI path")
	}
}

func TestPersistentPreRunE_LatchedTUI_TakesAbridgedPath(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	client := tmux.NewClient(satisfiedLatchAliveSaverCommander())
	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var deferredSeen bool
	var serverStarted bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, started bool) error {
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		serverStarted = started
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("abridged path: orchestrator calls = %d, want 0 (never runs the full bootstrap)", runner.calls)
	}
	if deferredSeen {
		t.Error("abridged path stashed a deferred bootstrap; want none (serverStarted=false must survive to the instant-picker gate)")
	}
	if serverStarted {
		t.Error("abridged path threaded serverStarted=true; want false (no loading page)")
	}
}

func TestPersistentPreRunE_ColdCLI_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(coldCommander())
	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	withListDeps(t, ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("cold CLI path: orchestrator calls = %d, want 1 (synchronous, not deferred)", runner.calls)
	}
}
