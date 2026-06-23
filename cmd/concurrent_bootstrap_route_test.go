package cmd

// Task spectrum-tui-design-5-2 — the cold/TUI concurrent route through
// PersistentPreRunE → openTUI.
//
// On the cold + TUI path PersistentPreRunE must DEFER bootstrap (NOT run the
// orchestrator synchronously) and hand a deferred runner to openTUI, which runs
// it in a goroutine streaming progress over the channel. The warm/CLI path keeps
// the synchronous runBootstrap + serverStartedKey context + sync.Once memo
// byte-for-byte.
//
// Tests mutate package-level state (bootstrapDeps, openTUIFunc, rootCmd) and
// MUST NOT use t.Parallel.

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// coldCommander is a recordingCommander whose "info" probe fails, so
// client.ServerRunning() reports false — the cold-boot signal.
func coldCommander() *recordingCommander {
	return &recordingCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "info" {
				return "", context.DeadlineExceeded // any non-nil error == server not running
			}
			return "", nil
		},
	}
}

// TestPersistentPreRunE_ColdTUI_DefersBootstrap proves the orchestrator is NOT
// run synchronously by PersistentPreRunE on the cold + TUI path — it is deferred
// to openTUI's goroutine. The recordingRunner must see zero calls from
// PersistentPreRunE; openTUI must observe the deferred route.
func TestPersistentPreRunE_ColdTUI_DefersBootstrap(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(coldCommander())
	runner := &recordingRunner{started: true}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var deferredSeen bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, _ bool) error {
		// On the deferred route, the runner has NOT run yet inside PersistentPreRunE.
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the cold/TUI path; want deferred", runner.calls)
		}
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deferredSeen {
		t.Error("openTUI did not receive a deferred bootstrap on the cold/TUI path")
	}
}

// TestPersistentPreRunE_WarmTUI_RunsSynchronously proves the warm + TUI path
// keeps the synchronous route: the orchestrator runs in PersistentPreRunE,
// serverStarted is threaded via serverStartedKey, and NO deferred bootstrap is
// stashed.
func TestPersistentPreRunE_WarmTUI_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	// Warm: info probe succeeds → ServerRunning() == true.
	client := tmux.NewClient(&recordingCommander{})
	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var deferredSeen bool
	var serverStarted bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, started bool) error {
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		serverStarted = started
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("warm path: orchestrator calls = %d, want 1 (synchronous)", runner.calls)
	}
	if deferredSeen {
		t.Error("warm path stashed a deferred bootstrap; want synchronous route")
	}
	if serverStarted {
		t.Error("warm path threaded serverStarted=true; want false")
	}
}

// TestPersistentPreRunE_ColdCLI_RunsSynchronously proves the cold CLI/direct-path
// (a non-TUI command) is NOT routed concurrent even when the server is cold — the
// flip is scoped to the TUI path only.
func TestPersistentPreRunE_ColdCLI_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	client := tmux.NewClient(coldCommander())
	runner := &recordingRunner{started: true}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	listDeps = &ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	}
	t.Cleanup(func() { listDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("cold CLI path: orchestrator calls = %d, want 1 (synchronous, not deferred)", runner.calls)
	}
}
