package cmd

import (
	"testing"

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

func TestShouldRunConcurrentBootstrap(t *testing.T) {
	runner := &recordingRunner{}

	t.Run("it routes concurrent for a bare open with a runner", func(t *testing.T) {
		if !shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, runner) {
			t.Error("TUI + runner: shouldRunConcurrentBootstrap = false, want true")
		}
	})

	t.Run("it routes non-concurrent with no runner, so the picker opens straight onto its list", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{}, nil) {
			t.Error("TUI + no runner: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	t.Run("it routes non-concurrent for a non-TUI command", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(&cobra.Command{Use: "list"}, []string{}, runner) {
			t.Error("non-TUI command: shouldRunConcurrentBootstrap = true, want false (not the TUI path)")
		}
	})

	t.Run("it routes non-concurrent for a direct-path open", func(t *testing.T) {
		if shouldRunConcurrentBootstrap(openProbeCmd(), []string{"~/dir"}, runner) {
			t.Error("direct-path open: shouldRunConcurrentBootstrap = true, want false")
		}
	})

	// A domain-pin open dispatches one resolved target directly, so it must
	// take the synchronous bootstrap: restore has to run before
	// ResolveSessionPin.
	for _, flag := range []string{"session", "path", "zoxide", "alias"} {
		t.Run("it routes non-concurrent for open --"+flag+" (domain pin)", func(t *testing.T) {
			c := openProbeCmdWithFlags()
			if err := c.Flags().Set(flag, "val"); err != nil {
				t.Fatalf("set --%s: %v", flag, err)
			}
			if shouldRunConcurrentBootstrap(c, []string{}, runner) {
				t.Errorf("open --%s: shouldRunConcurrentBootstrap = true, want false (synchronous direct dispatch)", flag)
			}
		})
	}

	t.Run("it routes concurrent for open -f text (filter)", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("filter", "text"); err != nil {
			t.Fatalf("set --filter: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, []string{}, runner) {
			t.Error("open -f text: shouldRunConcurrentBootstrap = false, want true (filter is a TUI path)")
		}
	})

	t.Run("it routes concurrent for open -e cmd (command, no target)", func(t *testing.T) {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set("exec", "vim"); err != nil {
			t.Fatalf("set --exec: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, []string{}, runner) {
			t.Error("open -e cmd: shouldRunConcurrentBootstrap = false, want true (command-only open is a TUI path)")
		}
	})

	t.Run("it routes concurrent for open -- cmd (command after the separator)", func(t *testing.T) {
		c := openProbeCmd()
		if err := c.ParseFlags([]string{"--", "claude"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		if !shouldRunConcurrentBootstrap(c, c.Flags().Args(), runner) {
			t.Error("open -- claude: shouldRunConcurrentBootstrap = false, want true (command-only open is a TUI path)")
		}
	})
}

func TestWithBootstrapping_GatesLoadingPage(t *testing.T) {
	t.Run("warm (no bootstrap) starts on PageSessions, never PageLoading", func(t *testing.T) {
		m := tui.New(fakePickerLister(), tui.WithBootstrapping(false))

		if m.ActivePage() == tui.PageLoading {
			t.Error("warm path landed on PageLoading; want PageSessions")
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (%d)", m.ActivePage(), tui.PageSessions)
		}
	})

	t.Run("cold (bootstrapping) starts on PageLoading", func(t *testing.T) {
		m := tui.New(fakePickerLister(), tui.WithBootstrapping(true))

		if m.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading (%d)", m.ActivePage(), tui.PageLoading)
		}
	})
}
