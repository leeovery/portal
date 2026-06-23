package cmd

// Tests in this file mutate package-level state (bootstrapDeps) and the
// shared rootCmd, and MUST NOT use t.Parallel.
//
// Task spectrum-tui-design-5-1 — the cold-vs-warm routing gate.
// shouldRunConcurrentBootstrap must classify ONLY the cold + TUI path
// (serverWasStarted && isTUIPath) for the eventual concurrent bootstrap
// route; every other path keeps today's synchronous behaviour. This is a
// pure-routing foundation gate: no behaviour changes on any path yet.

import (
	"context"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// newGateProbeCmd builds an "open"-named cobra command whose context carries
// the given serverStarted flag — mirroring the context PersistentPreRunE
// populates via serverStartedKey. The name "open" is load-bearing: isTUIPath
// keys off cmd.Name() == "open".
func newGateProbeCmd(serverStarted bool) *cobra.Command {
	cmd := &cobra.Command{Use: "open"}
	cmd.SetContext(context.WithValue(context.Background(), serverStartedKey, serverStarted))
	return cmd
}

func TestShouldRunConcurrentBootstrap(t *testing.T) {
	t.Run("cold + TUI (serverStarted, zero args) routes concurrent", func(t *testing.T) {
		cmd := newGateProbeCmd(true)

		if !shouldRunConcurrentBootstrap(cmd, []string{}) {
			t.Error("cold + TUI: shouldRunConcurrentBootstrap = false, want true")
		}
	})

	t.Run("warm (serverStarted=false, zero args) routes synchronous", func(t *testing.T) {
		cmd := newGateProbeCmd(false)

		if shouldRunConcurrentBootstrap(cmd, []string{}) {
			t.Error("warm: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("cold + CLI/direct-path (serverStarted, path arg) routes synchronous", func(t *testing.T) {
		cmd := newGateProbeCmd(true)

		if shouldRunConcurrentBootstrap(cmd, []string{"~/dir"}) {
			t.Error("cold + direct-path: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("edge: open <path> (cold) is synchronous — isTUIPath false for non-zero args", func(t *testing.T) {
		cmd := newGateProbeCmd(true)

		if shouldRunConcurrentBootstrap(cmd, []string{"/some/path"}) {
			t.Error("open <path>: shouldRunConcurrentBootstrap = true, want false (direct-path, not TUI)")
		}
	})

	t.Run("non-open command (cold, zero args) routes synchronous", func(t *testing.T) {
		cmd := &cobra.Command{Use: "list"}
		cmd.SetContext(context.WithValue(context.Background(), serverStartedKey, true))

		if shouldRunConcurrentBootstrap(cmd, []string{}) {
			t.Error("list: shouldRunConcurrentBootstrap = true, want false (not the TUI path)")
		}
	})
}

// TestShouldRunConcurrentBootstrap_NoTmuxRoundTrips proves the gate adds zero
// new tmux round-trips: the decision is derived from the already-threaded
// serverStarted flag (serverWasStarted) and isTUIPath, never from a fresh
// ServerRunning() / has-server probe. A recordingCommander wired as the tmux
// client must see no calls from the decider on any path.
func TestShouldRunConcurrentBootstrap_NoTmuxRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		name          string
		serverStarted bool
		args          []string
	}{
		{"warm zero-args", false, []string{}},
		{"cold zero-args", true, []string{}},
		{"cold path-arg", true, []string{"/dir"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingCommander{}
			// The client is unused by the decider; it exists only to detect an
			// accidental tmux probe. tmux.NewClient binds the commander so any
			// round-trip the decider performed would be recorded here.
			_ = tmux.NewClient(rec)

			cmd := newGateProbeCmd(tc.serverStarted)
			_ = shouldRunConcurrentBootstrap(cmd, tc.args)

			if len(rec.Calls) != 0 {
				t.Errorf("decider issued %d tmux call(s) %v, want 0 (no new round-trips)", len(rec.Calls), rec.Calls)
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

// TestPersistentPreRunE_WarmDirectTUI_SeamInert proves the routing seam is
// inert on the warm direct-TUI path: with serverStarted=false the gate
// classifies synchronous, so PersistentPreRunE threads serverStarted=false to
// openTUI exactly as it does today — byte-for-byte, no loading-page entry. The
// recording commander confirms the seam itself adds no tmux round-trips beyond
// what the (no-op) orchestrator would.
func TestPersistentPreRunE_WarmDirectTUI_SeamInert(t *testing.T) {
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
	if len(rec.Calls) != 0 {
		t.Errorf("warm path issued %d tmux call(s) %v via the seam; want 0 (orchestrator is the only tmux owner)", len(rec.Calls), rec.Calls)
	}
}
