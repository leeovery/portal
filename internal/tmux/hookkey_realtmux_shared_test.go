package tmux_test

// Shared 3-pane stamped-session fixture for the real-tmux hook-key guards.
// Both the HookKeyFormat guard (hookkey_format_realtmux_test.go) and the
// cross-site consistency guard (hookkey_cross_site_realtmux_test.go) need the
// same live topology — a session stamped with @portal-id spanning three panes
// with distinct w.p suffixes (w0.p0, w0.p1, w1.p0) — so the setup is owned here
// once rather than duplicated per file. Like the guards themselves it carries
// NO build tag and relies on the callers' SkipIfNoTmux(t) gate.

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// seedThreePaneStampedSession creates sessionName on ts, stamps it with the
// given @portal-id, then splits window 0 (gaining pane 1) and adds a second
// window (window 1, pane 0) so the session spans three panes with distinct w.p
// suffixes under the one shared id. Returns the pane ids in tmux enumeration
// order for callers that address panes by #{pane_id}; callers that address by
// session:w.p target string may ignore the return.
func seedThreePaneStampedSession(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, sessionName, portalID string) []string {
	t.Helper()
	if err := client.NewSession(sessionName, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", sessionName, err)
	}
	ts.WaitForSession(t, sessionName, 2*time.Second)
	if err := client.SetSessionOption(sessionName, portalIDLiteral, portalID); err != nil {
		t.Fatalf("SetSessionOption(%q, %q, %q): %v", sessionName, portalIDLiteral, portalID, err)
	}
	if err := client.SplitWindow(sessionName+":0", "", ""); err != nil {
		t.Fatalf("SplitWindow(%q): %v", sessionName+":0", err)
	}
	if err := client.NewWindow(sessionName, "", "", ""); err != nil {
		t.Fatalf("NewWindow(%q): %v", sessionName, err)
	}
	return sessionPaneIDs(t, ts, sessionName)
}

// sessionPaneIDs returns every #{pane_id} of the named session, in the order
// tmux enumerates them, via a real list-panes -s read on the isolated socket.
func sessionPaneIDs(t *testing.T, ts *tmuxtest.Socket, session string) []string {
	t.Helper()
	out := ts.Run(t, "list-panes", "-s", "-t", session, "-F", "#{pane_id}")
	var ids []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
