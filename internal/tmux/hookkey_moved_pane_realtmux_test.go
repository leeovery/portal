package tmux_test

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestHookKeyDurability_PaneMoves(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	const (
		sessionName  = "mvpane"
		otherSession = "mvpane-elsewhere"
		token        = "tokMove"
	)

	ts, client, probe := newStampedPaneFixture(t, sessionName, token)
	if err := client.NewSession(otherSession, t.TempDir(), ""); err != nil {
		t.Fatalf("NewSession(%q): %v", otherSession, err)
	}
	ts.WaitForSession(t, otherSession, 2*time.Second)

	// The subtests run in declaration order against one pane: each move starts
	// where the previous one left it, and each asserts immediately after its own
	// move so a failure names the operation that broke the key rather than only
	// the end state.
	t.Run("it keeps the hook key when the pane is broken out to its own window", func(t *testing.T) {
		// The destination is named explicitly: without -t, break-pane lands the
		// pane in whichever session tmux considers current, which would silently
		// make this a cross-session move as well.
		ts.Run(t, "break-pane", "-d", "-s", probe.paneID, "-t", sessionName+":")
		probe.assertSurvivedMove(t)
	})

	t.Run("it keeps the hook key when an earlier window is closed under renumber-windows on", func(t *testing.T) {
		_, beforeWindow := splitLocation(t, probe.location)
		ts.Run(t, "kill-window", "-t", sessionName+":0")
		row := probe.assertSurvivedMove(t)
		if _, afterWindow := splitLocation(t, row.Location); afterWindow == beforeWindow {
			t.Errorf("window index still %q after kill-window: tmux did not renumber, so the case proves nothing", afterWindow)
		}
	})

	t.Run("it keeps the hook key when the pane is moved back", func(t *testing.T) {
		ts.Run(t, "move-pane", "-s", probe.paneID, "-t", sessionName+":0")
		probe.assertSurvivedMove(t)
	})

	t.Run("it keeps the hook key when the pane is moved to another session", func(t *testing.T) {
		beforeSession, _ := splitLocation(t, probe.location)
		ts.Run(t, "move-pane", "-s", probe.paneID, "-t", otherSession+":0")
		row := probe.assertSurvivedMove(t)
		afterSession, _ := splitLocation(t, row.Location)
		if afterSession == beforeSession {
			t.Errorf("session half still %q after move-pane across sessions: the pane did not change session", afterSession)
		}
		if afterSession != otherSession {
			t.Errorf("session half after cross-session move = %q, want %q", afterSession, otherSession)
		}
	})

	t.Run("it enumerates exactly one row carrying the token after every move", func(t *testing.T) {
		row := probe.tokenRow(t)
		if row.Location != probe.location {
			t.Errorf("sole token row at %q, want the pane's last known location %q", row.Location, probe.location)
		}
	})
}

func TestHookKeyDurability_RespawnPane(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	const (
		sessionName = "mvrespawn"
		token       = "tokRespawn"
	)

	_, client, probe := newStampedPaneFixture(t, sessionName, token)
	before := probe.location

	// Restore arms every pane this way, so the stamp has to outlive it.
	if err := client.RespawnPane(tmux.PaneIDTarget(probe.paneID), "sleep 30"); err != nil {
		t.Fatalf("RespawnPane(%q): %v", probe.paneID, err)
	}

	probe.assertResolvesToToken(t)
	row := probe.tokenRow(t)
	if row.Location != before {
		t.Errorf("location after respawn-pane -k = %q, want it unchanged at %q", row.Location, before)
	}
}

func TestHookKeyDurability_NoInheritance(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	const (
		sessionName = "mvsplit"
		token       = "tokSplit"
	)

	ts, client, probe := newStampedPaneFixture(t, sessionName, token)

	t.Run("it does not inherit the token into a split", func(t *testing.T) {
		before := sessionPaneIDs(t, ts, sessionName)
		if err := client.SplitWindow(tmux.PaneIDTarget(probe.paneID), "", ""); err != nil {
			t.Fatalf("SplitWindow(%q): %v", probe.paneID, err)
		}
		probe.assertCreatedPaneCarriesNoToken(t, newPaneID(t, before, sessionPaneIDs(t, ts, sessionName)))
	})

	t.Run("it does not inherit the token into a new window", func(t *testing.T) {
		before := sessionPaneIDs(t, ts, sessionName)
		if err := client.NewWindow(sessionName, "", "", ""); err != nil {
			t.Fatalf("NewWindow(%q): %v", sessionName, err)
		}
		probe.assertCreatedPaneCarriesNoToken(t, newPaneID(t, before, sessionPaneIDs(t, ts, sessionName)))
	})
}

// paneTokenProbe holds one stamped pane and the location it was last seen at.
// The pane's %N id is the test's handle on it — stable for the server's
// lifetime, so the same pane can be re-read after each move — and is never the
// identity under test.
type paneTokenProbe struct {
	client   *tmux.Client
	paneID   string
	token    string
	location string
}

// newStampedPaneFixture starts an isolated server whose sessions renumber their
// windows, seeds a three-window session whose second window holds two panes,
// stamps the token on the second of those and returns a probe on it.
func newStampedPaneFixture(t *testing.T, sessionName, token string) (*tmuxtest.Socket, *tmux.Client, *paneTokenProbe) {
	t.Helper()

	ts, client, _ := seedRealTmuxServer(t, hookKeyFixture(sessionName, func(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, _ string) {
		// renumber-windows is off in vanilla tmux; without it kill-window leaves
		// the surviving window indices alone, so the renumbering case would
		// exercise no renumbering at all.
		ts.Run(t, "set-option", "-g", "renumber-windows", "on")

		for range 2 {
			if err := client.NewWindow(tmux.Target(sessionName), "", "", ""); err != nil {
				t.Fatalf("NewWindow(%q): %v", sessionName, err)
			}
		}
		if err := client.SplitWindow(tmux.Target(sessionName+":1"), "", ""); err != nil {
			t.Fatalf("SplitWindow(%q): %v", sessionName+":1", err)
		}
	}))

	paneID := paneIDAt(t, ts, sessionName+":1.1")
	ts.StampPaneToken(t, tmux.PaneIDTarget(paneID), token)

	probe := &paneTokenProbe{client: client, paneID: paneID, token: token}
	probe.location = probe.tokenRow(t).Location
	return ts, client, probe
}

// paneTokenRow is one live pane: its token, empty when unstamped, and its
// "<session>:<window>.<pane>" location.
type paneTokenRow struct {
	Token, Location string
}

// paneTokenRowFormat puts the token first and splits at its first "|", since a
// location carries ":" and ".".
const paneTokenRowFormat = "#{" + tmux.PortalPaneIDOption + "}|#{session_name}:#{window_index}.#{pane_index}"

// tokenRow returns the one live pane carrying the probe's token. Zero rows
// means the key was lost; more than one means a rearrangement duplicated the
// token, which would fire one hook on two panes.
func (p *paneTokenProbe) tokenRow(t *testing.T) paneTokenRow {
	t.Helper()
	raw, err := p.client.ListAllPanesWithFormat(paneTokenRowFormat)
	if err != nil {
		t.Fatalf("list-panes -a: %v", err)
	}
	var matches []paneTokenRow
	for line := range strings.SplitSeq(raw, "\n") {
		if token, location, _ := strings.Cut(line, "|"); token == p.token {
			matches = append(matches, paneTokenRow{Token: token, Location: location})
		}
	}
	if len(matches) != 1 {
		t.Fatalf("panes carrying token %q = %d, want exactly 1; list-panes was %q", p.token, len(matches), raw)
	}
	return matches[0]
}

func (p *paneTokenProbe) assertResolvesToToken(t *testing.T) {
	t.Helper()
	resolved, err := p.client.ResolveHookKey(tmux.PaneIDTarget(p.paneID))
	if err != nil {
		t.Fatalf("ResolveHookKey(%q): %v", p.paneID, err)
	}
	if resolved != p.token {
		t.Errorf("ResolveHookKey(%q) = %q, want %q", p.paneID, resolved, p.token)
	}
}

// assertSurvivedMove asserts the pane still answers to its token through both
// Portal reads, and that it genuinely moved: a rearrangement that left the
// coordinates alone says nothing about their irrelevance to the key.
func (p *paneTokenProbe) assertSurvivedMove(t *testing.T) paneTokenRow {
	t.Helper()
	p.assertResolvesToToken(t)
	row := p.tokenRow(t)
	if row.Location == p.location {
		t.Fatalf("pane location unchanged at %q: the operation did not move the pane", row.Location)
	}
	p.location = row.Location
	return row
}

// assertCreatedPaneCarriesNoToken asserts a pane derived from the stamped one is
// live, carries no token of its own, and left the stamped pane the sole holder.
func (p *paneTokenProbe) assertCreatedPaneCarriesNoToken(t *testing.T, createdPaneID string) {
	t.Helper()
	resolved, err := p.client.ResolveHookKey(tmux.PaneIDTarget(createdPaneID))
	if err != nil {
		t.Fatalf("ResolveHookKey(%q) on the created pane: %v", createdPaneID, err)
	}
	if resolved != "" {
		t.Errorf("created pane %q resolved to %q, want an empty key (a stamp must not be inherited)", createdPaneID, resolved)
	}
	if row := p.tokenRow(t); row.Location != p.location {
		t.Errorf("sole token row at %q, want the stamped pane's location %q", row.Location, p.location)
	}
}

// splitLocation breaks a "<session>:<window>.<pane>" location into its session
// and window-index halves.
func splitLocation(t *testing.T, location string) (session, window string) {
	t.Helper()
	session, rest, ok := strings.Cut(location, ":")
	if !ok {
		t.Fatalf("location %q is not <session>:<window>.<pane>", location)
	}
	window, _, ok = strings.Cut(rest, ".")
	if !ok {
		t.Fatalf("location %q is not <session>:<window>.<pane>", location)
	}
	return session, window
}

func paneIDAt(t *testing.T, ts *tmuxtest.Socket, target string) string {
	t.Helper()
	return strings.TrimSpace(ts.Run(t, "display-message", "-p", "-t", target, "#{pane_id}"))
}

// newPaneID returns the single pane id present in after but not in before.
func newPaneID(t *testing.T, before, after []string) string {
	t.Helper()
	existing := make(map[string]struct{}, len(before))
	for _, id := range before {
		existing[id] = struct{}{}
	}
	var created []string
	for _, id := range after {
		if _, seen := existing[id]; !seen {
			created = append(created, id)
		}
	}
	if len(created) != 1 {
		t.Fatalf("panes created = %v, want exactly 1 (before %v, after %v)", created, before, after)
	}
	return created[0]
}
