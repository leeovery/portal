package state_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const (
	carryTranscript = "x-transcript"
	carrySibling    = "tokzzz"
)

func carryTokenFile() string { return "pane-" + waitingPaneToken + ".bin" }

func carryTokenPath() string { return "scrollback/" + carryTokenFile() }

// carryPrev is the index X was committed in: session foo holding X (token T,
// filed under its token) beside an unmarked sibling, and a session other.
func carryPrev() state.Index {
	return prevIndexOf(
		prevPane{"foo", 0, state.Pane{Index: 0, CWD: "/x", Active: true, CurrentCommand: "claude", ScrollbackFile: carryTokenPath(), PortalPaneID: waitingPaneToken}},
		prevPane{"foo", 0, state.Pane{Index: 1, CWD: "/y", CurrentCommand: "zsh", ScrollbackFile: "scrollback/foo__0.1.bin"}},
		prevPane{"other", 0, state.Pane{Index: 0, CWD: "/o", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/other__0.0.bin"}},
	)
}

func seedCarryPrev(t *testing.T, dir string, prev state.Index) {
	t.Helper()
	seedScrollback(t, dir, carryTokenFile(), carryTranscript)
	seedScrollback(t, dir, "foo__0.1.bin", "y-body")
	seedScrollback(t, dir, "other__0.0.bin", "o-body")
	if err := state.Commit(dir, prev, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

// carryWorld is one capture's view of tmux: the session-name read, the pane
// rows and the per-session environment failures.
type carryWorld struct {
	names   []string
	rows    []string
	envErrs map[string]error
}

func (w carryWorld) client(t *testing.T) state.CaptureCycleClient {
	return tmux.NewClient((&captureMock{
		listSessions: listSessionsFor(w.names...),
		listPanes:    strings.Join(w.rows, "\n"),
		envErrs:      w.envErrs,
		t:            t,
	}).commander())
}

func xRow(session string, pending bool) string {
	flag := ""
	if pending {
		flag = "1"
	}
	return paneLineWithPending(session, 0, "main", "L", false, true, 0, "/live", true, "zsh", waitingPaneToken, flag)
}

func siblingRow(session string) string {
	return paneLineWithPending(session, 0, "main", "L", false, true, 1, "/live", false, "zsh", "", "")
}

func otherRow() string {
	return paneLineWithPending("other", 0, "main", "L", false, true, 0, "/o", true, "zsh", "", "")
}

// committer runs one committing cycle against a world, the way the daemon's
// tick or commit-now does.
type committer struct {
	name string
	// run takes the previous index the daemon holds in memory; commit-now
	// ignores it and reads sessions.json under the lock.
	run func(t *testing.T, dir string, w carryWorld, memPrev state.Index) (state.CaptureCycle, error)
}

var committers = []committer{
	{
		name: "daemon tick",
		run: func(t *testing.T, dir string, w carryWorld, memPrev state.Index) (state.CaptureCycle, error) {
			return state.RunCommitCycle(state.CommitCycle{
				Client:   w.client(t),
				Dir:      dir,
				LoadPrev: func() *state.Index { return &memPrev },
				HashMap:  state.HashMap{},
				Dump:     func(state.CaptureCycle) (bool, error) { return false, nil },
			})
		},
	},
	{
		name: "commit-now",
		run: func(t *testing.T, dir string, w carryWorld, _ state.Index) (state.CaptureCycle, error) {
			return state.RunCommitCycle(state.CommitCycle{
				Client: w.client(t),
				Dir:    dir,
				LoadPrev: func() *state.Index {
					idx, _, err := state.ReadIndex(dir)
					if err != nil {
						t.Errorf("ReadIndex: %v", err)
					}
					return &idx
				},
			})
		},
	},
}

func mustCommit(t *testing.T, c committer, dir string, w carryWorld, memPrev state.Index) state.CaptureCycle {
	t.Helper()
	capture, err := c.run(t, dir, w, memPrev)
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	return capture
}

func sessionNamed(idx state.Index, name string) (state.Session, bool) {
	for _, s := range idx.Sessions {
		if s.Name == name {
			return s, true
		}
	}
	return state.Session{}, false
}

func assertCarriedFoo(t *testing.T, dir string, prev state.Index) {
	t.Helper()
	if got := readScrollback(t, dir, carryTokenFile()); got != carryTranscript {
		t.Errorf("%s = %q, want %q", carryTokenFile(), got, carryTranscript)
	}
	want, _ := sessionNamed(prev, "foo")
	got, found := sessionNamed(onDiskIndex(t, dir), "foo")
	if !found {
		t.Fatalf("sessions.json holds no foo: %v", sessionNamesOf(onDiskIndex(t, dir)))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("committed foo = %+v, want the previous index's %+v", got, want)
	}
}

func assertXFiledAt(t *testing.T, dir, session string) {
	t.Helper()
	p := findPane(onDiskIndex(t, dir), session, 0, 0)
	if p == nil {
		t.Fatalf("sessions.json holds no X at %s:0.0", session)
	}
	if p.ScrollbackFile != carryTokenPath() || p.PortalPaneID != waitingPaneToken {
		t.Errorf("X's record = %+v, want token %q naming %q", *p, waitingPaneToken, carryTokenPath())
	}
	if got := readScrollback(t, dir, carryTokenFile()); got != carryTranscript {
		t.Errorf("%s = %q, want %q", carryTokenFile(), got, carryTranscript)
	}
}

func noSuchFoo() map[string]error { return map[string]error{"foo": noSuchSessionErr("foo")} }

func TestCommitCycleCarriesAMissedWaitingPanesSession(t *testing.T) {
	scenarios := []struct {
		name    string
		missed  carryWorld
		reached carryWorld
		// liveName is the name X's session reaches the next capture under.
		liveName string
	}{
		{
			name:     "a rename landing before the pane read",
			missed:   carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("bar", true), siblingRow("bar"), otherRow()}, envErrs: noSuchFoo()},
			reached:  carryWorld{names: []string{"bar", "other"}, rows: []string{xRow("bar", true), siblingRow("bar"), otherRow()}},
			liveName: "bar",
		},
		{
			name:     "a rename landing after the pane read",
			missed:   carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: noSuchFoo()},
			reached:  carryWorld{names: []string{"bar", "other"}, rows: []string{xRow("bar", true), siblingRow("bar"), otherRow()}},
			liveName: "bar",
		},
		{
			name:     "an environment read failing on something other than vanishing",
			missed:   carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: map[string]error{"foo": errors.New("boom")}},
			reached:  carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}},
			liveName: "foo",
		},
	}
	for _, sc := range scenarios {
		for _, c := range committers {
			t.Run(sc.name+" under a "+c.name, func(t *testing.T) {
				dir := t.TempDir()
				prev := carryPrev()
				seedCarryPrev(t, dir, prev)

				first := mustCommit(t, c, dir, sc.missed, prev)
				assertCarriedFoo(t, dir, prev)

				mustCommit(t, c, dir, sc.reached, first.Index)
				assertXFiledAt(t, dir, sc.liveName)
				if sc.liveName != "foo" {
					if _, found := sessionNamed(onDiskIndex(t, dir), "foo"); found {
						t.Errorf("sessions.json still holds foo once X reached the capture as %s", sc.liveName)
					}
				}
			})
		}
	}
}

// staleCarryPrev is carryPrev as committed before X's re-file landed: X still
// named at its positional path.
func staleCarryPrev() state.Index {
	prev := carryPrev()
	prev.Sessions[0].Windows[0].Panes[0].ScrollbackFile = "scrollback/foo__0.0.bin"
	return prev
}

func TestCommitCycleRefilesACarriedWaitingRecordAStalePrevNamesPositionally(t *testing.T) {
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			prev := staleCarryPrev()
			seedScrollback(t, dir, "foo__0.1.bin", "y-body")
			seedScrollback(t, dir, "other__0.0.bin", "o-body")
			if err := state.Commit(dir, prev, false, nil); err != nil {
				t.Fatalf("seed sessions.json: %v", err)
			}
			// X's transcript already sits under its token; the positional file is gone.
			seedScrollback(t, dir, carryTokenFile(), carryTranscript)
			missed := carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: noSuchFoo()}

			mustCommit(t, c, dir, missed, prev)

			assertXFiledAt(t, dir, "foo")
		})
	}
}

func TestCommitCycleCarriedSessionIsNotCaptured(t *testing.T) {
	dir := t.TempDir()
	prev := carryPrev()
	seedCarryPrev(t, dir, prev)
	w := carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: map[string]error{"foo": errors.New("boom")}}

	capture := mustCommit(t, committers[0], dir, w, prev)

	want := map[string]struct{}{
		state.SanitizePaneKey("foo", 0, 0): {},
		state.SanitizePaneKey("foo", 0, 1): {},
	}
	assertPaneKeySet(t, capture.Carried, want)
	if len(capture.Pending) != 0 {
		t.Errorf("pending set = %v, want empty: X's session did not reach the capture", capture.Pending)
	}
}

func TestCommitCycleDropsACarriedSessionOnceItsPaneIsGone(t *testing.T) {
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			prev := carryPrev()
			seedCarryPrev(t, dir, prev)
			killedMidCapture := carryWorld{names: []string{"foo", "other"}, rows: []string{xRow("foo", true), siblingRow("foo"), otherRow()}, envErrs: noSuchFoo()}

			first := mustCommit(t, c, dir, killedMidCapture, prev)
			assertCarriedFoo(t, dir, prev)

			mustCommit(t, c, dir, carryWorld{names: []string{"other"}, rows: []string{otherRow()}}, first.Index)
			if got := sessionNamesOf(onDiskIndex(t, dir)); !slices.Equal(got, []string{"other"}) {
				t.Errorf("committed sessions = %v, want [other]", got)
			}
		})
	}
}

func TestCommitCycleCarriesNothingWithoutALiveWaitingPane(t *testing.T) {
	worlds := map[string]carryWorld{
		"a session killed before the pane read":       {names: []string{"foo", "other"}, rows: []string{otherRow()}, envErrs: noSuchFoo()},
		"a missed session holding no waiting pane":    {names: []string{"foo", "other"}, rows: []string{xRow("foo", false), siblingRow("foo"), otherRow()}, envErrs: noSuchFoo()},
		"a renamed session holding no waiting pane":   {names: []string{"foo", "other"}, rows: []string{xRow("bar", false), siblingRow("bar"), otherRow()}, envErrs: noSuchFoo()},
		"a failing session holding no waiting pane":   {names: []string{"foo", "other"}, rows: []string{xRow("foo", false), siblingRow("foo"), otherRow()}, envErrs: map[string]error{"foo": errors.New("boom")}},
		"an internal session listing the waiting row": {names: []string{"foo", "other"}, rows: []string{xRow("_portal-saver", true), otherRow()}, envErrs: noSuchFoo()},
	}
	for name, w := range worlds {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			prev := carryPrev()
			seedCarryPrev(t, dir, prev)

			capture := mustCommit(t, committers[1], dir, w, prev)

			if got := sessionNamesOf(onDiskIndex(t, dir)); !slices.Equal(got, []string{"other"}) {
				t.Errorf("committed sessions = %v, want [other]", got)
			}
			if len(capture.Carried) != 0 {
				t.Errorf("carried set = %v, want empty", capture.Carried)
			}
		})
	}
}

func TestCaptureStructureCarriedSessionHoldsNoTokenAFreshRecordCarries(t *testing.T) {
	prev := carryPrev()
	prev.Sessions[0].Windows[0].Panes[1].PortalPaneID = carrySibling
	mock := &captureMock{
		listSessions: listSessionsFor("foo", "other"),
		listPanes: strings.Join([]string{
			xRow("bar", true),
			otherRow(),
			paneLineWithPending("other", 1, "moved", "L", false, false, 0, "/y", true, "zsh", carrySibling, ""),
		}, "\n"),
		envErrs: noSuchFoo(),
		t:       t,
	}

	idx, _, err := state.CaptureStructure(tmux.NewClient(mock.commander()), nil, &prev, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	holders := map[string][]string{}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PortalPaneID != "" {
					holders[p.PortalPaneID] = append(holders[p.PortalPaneID], state.SanitizePaneKey(s.Name, w.Index, p.Index))
				}
			}
		}
	}
	want := map[string][]string{
		waitingPaneToken: {state.SanitizePaneKey("foo", 0, 0)},
		carrySibling:     {state.SanitizePaneKey("other", 1, 0)},
	}
	if !reflect.DeepEqual(holders, want) {
		t.Errorf("token holders = %v, want %v", holders, want)
	}
	if p := findPane(idx, "foo", 0, 1); p == nil || p.ScrollbackFile != "scrollback/foo__0.1.bin" {
		t.Errorf("carried sibling = %+v, want its previous record without the token", p)
	}
}

func TestCommitCycleRefusesACarryOntoALiveSessionsName(t *testing.T) {
	// Within one capture foo is renamed to bar and baz is renamed to foo.
	collided := carryWorld{
		names:   []string{"baz", "foo"},
		rows:    []string{xRow("bar", true), paneLineWithPending("foo", 0, "main", "L", false, true, 0, "/z", true, "zsh", "", "")},
		envErrs: map[string]error{"baz": noSuchSessionErr("baz")},
	}
	settled := carryWorld{
		names: []string{"bar", "foo"},
		rows:  []string{xRow("bar", true), paneLineWithPending("foo", 0, "main", "L", false, true, 0, "/z", true, "zsh", "", "")},
	}
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			prev := carryPrev()
			seedCarryPrev(t, dir, prev)
			before, err := os.ReadFile(state.SessionsJSON(dir))
			if err != nil {
				t.Fatalf("read sessions.json: %v", err)
			}
			namesBefore := scrollbackNames(t, dir)

			if _, err := c.run(t, dir, collided, prev); err == nil {
				t.Fatal("cycle committed a carry landing on a live session's name, want an error")
			}

			after, err := os.ReadFile(state.SessionsJSON(dir))
			if err != nil {
				t.Fatalf("read sessions.json: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("sessions.json changed:\nbefore %s\nafter  %s", before, after)
			}
			if got := scrollbackNames(t, dir); !slices.Equal(got, namesBefore) {
				t.Errorf("scrollback files = %v, want %v untouched", got, namesBefore)
			}

			mustCommit(t, c, dir, settled, prev)
			assertXFiledAt(t, dir, "bar")
		})
	}
}

func TestCaptureStructureRefusedCarryReturnsNoPartialIndex(t *testing.T) {
	prev := carryPrev()
	mock := &captureMock{
		listSessions: listSessionsFor("baz", "foo"),
		listPanes:    strings.Join([]string{xRow("bar", true), paneLineWithPending("foo", 0, "main", "L", false, true, 0, "/z", true, "zsh", "", "")}, "\n"),
		envErrs:      map[string]error{"baz": noSuchSessionErr("baz")},
		t:            t,
	}

	idx, pending, err := state.CaptureStructure(tmux.NewClient(mock.commander()), nil, &prev, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if len(idx.Sessions) != 0 || pending == nil || len(pending) != 0 {
		t.Errorf("returned (%d sessions, pending %v), want the empty index and a non-nil empty set", len(idx.Sessions), pending)
	}
}
