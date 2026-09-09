package tmux_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// liveHookKeyPane starts an isolated server with one session and returns the
// socket, its client and the session's sole pane id.
func liveHookKeyPane(t *testing.T, sessionName string) (*tmuxtest.Socket, *tmux.Client, string) {
	t.Helper()
	ts, client, _ := seedRealTmuxServer(t, hookKeyFixture(sessionName, nil))

	paneIDs := sessionPaneIDs(t, ts, sessionName)
	if len(paneIDs) != 1 {
		t.Fatalf("session %q reported %d panes, want 1", sessionName, len(paneIDs))
	}
	return ts, client, paneIDs[0]
}

func TestResolveHookKey_AgainstARealServer(t *testing.T) {
	const bogusPane = "%999"

	t.Run("it fails for a pane id no pane answers to", func(t *testing.T) {
		_, client, _ := liveHookKeyPane(t, "rhk-gone")

		got, err := client.ResolveHookKey(bogusPane)
		if err == nil {
			t.Fatalf("ResolveHookKey(%q) on a live server = (%q, nil), want a non-nil error", bogusPane, got)
		}
		if got != "" {
			t.Errorf("hook key for a gone pane = %q, want \"\"", got)
		}
		if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
			t.Fatalf("errors.AsType[*tmux.CommandError](err) = false; want a recoverable *tmux.CommandError; err = %v", err)
		}
		if !strings.Contains(err.Error(), "no such pane") {
			t.Errorf("error = %q, want tmux's own words preserved", err.Error())
		}
	})

	t.Run("it resolves a live pane carrying no pane options at all", func(t *testing.T) {
		ts, client, pane := liveHookKeyPane(t, "rhk-bare")

		if out := strings.TrimSpace(ts.Run(t, "show-options", "-p", "-t", pane)); out != "" {
			t.Fatalf("fixture pane %q already carries pane options: %q", pane, out)
		}

		got, err := client.ResolveHookKey(tmux.PaneIDTarget(pane))
		if err != nil {
			t.Fatalf("ResolveHookKey on a live pane with no pane options: %v", err)
		}
		if got != "" {
			t.Errorf("hook key for an un-stamped pane = %q, want an empty token", got)
		}
	})

	t.Run("it resolves a stamped pane to its token", func(t *testing.T) {
		ts, client, pane := liveHookKeyPane(t, "rhk-stamped")

		ts.StampPaneToken(t, tmux.PaneIDTarget(pane), "tok123")

		got, err := client.ResolveHookKey(tmux.PaneIDTarget(pane))
		if err != nil {
			t.Fatalf("ResolveHookKey(%q): %v", pane, err)
		}
		if got != "tok123" {
			t.Errorf("stamped pane hook key = %q, want %q", got, "tok123")
		}
	})

	t.Run("it probes without naming the option", func(t *testing.T) {
		ts, client, pane := liveHookKeyPane(t, "rhk-named")

		out, err := ts.TryRun("show-options", "-p", "-t", pane, state.PortalPaneIDOption)
		if err == nil {
			t.Fatalf("show-options -p -t %s %s exited 0 (%q); naming the option is expected to fail on a live un-stamped pane",
				pane, state.PortalPaneIDOption, out)
		}
		if !strings.Contains(out, "invalid option") {
			t.Errorf("show-options naming the option said %q, want %q — the failure indistinguishable from a gone pane", out, "invalid option")
		}

		if _, err := client.ResolveHookKey(tmux.PaneIDTarget(pane)); err != nil {
			t.Fatalf("ResolveHookKey on the same live pane failed (%v); the probe must not name the option", err)
		}
	})

	t.Run("it pins the raw tmux facts the probe rests on", func(t *testing.T) {
		ts, _, _ := liveHookKeyPane(t, "rhk-facts")

		if out, err := ts.TryRun("set-option", "-p", "-t", bogusPane, state.PortalPaneIDOption, "X"); err == nil {
			t.Errorf("set-option -p against %s exited 0 (%q), want non-zero", bogusPane, out)
		}
		if out, err := ts.TryRun("display-message", "-p", "-t", bogusPane, tmux.HookKeyFormat); err != nil {
			t.Errorf("display-message -p against %s exited non-zero (%v, %q); the read cannot detect a gone pane, which is why the probe exists",
				bogusPane, err, out)
		}
	})
}
