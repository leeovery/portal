package cmd

import (
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// The name "open" is load-bearing: isTUIPath keys off cmd.Name().
func openProbeCmd() *cobra.Command {
	return &cobra.Command{Use: "open"}
}

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

	t.Run("open -- cmd (command after the separator, no target) IS the TUI path", func(t *testing.T) {
		c := openProbeCmd()
		if err := c.ParseFlags([]string{"--", "claude"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if !isTUIPath(c, c.Flags().Args()) {
			t.Error("open -- claude: isTUIPath = false, want true (opens the Projects picker)")
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

	t.Run("a search form IS the TUI path", func(t *testing.T) {
		for _, form := range []string{"/port", "/"} {
			if !isTUIPath(openProbeCmd(), []string{form}) {
				t.Errorf("open %s: isTUIPath = false, want true (a search form heads for the picker)", form)
			}
		}
	})

	t.Run("a search form at any positional index IS the TUI path", func(t *testing.T) {
		if !isTUIPath(openProbeCmd(), []string{"api", "/port"}) {
			t.Error("open api /port: isTUIPath = false, want true")
		}
	})

	t.Run("a path positional is NOT the TUI path", func(t *testing.T) {
		for _, arg := range []string{"/Users/x/y", "/tmp/", "./port", "~/dir", "api"} {
			if isTUIPath(openProbeCmd(), []string{arg}) {
				t.Errorf("open %s: isTUIPath = true, want false", arg)
			}
		}
	})

	t.Run("two positional targets are NOT the TUI path", func(t *testing.T) {
		if isTUIPath(openProbeCmd(), []string{"api", "blog"}) {
			t.Error("open api blog: isTUIPath = true, want false (multi-target burst dispatches directly)")
		}
	})

	t.Run("words after a -- separator are never inspected", func(t *testing.T) {
		c := openProbeCmd()
		if err := c.ParseFlags([]string{"~/Code/api", "--", "ls", "/tmp"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if isTUIPath(c, c.Flags().Args()) {
			t.Error("open ~/Code/api -- ls /tmp: isTUIPath = true, want false (/tmp is the command's own argument)")
		}
	})

	t.Run("a search form beside a domain pin is NOT the TUI path", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("session", "api"); err != nil {
			t.Fatalf("set --session: %v", err)
		}
		if isTUIPath(c, []string{"/port"}) {
			t.Error("open /port -s api: isTUIPath = true, want false (a domain pin dispatches directly)")
		}
	})

	t.Run("a search form classifies exactly as -f", func(t *testing.T) {
		filtered := openProbeCmdWithFlags()
		if err := filtered.Flags().Set("filter", "port"); err != nil {
			t.Fatalf("set --filter: %v", err)
		}
		want := isTUIPath(filtered, []string{})
		if got := isTUIPath(openProbeCmdWithFlags(), []string{"/port"}); got != want {
			t.Errorf("isTUIPath(open /port) = %v, isTUIPath(open -f port) = %v; want the same verdict", got, want)
		}
	})
}

// The decider issues zero tmux round-trips, so the backing commander is
// never called.
func probeClient() *tmux.Client {
	return tmux.NewClient(commandertest.Quiet())
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

	// A domain-pin open dispatches one resolved target directly, so even on a
	// cold/unlatched server it must take the synchronous bootstrap — restore has
	// to run before ResolveSessionPin.
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

	t.Run("it routes concurrent for open -- cmd (command after the separator, not satisfied)", func(t *testing.T) {
		c := openProbeCmd()
		if err := c.ParseFlags([]string{"--", "claude"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, c.Flags().Args(), probeClient(), false) {
			t.Error("open -- claude + not satisfied: shouldRunConcurrentBootstrap = false, want true (command-only open is a TUI path)")
		}
	})
}

func TestShouldRunConcurrentBootstrap_IssuesNoProbe(t *testing.T) {
	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"non-TUI command", &cobra.Command{Use: "list"}, []string{}},
		{"direct-path open", openProbeCmd(), []string{"/dir/sub"}},
		{"TUI path", openProbeCmd(), []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := commandertest.Quiet()
			client := tmux.NewClient(rec)
			_ = shouldRunConcurrentBootstrap(tc.cmd, tc.args, client, false)
			if len(rec.Calls()) != 0 {
				t.Errorf("%s: decider issued %d tmux round-trips %v, want 0", tc.name, len(rec.Calls()), rec.Calls())
			}
		})
	}
}

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

func TestPersistentPreRunE_LatchedTUI_ReadsLatchExactlyOnce(t *testing.T) {
	resetBootstrapOnce(t)
	resetBootstrapWarnings(t)

	rec := satisfiedLatchAliveSaverCommander()
	client := tmux.NewClient(rec)
	runner := &recordingRunner{started: false} // warm: server already running
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var capturedServerStarted bool
	var openTUIReached bool
	withFuncSeam(t, &openTUIFunc, func(_ *cobra.Command, _ pickerLanding, _ []string, serverStarted bool) error {
		capturedServerStarted = serverStarted
		openTUIReached = true
		return nil
	})

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
	if got := len(rec.CallsMatching("show-option")); got != 1 {
		t.Errorf("latch read count (show-option) = %d, want exactly 1 (single-read invariant): %v", got, rec.Calls())
	}
}
