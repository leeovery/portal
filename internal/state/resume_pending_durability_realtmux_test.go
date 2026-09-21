package state_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	pendingSubjectSession = "pending-subject"
	pendingOtherSession   = "pending-other"
	pendingRenamedSession = "pending-renamed"
	pendingPaneToken      = "pendingtok01"
	pendingSettleTimeout  = 2 * time.Second
	pendingSettlePoll     = 20 * time.Millisecond
)

// pendingAddr is the pane's live address as Portal's own capture reports it.
type pendingAddr struct {
	session string
	window  int
	pane    int
}

func (a pendingAddr) key() string {
	return state.SanitizePaneKey(a.session, a.window, a.pane)
}

func (a pendingAddr) String() string {
	return fmt.Sprintf("%s:%d.%d", a.session, a.window, a.pane)
}

type pendingRearrangement struct {
	name    string
	perform func(t *testing.T, ts *tmuxtest.Socket, idx state.Index, at pendingAddr)
	moved   func(before, after pendingAddr) bool
	want    string
}

func TestResumePendingMarkerRealTmuxDurability(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-pending-")
	client := ts.Client()
	paneID := seedPendingFixture(t, ts)
	target := tmux.PaneIDTarget(paneID)

	// The token is stamped before the baseline: the frozen record is carried
	// forward on the token the previous capture recorded, so a baseline taken
	// without it would match nothing the moment the pane moved.
	ts.StampPaneToken(t, target, pendingPaneToken)

	baseline, pending, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("baseline CaptureStructure: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("precondition: an unmarked fixture reported %d pending panes: %v", len(pending), pending)
	}
	at, pane := locatePendingPane(t, baseline, pendingPaneToken)
	baselineFile := pane.ScrollbackFile
	if baselineFile == "" {
		t.Fatalf("precondition: the baseline record of %s names no scrollback file", at)
	}

	ts.MarkResumePending(t, target)
	if got := ts.ReadResumePending(t, target); got != "1" {
		t.Fatalf("precondition: %s reads back %q for %s; want %q",
			at, got, state.ResumePendingOption, "1")
	}

	prev := baseline
	for _, step := range pendingRearrangements(paneID) {
		t.Run(step.name, func(t *testing.T) {
			before := at
			step.perform(t, ts, prev, before)

			idx, pending, after, record := capturePendingUntil(t, client, prev, func(a pendingAddr) bool {
				return step.moved(before, a)
			})
			prev, at = idx, after

			if !step.moved(before, after) {
				t.Errorf("the pane went from %s to %s; want %s", before, after, step.want)
			}
			if _, marked := pending[after.key()]; !marked {
				t.Errorf("%s is not reported pending under %q; pending = %v",
					after, after.key(), sortedPendingKeys(pending))
			}
			if record.ScrollbackFile != baselineFile {
				t.Errorf("%s's record names %q; want the file its bytes were filed under, %q",
					after, record.ScrollbackFile, baselineFile)
			}

			t.Run("it reports exactly one pending pane after every move", func(t *testing.T) {
				if len(pending) != 1 {
					t.Errorf("%d panes report pending: %v", len(pending), sortedPendingKeys(pending))
				}
			})
		})
	}
}

func pendingRearrangements(paneID string) []pendingRearrangement {
	windowChanged := func(before, after pendingAddr) bool {
		return after.session == before.session && after.window != before.window
	}
	sessionChanged := func(before, after pendingAddr) bool { return after.session != before.session }

	return []pendingRearrangement{
		{
			name: "it keeps the marker and the frozen record when the pane is broken out to its own window",
			perform: func(t *testing.T, ts *tmuxtest.Socket, _ state.Index, at pendingAddr) {
				// The destination session is named: with no attached client,
				// break-pane's default lands the window in whichever session
				// tmux last used, which is not the pane's own.
				ts.Run(t, "break-pane", "-d", "-s", paneID, "-t", string(tmux.CoordTargetExact(at.session)))
			},
			moved: windowChanged,
			want:  "a window of its own in the same session",
		},
		{
			name: "it keeps them when an earlier window is closed under renumber-windows on",
			perform: func(t *testing.T, ts *tmuxtest.Socket, idx state.Index, at pendingAddr) {
				ts.Run(t, "kill-window", "-t", pendingWindowTarget(at.session, otherWindowIndex(t, idx, at)))
			},
			moved: windowChanged,
			want:  "a lower window index, renumbered by the close",
		},
		{
			name: "it keeps them when the pane is moved back",
			perform: func(t *testing.T, ts *tmuxtest.Socket, idx state.Index, at pendingAddr) {
				ts.Run(t, "move-pane", "-d", "-s", paneID,
					"-t", pendingWindowTarget(at.session, otherWindowIndex(t, idx, at)))
			},
			moved: windowChanged,
			want:  "another window of the same session",
		},
		{
			name: "it keeps them when the pane is moved to another session",
			perform: func(t *testing.T, ts *tmuxtest.Socket, idx state.Index, _ pendingAddr) {
				ts.Run(t, "move-pane", "-d", "-s", paneID,
					"-t", pendingWindowTarget(pendingOtherSession, firstWindowIndex(t, idx, pendingOtherSession)))
			},
			moved: sessionChanged,
			want:  "a window of " + pendingOtherSession,
		},
		{
			name: "it keeps them when the pane is respawned with -k",
			perform: func(t *testing.T, ts *tmuxtest.Socket, _ state.Index, _ pendingAddr) {
				ts.Run(t, "respawn-pane", "-k", "-t", paneID)
			},
			// A respawn replaces the pane's process and moves nothing, so the
			// address it keeps is the assertion.
			moved: func(before, after pendingAddr) bool { return after == before },
			want:  "the address it already had",
		},
		{
			name: "it keeps them when the session is renamed",
			perform: func(t *testing.T, ts *tmuxtest.Socket, _ state.Index, at pendingAddr) {
				ts.Run(t, "rename-session", "-t", string(tmux.CoordTargetExact(at.session)), pendingRenamedSession)
			},
			moved: sessionChanged,
			want:  "the session under its new name",
		},
	}
}

// seedPendingFixture builds a server holding a three-window subject session
// whose middle window carries two panes, plus a second session to move the pane
// into, and returns the subject pane's id. renumber-windows is set explicitly:
// it is off in vanilla tmux, and without it closing a window leaves the
// surviving indices alone and the renumbering case proves nothing.
func seedPendingFixture(t *testing.T, ts *tmuxtest.Socket) string {
	t.Helper()
	dir := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", pendingSubjectSession, "-c", dir)
	ts.Run(t, "set-option", "-g", "renumber-windows", "on")
	subjectWindowPane := pendingNewWindow(t, ts, dir)
	pendingNewWindow(t, ts, dir)
	ts.Run(t, "new-session", "-d", "-s", pendingOtherSession, "-c", dir)
	waitForListedSessions(t, ts, pendingSubjectSession, pendingOtherSession)
	return strings.TrimSpace(ts.Run(t, "split-window", "-d", "-P", "-F", "#{pane_id}",
		"-t", subjectWindowPane, "-c", dir))
}

func pendingNewWindow(t *testing.T, ts *tmuxtest.Socket, dir string) string {
	t.Helper()
	return strings.TrimSpace(ts.Run(t, "new-window", "-d", "-P", "-F", "#{pane_id}",
		"-t", string(tmux.CoordTargetExact(pendingSubjectSession)), "-c", dir))
}

// capturePendingUntil reads through Portal's own capture until the pane has
// settled where the move put it, so a failure reports the state tmux came to
// rest in rather than one mid-flight.
func capturePendingUntil(
	t *testing.T,
	client state.CaptureClient,
	prev state.Index,
	settled func(pendingAddr) bool,
) (state.Index, map[string]struct{}, pendingAddr, state.Pane) {
	t.Helper()
	deadline := time.Now().Add(pendingSettleTimeout)
	for {
		idx, pending, err := state.CaptureStructure(client, nil, &prev, nil)
		if err != nil {
			t.Fatalf("CaptureStructure: %v", err)
		}
		addr, record := locatePendingPane(t, idx, pendingPaneToken)
		if settled(addr) || time.Now().After(deadline) {
			return idx, pending, addr, record
		}
		time.Sleep(pendingSettlePoll)
	}
}

// locatePendingPane finds the one pane carrying the token, fatalling on any
// other count: a rearrangement that duplicated either the token or the pane is
// a failure of the same property the marker rests on.
func locatePendingPane(t *testing.T, idx state.Index, token string) (pendingAddr, state.Pane) {
	t.Helper()
	var addrs []pendingAddr
	var record state.Pane
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PortalPaneID != token {
					continue
				}
				addrs = append(addrs, pendingAddr{session: s.Name, window: w.Index, pane: p.Index})
				record = p
			}
		}
	}
	if len(addrs) != 1 {
		t.Fatalf("%d live panes carry token %q: %v", len(addrs), token, addrs)
	}
	return addrs[0], record
}

// pendingWindowTarget pins the session half of a window target the way the
// client does, so a live prefix sibling can never answer in its place.
func pendingWindowTarget(session string, window int) string {
	return fmt.Sprintf("=%s:%d", session, window)
}

// otherWindowIndex returns the subject session's lowest window index that is
// not the pane's own, which is both the window an earlier-window close reaches
// and the window the pane is moved back into.
func otherWindowIndex(t *testing.T, idx state.Index, at pendingAddr) int {
	t.Helper()
	for _, w := range sessionWindowIndices(t, idx, at.session) {
		if w != at.window {
			return w
		}
	}
	t.Fatalf("session %q holds no window besides the pane's own %d", at.session, at.window)
	return 0
}

func firstWindowIndex(t *testing.T, idx state.Index, session string) int {
	t.Helper()
	return sessionWindowIndices(t, idx, session)[0]
}

func sessionWindowIndices(t *testing.T, idx state.Index, session string) []int {
	t.Helper()
	for _, s := range idx.Sessions {
		if s.Name != session {
			continue
		}
		indices := make([]int, 0, len(s.Windows))
		for _, w := range s.Windows {
			indices = append(indices, w.Index)
		}
		if len(indices) == 0 {
			t.Fatalf("session %q holds no windows", session)
		}
		return indices
	}
	t.Fatalf("the capture holds no session %q", session)
	return nil
}

func sortedPendingKeys(pending map[string]struct{}) []string {
	return slices.Sorted(maps.Keys(pending))
}
