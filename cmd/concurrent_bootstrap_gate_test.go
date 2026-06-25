package cmd

// Tests in this file mutate package-level state (bootstrapDeps) and the
// shared rootCmd, and MUST NOT use t.Parallel.
//
// Task spectrum-tui-design-5-1 — the cold-vs-warm routing gate.
// shouldRunConcurrentBootstrap must classify ONLY the cold + TUI path for the
// concurrent bootstrap route. Cold is decided by a cheap `has-server` probe
// (client.ServerRunning(), §10.1) BEFORE bootstrap runs — the flip defers the
// orchestrator into a goroutine, so the post-bootstrap serverStarted signal is
// not yet available. The probe is gated behind isTUIPath so non-TUI commands and
// direct-path opens never probe.

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

// runningClient returns a *tmux.Client whose `info` probe SUCCEEDS, so
// ServerRunning() == true (warm). coldClient returns one whose `info` FAILS, so
// ServerRunning() == false (cold).
func runningClient() *tmux.Client {
	return tmux.NewClient(&recordingCommander{})
}

func coldClient() *tmux.Client {
	return tmux.NewClient(coldCommander())
}

func TestShouldRunConcurrentBootstrap(t *testing.T) {
	t.Run("cold + TUI (server not running, zero args) routes concurrent", func(t *testing.T) {
		if !shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, coldClient()) {
			t.Error("cold + TUI: shouldRunConcurrentBootstrap = false, want true")
		}
	})

	t.Run("warm (server running, zero args) routes synchronous", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, runningClient()) {
			t.Error("warm: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("cold + CLI/direct-path (server not running, path arg) routes synchronous", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{"~/dir"}, coldClient()) {
			t.Error("cold + direct-path: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("non-open command (cold, zero args) routes synchronous", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(&cobra.Command{Use: "list"}, []string{}, coldClient()) {
			t.Error("list: shouldRunConcurrentBootstrap = true, want false (not the TUI path)")
		}
	})

	t.Run("nil client routes synchronous (defensive)", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, nil) {
			t.Error("nil client: shouldRunConcurrentBootstrap = true, want false")
		}
	})
}

// TestShouldRunConcurrentBootstrap_ProbesOnlyOnTUIPath proves the `has-server`
// probe (the single sanctioned `info` round-trip, §10.1) fires ONLY on the TUI
// path. Non-TUI commands and direct-path opens are classified before the probe,
// so they issue zero tmux round-trips through the decider.
func TestShouldRunConcurrentBootstrap_ProbesOnlyOnTUIPath(t *testing.T) {
	t.Run("non-TUI command: no probe", func(t *testing.T) {
		rec := &recordingCommander{}
		client := tmux.NewClient(rec)
		_ = shouldRunConcurrentBootstrap(&cobra.Command{Use: "list"}, []string{}, client)
		if len(rec.Calls) != 0 {
			t.Errorf("non-TUI path probed %d times %v, want 0", len(rec.Calls), rec.Calls)
		}
	})

	t.Run("direct-path open: no probe", func(t *testing.T) {
		rec := &recordingCommander{}
		client := tmux.NewClient(rec)
		_ = shouldRunConcurrentBootstrap(openProbeCmd(), []string{"/dir"}, client)
		if len(rec.Calls) != 0 {
			t.Errorf("direct-path probed %d times %v, want 0", len(rec.Calls), rec.Calls)
		}
	})

	t.Run("TUI path: exactly one `info` probe", func(t *testing.T) {
		rec := &recordingCommander{}
		client := tmux.NewClient(rec)
		_ = shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, client)
		if len(rec.Calls) != 1 || len(rec.Calls[0]) == 0 || rec.Calls[0][0] != "info" {
			t.Errorf("TUI path probe calls = %v, want exactly [[info]] (the sanctioned has-server check)", rec.Calls)
		}
	})
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

// TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously proves the warm
// direct-TUI path keeps the synchronous route: serverStarted=false is threaded
// to openTUI exactly as today (no deferred bootstrap, no loading page). The only
// seam-issued tmux call is the single sanctioned `info` has-server probe (§10.1)
// that decided the path is warm — the orchestrator (a recording fake) is the
// owner of every other tmux round-trip.
func TestPersistentPreRunE_WarmDirectTUI_RunsSynchronously(t *testing.T) {
	resetBootstrapOnce(t)

	rec := &recordingCommander{}
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
		t.Fatal("openTUI was not reached on the warm direct-TUI path")
	}
	if capturedServerStarted {
		t.Error("warm path threaded serverStarted=true to openTUI; want false (no loading page)")
	}
	if runner.calls != 1 {
		t.Errorf("warm path: orchestrator calls = %d, want 1 (synchronous)", runner.calls)
	}
	// Exactly one seam tmux call: the sanctioned `info` has-server probe (§10.1).
	if len(rec.Calls) != 1 || rec.Calls[0][0] != "info" {
		t.Errorf("warm path seam calls = %v, want exactly [[info]] (the has-server probe)", rec.Calls)
	}
}
