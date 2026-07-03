package cmd

// Tests in this file mutate package-level state (bootstrapDeps) and the
// shared rootCmd, and MUST NOT use t.Parallel.
//
// The cold-vs-warm routing gate, re-keyed for skip-bootstrap-when-warm.
// shouldRunConcurrentBootstrap now decides the concurrent + loading-screen route
// off the caller-supplied latch verdict, NOT its own has-server probe: it fires
// on the TUI path (`portal open`, zero args) with a non-nil client whenever the
// latch is NOT satisfied — i.e. whenever a FULL bootstrap must run behind the
// loading screen. The retired ServerRunning() probe is gone, so the decider
// issues zero tmux round-trips.

import (
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// openProbeCmd builds an "open"-named cobra command. The name "open" is
// load-bearing: isTUIPath keys off cmd.Name() == "open".
func openProbeCmd() *cobra.Command {
	return &cobra.Command{Use: "open"}
}

// probeClient returns a non-nil *tmux.Client for the decider unit tests. The
// re-keyed decider issues ZERO tmux round-trips (the has-server probe is gone),
// so the backing commander is never called — a plain recording commander suffices.
func probeClient() *tmux.Client {
	return tmux.NewClient(&recordingCommander{})
}

func TestShouldRunConcurrentBootstrap(t *testing.T) {
	t.Run("it routes concurrent for warm-unlatched open (TUI, not satisfied)", func(t *testing.T) {
		if !shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, probeClient(), false) {
			t.Error("TUI + not satisfied: shouldRunConcurrentBootstrap = false, want true")
		}
	})

	t.Run("it routes non-concurrent when the latch is satisfied", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, probeClient(), true) {
			t.Error("TUI + satisfied: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("it routes non-concurrent for a nil client", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, nil, false) {
			t.Error("nil client: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("it routes non-concurrent for a non-TUI command", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(&cobra.Command{Use: "list"}, []string{}, probeClient(), false) {
			t.Error("non-TUI command: shouldRunConcurrentBootstrap = true, want false (not the TUI path)")
		}
	})

	t.Run("it routes non-concurrent for a direct-path open", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{"~/dir"}, probeClient(), false) {
			t.Error("direct-path open: shouldRunConcurrentBootstrap = true, want false")
		}
	})
}

// TestShouldRunConcurrentBootstrap_IssuesNoProbe proves the re-keyed decider is
// pure: the retired has-server `info` probe is gone, so it issues ZERO tmux
// round-trips on EVERY path (the route is decided by the caller-supplied
// latchSatisfied verdict, not by probing the client). Previously the TUI path
// paid exactly one sanctioned `info` round-trip; now it pays none.
func TestShouldRunConcurrentBootstrap_IssuesNoProbe(t *testing.T) {
	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"non-TUI command", &cobra.Command{Use: "list"}, []string{}},
		{"direct-path open", openProbeCmd(), []string{"/dir"}},
		{"TUI path", openProbeCmd(), []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingCommander{}
			client := tmux.NewClient(rec)
			_ = shouldRunConcurrentBootstrap(tc.cmd, tc.args, client, false)
			if len(rec.Calls) != 0 {
				t.Errorf("%s: decider issued %d tmux round-trips %v, want 0", tc.name, len(rec.Calls), rec.Calls)
			}
		})
	}
}

// TestWithServerStarted_GatesLoadingPage confirms the warm path
// (serverStarted=false) NEVER lands on PageLoading — it goes straight to the
// picker, exactly as today. The concurrent flip is scoped to cold only, so
// this gating must remain intact.
func TestWithServerStarted_GatesLoadingPage(t *testing.T) {
	t.Run("warm (serverStarted=false) starts on PageSessions, never PageLoading", func(t *testing.T) {
		m := tui.New(&mockSessionLister{}, tui.WithServerStarted(false))

		if m.ActivePage() == tui.PageLoading {
			t.Error("warm path landed on PageLoading; want PageSessions")
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
	})

	t.Run("cold (serverStarted=true) starts on PageLoading", func(t *testing.T) {
		m := tui.New(&mockSessionLister{}, tui.WithServerStarted(true))

		if m.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading (%d)", m.ActivePage(), tui.PageLoading)
		}
	})
}

// TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously proves a latch-satisfied
// TUI open keeps the synchronous route: serverStarted=false is threaded to
// openTUI (no deferred bootstrap, no loading page) and the orchestrator (a
// recording fake) runs exactly once.
func TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	// Transitional (task 2-2): PersistentPreRunE reads @portal-bootstrapped to
	// compute latchSatisfied before routing. Return the running version so the
	// latch is SATISFIED — keeping this the synchronous (non-concurrent) route
	// while the abridged branch is not yet wired (task 2-3 owns that upstream).
	rec := &recordingCommander{
		RunFunc: func(_ ...string) (string, error) { return version, nil },
	}
	client := tmux.NewClient(rec)
	runner := &recordingRunner{started: false} // warm: server already running
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var capturedServerStarted bool
	var openTUIReached bool
	origFunc := openTUIFunc
	openTUIFunc = func(_ *cobra.Command, _ string, _ []string, serverStarted bool) error {
		capturedServerStarted = serverStarted
		openTUIReached = true
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !openTUIReached {
		t.Fatal("openTUI was not reached on the synchronous TUI path")
	}
	if capturedServerStarted {
		t.Error("synchronous path threaded serverStarted=true to openTUI; want false (no loading page)")
	}
	if runner.calls != 1 {
		t.Errorf("synchronous path: orchestrator calls = %d, want 1", runner.calls)
	}
	// NOTE(2-3): retune exact tmux-call count once the single upstream latch-read
	// lands. Transitionally the sole seam tmux call is the @portal-bootstrapped
	// latch read (show-option) that replaced the retired has-server info probe.
	if len(rec.Calls) != 1 || rec.Calls[0][0] != "show-option" {
		t.Errorf("synchronous path seam calls = %v, want exactly one show-option latch read", rec.Calls)
	}
}
