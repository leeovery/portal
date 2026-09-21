package state_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// The user-created session at the heart of this: a period-bearing name is legal
// and tmux resolves it, but a colon-free target splits on the period into
// window and pane, so the session lookup is a fallback any live prefix
// extension of "my" displaces. The capture reads each session's environment,
// classifies that failure as ErrNoSuchSession, and drops a live session — and
// its scrollback with it — as natural churn.
const (
	periodSession = "my.app"
	periodSibling = "my-cool-app-abc123"
	periodMarker  = "PORTAL_PERIOD_SCROLLBACK_MARKER"
)

func TestCaptureRealTmuxPeriodBearingSessionName(t *testing.T) {
	t.Run("it captures a period-bearing session into sessions.json instead of reporting it vanished", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)

		ts := tmuxtest.New(t, "ptl-periodcap-")
		client := ts.Client()
		if _, err := client.EnsureServer(); err != nil {
			t.Fatalf("EnsureServer: %v", err)
		}
		paneDir := t.TempDir()
		// EnsureServer leaves the _portal-bootstrap anchor behind, as it does
		// on every server Portal starts; the sibling is what tmux reaches
		// instead of the period-bearing name.
		for _, name := range []string{periodSession, periodSibling} {
			ts.Run(t, "new-session", "-d", "-s", name, "-c", paneDir)
		}
		waitForListedSessions(t, ts, tmux.PortalBootstrapName, periodSession, periodSibling)
		seedPeriodScrollback(t, ts)

		logger, sink := openTestLogger(t, t.TempDir())
		idx, _, err := state.CaptureStructure(client, nil, nil, logger)
		if err != nil {
			t.Fatalf("CaptureStructure: %v", err)
		}
		if !capturedSession(idx, periodSibling) {
			t.Fatalf("precondition: %q must be captured; Sessions = %+v", periodSibling, idx.Sessions)
		}
		if !capturedSession(idx, periodSession) {
			t.Fatalf("%q is absent from the capture; Sessions = %+v", periodSession, idx.Sessions)
		}
		if log := sink.Body(); strings.Contains(log, "capture skipping vanished session") {
			t.Errorf("a live %q was counted as natural churn; log:\n%s", periodSession, log)
		}

		// The scrollback half: the pane's bytes have to reach a file the
		// committed index still references, or the session survives empty.
		stateDir := t.TempDir()
		paneKey := state.SanitizePaneKey(periodSession, 0, 0)
		data, hash, err := state.CaptureAndHashPane(client, tmux.PaneTargetExact(periodSession, 0, 0))
		if err != nil {
			t.Fatalf("CaptureAndHashPane(%q): %v", periodSession, err)
		}
		if !strings.Contains(string(data), periodMarker) {
			t.Fatalf("the captured scrollback of %q does not carry %q:\n%s", periodSession, periodMarker, data)
		}
		written, err := state.WriteScrollbackIfChanged(stateDir, paneKey, data, hash, state.HashMap{})
		if err != nil || !written {
			t.Fatalf("WriteScrollbackIfChanged(%q) = %t, %v; want a write", paneKey, written, err)
		}

		if err := state.Commit(stateDir, idx, written, logger); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		// ReadIndex's bool reports whether the caller should skip restoration,
		// so a committed index reads back as (idx, false, nil).
		committed, skip, err := state.ReadIndex(stateDir)
		if err != nil || skip {
			t.Fatalf("ReadIndex = skip %t, %v; want the committed index", skip, err)
		}
		if !capturedSession(committed, periodSession) {
			t.Errorf("sessions.json does not hold %q; Sessions = %+v", periodSession, committed.Sessions)
		}
		// Commit garbage-collects every scrollback file its index does not
		// reference, so a dropped session takes its pane's bytes with it.
		if _, err := os.Stat(state.ScrollbackFile(stateDir, paneKey)); err != nil {
			t.Errorf("%q's scrollback did not survive the commit: %v", periodSession, err)
		}
	})
}

// seedPeriodScrollback puts a recognisable line in the period-bearing session's
// pane, so a capture that returned an empty buffer cannot read as a pass.
func seedPeriodScrollback(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	target := tmux.PaneTargetExact(periodSession, 0, 0)
	ts.Run(t, "send-keys", "-t", string(target), "echo "+periodMarker, "Enter")
	seeded := func() bool {
		out, err := ts.TryRun("capture-pane", "-p", "-t", string(target))
		return err == nil && strings.Contains(out, periodMarker)
	}
	if !harnesstest.PollUntil(t, 2*time.Second, 20*time.Millisecond, seeded) {
		t.Fatalf("%q did not appear in %q's scrollback within 2s", periodMarker, periodSession)
	}
}
