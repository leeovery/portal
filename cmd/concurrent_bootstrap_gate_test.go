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

// openProbeCmdWithFlags builds an "open"-named cobra command carrying the same
// -e/-f/-s/-p/-z/-a flag surface production registers (cmd/open.go's init), so
// isTUIPath / anyOpenDomainPin can be probed with specific flags marked
// Changed via Flags().Set. The domain pins (-s/-p/-z/-a) flip isTUIPath off;
// -f/--filter and -e/--exec do NOT (they still launch the picker).
func openProbeCmdWithFlags() *cobra.Command {
	c := &cobra.Command{Use: "open"}
	c.Flags().StringP("exec", "e", "", "")
	c.Flags().StringP("filter", "f", "", "")
	c.Flags().StringP("session", "s", "", "")
	c.Flags().StringP("path", "p", "", "")
	c.Flags().StringP("alias", "a", "", "")
	c.Flags().StringP("zoxide", "z", "", "")
	return c
}

// TestIsTUIPath locks the retire-attach routing fix (Phase 5, task 5-1): a bare
// picker-launching `open` is the TUI path, but a domain-pin open
// (-s/-p/-z/-a) is NOT — it dispatches a single resolved target directly and
// must take the synchronous direct-path bootstrap (spec § attach — Retired:
// "--session/--path never fall back to the TUI picker"). -f/--filter and
// -e/--exec still launch the picker, so they remain TUI paths.
func TestIsTUIPath(t *testing.T) {
	t.Run("bare open (no args, no pins) is the TUI path", func(t *testing.T) {
		if !isTUIPath(openProbeCmd(), []string{}) {
			t.Error("bare open: isTUIPath = false, want true")
		}
	})

	t.Run("open with a positional target is NOT the TUI path", func(t *testing.T) {
		if isTUIPath(openProbeCmd(), []string{"~/dir"}) {
			t.Error("positional open: isTUIPath = true, want false")
		}
	})

	domainPins := []struct{ flag, val string }{
		{"session", "api"},
		{"path", "/tmp/x"},
		{"zoxide", "proj"},
		{"alias", "work"},
	}
	for _, dp := range domainPins {
		t.Run("open --"+dp.flag+" (domain pin) is NOT the TUI path", func(t *testing.T) {
			c := openProbeCmdWithFlags()
			if err := c.Flags().Set(dp.flag, dp.val); err != nil {
				t.Fatalf("set --%s: %v", dp.flag, err)
			}
			if isTUIPath(c, []string{}) {
				t.Errorf("open --%s: isTUIPath = true, want false (domain pin dispatches directly)", dp.flag)
			}
		})
	}

	t.Run("open -f text (filter) IS the TUI path", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("filter", "text"); err != nil {
			t.Fatalf("set --filter: %v", err)
		}
		if !isTUIPath(c, []string{}) {
			t.Error("open -f text: isTUIPath = false, want true (filter opens the picker pre-filtered)")
		}
	})

	t.Run("open -e cmd (command, no target) IS the TUI path", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("exec", "vim"); err != nil {
			t.Fatalf("set --exec: %v", err)
		}
		if !isTUIPath(c, []string{}) {
			t.Error("open -e cmd: isTUIPath = false, want true (opens the Projects picker)")
		}
	})

	t.Run("repeated session pins (no positional) are NOT the TUI path", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		_ = c.Flags().Set("session", "a")
		_ = c.Flags().Set("session", "b")
		if isTUIPath(c, []string{}) {
			t.Error("open -s a -s b: isTUIPath = true, want false (multi-target burst dispatches directly)")
		}
	})

	t.Run("a non-open command is never the TUI path", func(t *testing.T) {
		if isTUIPath(&cobra.Command{Use: "list"}, []string{}) {
			t.Error("list: isTUIPath = true, want false")
		}
	})
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

	// Domain-pin opens dispatch a single resolved target directly (never the
	// picker), so even on a cold/unlatched server they take the SYNCHRONOUS
	// direct-path bootstrap — restore must run before ResolveSessionPin. This is
	// the retire-attach routing fix: a bare positional attach (`attach NAME`) was
	// always synchronous, so its replacement `open --session NAME` must be too.
	for _, flag := range []string{"session", "path", "zoxide", "alias"} {
		t.Run("it routes non-concurrent for open --"+flag+" (domain pin, not satisfied)", func(t *testing.T) {
			c := openProbeCmdWithFlags()
			if err := c.Flags().Set(flag, "val"); err != nil {
				t.Fatalf("set --%s: %v", flag, err)
			}
			if shouldRunConcurrentBootstrap(c, []string{}, probeClient(), false) {
				t.Errorf("open --%s + not satisfied: shouldRunConcurrentBootstrap = true, want false (synchronous direct dispatch)", flag)
			}
		})
	}

	t.Run("it routes concurrent for open -f text (filter, not satisfied)", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("filter", "text"); err != nil {
			t.Fatalf("set --filter: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, []string{}, probeClient(), false) {
			t.Error("open -f text + not satisfied: shouldRunConcurrentBootstrap = false, want true (filter is a TUI path)")
		}
	})

	t.Run("it routes concurrent for open -e cmd (command, no target, not satisfied)", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("exec", "vim"); err != nil {
			t.Fatalf("set --exec: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, []string{}, probeClient(), false) {
			t.Error("open -e cmd + not satisfied: shouldRunConcurrentBootstrap = false, want true (command-only open is a TUI path)")
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

// TestPersistentPreRunE_LatchedTUI_ReadsLatchExactlyOnce proves the latch
// verdict is computed EXACTLY ONCE per PersistentPreRunE: a satisfied latch is
// diverted to the abridged path after a single @portal-bootstrapped read
// (show-option), and the verdict is never re-read — the retired ServerRunning()
// probe is gone and shouldRunConcurrentBootstrap is never reached on the
// satisfied path. openTUI is reached with serverStarted=false (instant picker)
// and the full orchestrator never runs; the abridged saver-liveness probe
// (list-panes) is the only other seam call.
func TestPersistentPreRunE_LatchedTUI_ReadsLatchExactlyOnce(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	rec := satisfiedLatchAliveSaverCommander()
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
		t.Fatal("openTUI was not reached on the abridged TUI path")
	}
	if capturedServerStarted {
		t.Error("abridged path threaded serverStarted=true to openTUI; want false (no loading page)")
	}
	if runner.calls != 0 {
		t.Errorf("abridged path: orchestrator calls = %d, want 0 (never runs the full bootstrap)", runner.calls)
	}
	if got := countOp(rec.Calls, "show-option"); got != 1 {
		t.Errorf("latch read count (show-option) = %d, want exactly 1 (single-read invariant): %v", got, rec.Calls)
	}
}
