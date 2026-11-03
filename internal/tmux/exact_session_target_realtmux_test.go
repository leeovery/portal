package tmux_test

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// The gone session and the live prefix sibling every case in this file works
// against. "goneSession" is never created: a session renamed away leaves tmux
// in exactly this state.
const (
	goneSession   = "sib"
	prefixSibling = "sib-2"
)

// The shape prefixSiblingFixture builds: two windows of two panes each, with
// the second window current. A single-window sibling would let a form that
// resolves only the current window pass as if it had resolved the whole
// session.
var siblingAllCoords = []tmux.PaneCoord{
	{Window: 0, Pane: 0}, {Window: 0, Pane: 1},
	{Window: 1, Pane: 0}, {Window: 1, Pane: 1},
}

// prefixSiblingFixture holds prefixSibling and nothing else, shaped to
// siblingAllCoords.
var prefixSiblingFixture = realTmuxFixture{
	socketPrefix: "ptl-exacttgt-",
	sessions:     []string{prefixSibling},
	topology: func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client, _ string) {
		// new-window runs last-but-one, so window 1 is the session's current
		// window.
		ts.Run(t, "split-window", "-t", "="+prefixSibling+":0.0")
		ts.Run(t, "new-window", "-t", "="+prefixSibling+":")
		ts.Run(t, "split-window", "-t", "="+prefixSibling+":1.0")
	},
}

func siblingEnvironment(t *testing.T, ts *tmuxtest.Socket) string {
	t.Helper()
	return ts.Run(t, "show-environment", "-t", prefixSibling)
}

func siblingOptions(t *testing.T, ts *tmuxtest.Socket) string {
	t.Helper()
	return ts.Run(t, "show-options", "-t", prefixSibling)
}

// siblingWindowLayout reads the layout string of the sibling's first window,
// the window every SelectLayout case here writes to.
func siblingWindowLayout(t *testing.T, ts *tmuxtest.Socket) string {
	t.Helper()
	out := ts.Run(t, "display-message", "-p", "-t", "="+prefixSibling+":0", "#{window_layout}")
	return strings.TrimSpace(out)
}

// siblingFirstPaneField reads one tmux format field of the sibling's first
// pane — the pane both saver reads resolve to.
func siblingFirstPaneField(t *testing.T, ts *tmuxtest.Socket, format string) string {
	t.Helper()
	out := ts.Run(t, "list-panes", "-t", "="+prefixSibling+":", "-F", format)
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	if first == "" {
		t.Fatalf("list-panes -F %s on %q returned no pane", format, prefixSibling)
	}
	return first
}

// goneSessionRouteCases is the wrong-session half, one case per route: every
// per-session read and write must miss rather than silently land on the live
// sibling. Keyed by route name so the cases can be matched against
// perSessionRoutes rather than trusted to stay in step with it.
var goneSessionRouteCases = map[string]func(*testing.T){
	"ListPanesInSession": func(t *testing.T) {
		_, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		coords, err := client.ListPanesInSession(goneSession)
		if err == nil {
			t.Fatalf("ListPanesInSession(%q) succeeded with %v, want a tmux failure", goneSession, coords)
		}
	},

	"ShowEnvironment": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		ts.Run(t, "set-environment", "-t", prefixSibling, "PORTAL_SIB", "sibling-only")

		out, err := client.ShowEnvironment(goneSession)
		// The sentinel against real tmux, not a hand-supplied stderr: capture
		// discriminates on it to tell natural session churn from anomaly, so
		// the wire form has to keep producing it.
		if !errors.Is(err, tmux.ErrNoSuchSession) {
			t.Fatalf("ShowEnvironment(%q) = %q, %v; want an error matching ErrNoSuchSession", goneSession, out, err)
		}
		if strings.Contains(out, "sibling-only") {
			t.Errorf("ShowEnvironment(%q) read the live %q session's environment", goneSession, prefixSibling)
		}
	},

	"SetSessionEnvironment": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		if err := client.SetSessionEnvironment(goneSession, "PORTAL_LEAK", "leaked"); err == nil {
			t.Fatalf("SetSessionEnvironment(%q) succeeded, want a tmux failure", goneSession)
		}
		if got := siblingEnvironment(t, ts); strings.Contains(got, "PORTAL_LEAK") {
			t.Errorf("the write landed on the live %q session:\n%s", prefixSibling, got)
		}
	},

	"SetSessionOption": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		if err := client.SetSessionOption(goneSession, "@portal-leak", "leaked"); err == nil {
			t.Fatalf("SetSessionOption(%q) succeeded, want a tmux failure", goneSession)
		}
		if got := siblingOptions(t, ts); strings.Contains(got, "@portal-leak") {
			t.Errorf("the write landed on the live %q session:\n%s", prefixSibling, got)
		}
	},

	"SaverPaneID": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		siblingPane := siblingFirstPaneField(t, ts, "#{pane_id}")

		got, err := client.SaverPaneID(goneSession)
		if got == siblingPane {
			t.Fatalf("SaverPaneID(%q) returned %q — the live %q session's pane", goneSession, got, prefixSibling)
		}
		if !errors.Is(err, tmux.ErrNoSuchSession) {
			t.Errorf("SaverPaneID(%q) = %q, %v; want an error matching ErrNoSuchSession", goneSession, got, err)
		}
	},

	"SaverPanePIDOrAbsent": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		siblingPID := siblingFirstPaneField(t, ts, "#{pane_pid}")

		pid, present, err := tmux.SaverPanePIDOrAbsent(client, goneSession)
		if strconv.Itoa(pid) == siblingPID {
			t.Fatalf("SaverPanePIDOrAbsent(%q) returned %d — the live %q session's pane pid", goneSession, pid, prefixSibling)
		}
		if pid != 0 || present || err != nil {
			t.Errorf("SaverPanePIDOrAbsent(%q) = (%d, %t, %v); want a collapsed absence (0, false, nil)", goneSession, pid, present, err)
		}
	},

	"SelectLayout": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		before := siblingWindowLayout(t, ts)

		// Errorf, not Fatalf: a call that wrongly succeeds must still reach the
		// layout comparison, which is where the wrong-session write shows up.
		if err := client.SelectLayout(goneSession, 0, "even-horizontal"); err == nil {
			t.Errorf("SelectLayout(%q) succeeded, want a tmux failure", goneSession)
		}
		if got := siblingWindowLayout(t, ts); got != before {
			t.Errorf("the live %q session's layout changed from %q to %q", prefixSibling, before, got)
		}
	},
}

// liveSessionRouteCases is the half that keeps the exact forms honest: pinning
// the session must not cost the ordinary case, and a form tmux cannot resolve
// at all would read as a permanent miss rather than a wrong-session hit.
var liveSessionRouteCases = map[string]func(*testing.T){
	"ListPanesInSession": func(t *testing.T) {
		_, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		coords, err := client.ListPanesInSession(prefixSibling)
		if err != nil {
			t.Fatalf("ListPanesInSession(%q): %v", prefixSibling, err)
		}
		if !slices.Equal(coords, siblingAllCoords) {
			t.Errorf("ListPanesInSession(%q) = %v, want %v — every pane of every window", prefixSibling, coords, siblingAllCoords)
		}
	},

	"ShowEnvironment": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		ts.Run(t, "set-environment", "-t", prefixSibling, "PORTAL_SIB", "sibling-only")

		out, err := client.ShowEnvironment(prefixSibling)
		if err != nil {
			t.Fatalf("ShowEnvironment(%q): %v", prefixSibling, err)
		}
		if !strings.Contains(out, "PORTAL_SIB=sibling-only") {
			t.Errorf("ShowEnvironment(%q) = %q, want it to carry PORTAL_SIB=sibling-only", prefixSibling, out)
		}
	},

	"SetSessionEnvironment": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		if err := client.SetSessionEnvironment(prefixSibling, "PORTAL_SET", "written"); err != nil {
			t.Fatalf("SetSessionEnvironment(%q): %v", prefixSibling, err)
		}
		if got := siblingEnvironment(t, ts); !strings.Contains(got, "PORTAL_SET=written") {
			t.Errorf("the write did not land on %q:\n%s", prefixSibling, got)
		}
	},

	"SetSessionOption": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)

		if err := client.SetSessionOption(prefixSibling, "@portal-set", "written"); err != nil {
			t.Fatalf("SetSessionOption(%q): %v", prefixSibling, err)
		}
		if got := siblingOptions(t, ts); !strings.Contains(got, "@portal-set written") {
			t.Errorf("the write did not land on %q:\n%s", prefixSibling, got)
		}
	},

	"SaverPaneID": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		want := siblingFirstPaneField(t, ts, "#{pane_id}")

		got, err := client.SaverPaneID(prefixSibling)
		if err != nil {
			t.Fatalf("SaverPaneID(%q): %v", prefixSibling, err)
		}
		if got != want {
			t.Errorf("SaverPaneID(%q) = %q, want %q", prefixSibling, got, want)
		}
	},

	"SaverPanePIDOrAbsent": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		want := siblingFirstPaneField(t, ts, "#{pane_pid}")

		pid, present, err := tmux.SaverPanePIDOrAbsent(client, prefixSibling)
		if err != nil {
			t.Fatalf("SaverPanePIDOrAbsent(%q): %v", prefixSibling, err)
		}
		if !present || strconv.Itoa(pid) != want {
			t.Errorf("SaverPanePIDOrAbsent(%q) = (%d, %t), want (%s, true)", prefixSibling, pid, present, want)
		}
	},

	"SelectLayout": func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, prefixSiblingFixture)
		ts.Run(t, "select-layout", "-t", "="+prefixSibling+":0", "even-vertical")
		before := siblingWindowLayout(t, ts)

		if err := client.SelectLayout(prefixSibling, 0, "even-horizontal"); err != nil {
			t.Fatalf("SelectLayout(%q): %v", prefixSibling, err)
		}
		if got := siblingWindowLayout(t, ts); got == before {
			t.Errorf("the layout of %q:0 is still %q — the write did not land", prefixSibling, got)
		}
	},
}

// runRouteCases runs every case in a route-keyed map, ordered so a failing run
// reads the same way twice.
func runRouteCases(t *testing.T, cases map[string]func(*testing.T)) {
	t.Helper()
	for _, name := range slices.Sorted(maps.Keys(cases)) {
		t.Run(name, cases[name])
	}
}

func TestSessionTargets_GoneSessionDoesNotReachPrefixSibling(t *testing.T) {
	runRouteCases(t, goneSessionRouteCases)
}

func TestSessionTargets_LiveSessionStillResolves(t *testing.T) {
	runRouteCases(t, liveSessionRouteCases)
}

func TestSessionTargets_EveryPerSessionRouteIsCovered(t *testing.T) {
	t.Run("it covers every per-session read in the real-tmux prefix-sibling routes", func(t *testing.T) {
		halves := map[string]map[string]func(*testing.T){
			"gone-session": goneSessionRouteCases,
			"live-session": liveSessionRouteCases,
		}
		for _, half := range slices.Sorted(maps.Keys(halves)) {
			for _, route := range perSessionRoutes {
				if _, ok := halves[half][route.name]; !ok {
					t.Errorf("the %s half has no case for the %q route", half, route.name)
				}
			}
		}
	})
}
