package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

const (
	movedTranscript = "moved-transcript"
	movedCapture    = "moved-after-hydration"
	otherMovedToken = "ef34gh"
)

// movedPane places one pane at an address, carrying token ("" for none).
type movedPane struct {
	session string
	window  int
	pane    int
	token   string
}

func (p movedPane) key() string { return state.SanitizePaneKey(p.session, p.window, p.pane) }

func (p movedPane) file() string { return p.key() + ".bin" }

func (p movedPane) stored() string { return "scrollback/" + p.file() }

// movedClient is the live state after restore: every pane listed, none
// skeleton-marked, and waiting only where waiting names its key. It answers
// the cycle's own confirmation as ownServerPID and every confirmation after it
// with later.
type movedClient struct {
	live       []movedPane
	waiting    map[string]bool
	later      func() (int, error)
	cycleReads int
	laterReads int
}

func (c *movedClient) ShowAllServerOptions() (string, error)  { return "", nil }
func (c *movedClient) ShowEnvironment(string) (string, error) { return "", nil }

func (c *movedClient) ListSessionNamesProbe() ([]string, error) {
	var names []string
	for _, p := range c.live {
		if !slices.Contains(names, p.session) {
			names = append(names, p.session)
		}
	}
	return names, nil
}

func (c *movedClient) ListAllPanesWithFormat(string) (string, error) {
	lines := make([]string, 0, len(c.live))
	for _, p := range c.live {
		pending := ""
		if c.waiting[p.key()] {
			pending = "1"
		}
		lines = append(lines, paneLineWithPending(p.session, p.window, "main", "tiled", false, true, p.pane, "/tmp", true, "zsh", p.token, pending))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *movedClient) ConfirmAnswering() (int, error) {
	if c.cycleReads == 0 {
		c.cycleReads++
		return ownServerPID, nil
	}
	c.laterReads++
	if c.later == nil {
		return ownServerPID, nil
	}
	return c.later()
}

// seedMoved commits an index holding each saved pane on its positional file,
// with that file on disk holding the pane's saved bytes.
func seedMoved(t *testing.T, dir string, saved ...movedPane) state.Index {
	t.Helper()
	idx := state.Index{Version: state.SchemaVersion}
	for _, p := range saved {
		si := slices.IndexFunc(idx.Sessions, func(s state.Session) bool { return s.Name == p.session })
		if si < 0 {
			idx.Sessions = append(idx.Sessions, state.Session{Name: p.session, Environment: map[string]string{}})
			si = len(idx.Sessions) - 1
		}
		idx.Sessions[si].Windows = append(idx.Sessions[si].Windows, state.Window{
			Index: p.window, Name: "main", Layout: "tiled", Active: true,
			Panes: []state.Pane{{Index: p.pane, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: p.stored(), PortalPaneID: p.token}},
		})
		seedScrollback(t, dir, p.file(), movedTranscript+"-"+p.key())
	}
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	idx.Canonicalize()
	return idx
}

func movedTick(client *movedClient, dir string, prev state.Index, hm state.HashMap, dump dumpFunc) state.CommitCycle {
	return state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    client,
		Dir:       dir,
		LoadPrev:  func() *state.Index { return &prev },
		HashMap:   hm,
		Dump:      dump,
	}
}

func movedCommitNow(t *testing.T, client *movedClient, dir string) state.CommitCycle {
	return state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    client,
		Dir:       dir,
		LoadPrev: func() *state.Index {
			idx := onDiskIndex(t, dir)
			return &idx
		},
	}
}

func writesPane(key string, data []byte, written *bool, writeErr *error) dumpFunc {
	return func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		*written, *writeErr = w.Write(key, data, xxhash.Sum64(data))
		return *written, nil
	}
}

func recordAt(t *testing.T, idx state.Index, at movedPane) state.Pane {
	t.Helper()
	for _, s := range idx.Sessions {
		if s.Name != at.session {
			continue
		}
		for _, w := range s.Windows {
			if w.Index != at.window {
				continue
			}
			for _, p := range w.Panes {
				if p.Index == at.pane {
					return p
				}
			}
		}
	}
	t.Fatalf("index holds no pane at %s: %+v", at.key(), idx.Sessions)
	return state.Pane{}
}

func scrollbackAbsent(t *testing.T, dir, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s: %v", name, err)
	}
	return err != nil
}

func assertNoFileOnTwoRecords(t *testing.T, idx state.Index) {
	t.Helper()
	naming := map[string][]string{}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				naming[p.ScrollbackFile] = append(naming[p.ScrollbackFile], state.SanitizePaneKey(s.Name, w.Index, p.Index))
			}
		}
	}
	for file, keys := range naming {
		if len(keys) > 1 {
			t.Errorf("%q is named by %d records: %v", file, len(keys), keys)
		}
	}
}

func tokenFile(token string) string { return "pane-" + token + ".bin" }

func savedBytes(p movedPane) string { return movedTranscript + "-" + p.key() }

// assertHeldOnTranscript checks the committed record at live names the moved
// pane's token-named transcript, holding the bytes saved at saved.
func assertHeldOnTranscript(t *testing.T, dir string, committed state.Index, saved, live movedPane) {
	t.Helper()
	if got, want := recordAt(t, committed, live).ScrollbackFile, state.PendingScrollbackFile(live.token); got != want {
		t.Errorf("sessions.json names %q for the pane moved to %s, want its token-named transcript %q", got, live.key(), want)
	}
	if got := readScrollback(t, dir, tokenFile(live.token)); got != savedBytes(saved) {
		t.Errorf("token-named transcript of %s = %q, want its saved bytes", live.key(), got)
	}
}

// assertMovedKeptOnSaved checks the committed state keeps the moved pane on
// its token-named transcript, holding its saved bytes, with nothing at its
// live file.
func assertMovedKeptOnSaved(t *testing.T, dir string, saved, live movedPane) {
	t.Helper()
	committed := onDiskIndex(t, dir)
	assertHeldOnTranscript(t, dir, committed, saved, live)
	if !scrollbackAbsent(t, dir, live.file()) {
		t.Errorf("%s written at the moved pane's live positional path", live.file())
	}
	assertSavedScrollbackPresent(t, dir)
	assertNoFileOnTwoRecords(t, committed)
}

// emptyCaptureRefusals are the answers to the read sent after an empty
// capture that do not confirm it.
var emptyCaptureRefusals = []struct {
	name  string
	later func() (int, error)
}{
	{"refused", func() (int, error) { return 0, errors.New("server exited") }},
	{"answered by another server on the socket", func() (int, error) { return ownServerPID + 1, nil }},
	{"answered naming no server", func() (int, error) { return 0, nil }},
}

type paneMove struct {
	name        string
	saved, live movedPane
}

func paneMoves() []paneMove {
	return []paneMove{
		{"restore closed a window-index gap", movedPane{"work", 2, 0, waitingPaneToken}, movedPane{"work", 1, 0, waitingPaneToken}},
		{"tmux renumbered its window", movedPane{"work", 3, 0, waitingPaneToken}, movedPane{"work", 0, 0, waitingPaneToken}},
		{"its session was renamed", movedPane{"old", 1, 0, waitingPaneToken}, movedPane{"work", 1, 0, waitingPaneToken}},
	}
}

func TestRunCommitCycleKeepsAMovedPanesTranscriptOverAnUnconfirmedEmptyCapture(t *testing.T) {
	for _, move := range paneMoves() {
		for _, refusal := range emptyCaptureRefusals {
			t.Run(move.name+", the read after the empty capture "+refusal.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedMoved(t, dir, move.saved)
				client := &movedClient{live: []movedPane{move.live}, later: refusal.later}
				var written bool
				var writeErr error

				if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPane(move.live.key(), nil, &written, &writeErr))); err != nil {
					t.Fatalf("RunCommitCycle: %v", err)
				}

				if written || !errors.Is(writeErr, state.ErrUnconfirmedEmptyCapture) {
					t.Errorf("Write = %t, %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", written, writeErr)
				}
				if client.laterReads != 1 {
					t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
				}
				assertMovedKeptOnSaved(t, dir, move.saved, move.live)
			})
		}
	}
}

func TestRunCommitCycleKeepsNamingAMovedPanesTranscriptUntilItsCaptureIsWritten(t *testing.T) {
	for _, move := range paneMoves() {
		t.Run(move.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, move.saved)
			live := []movedPane{move.live}
			hm := state.HashMap{}

			capture, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, seed, hm, dumpsNothing))
			if err != nil {
				t.Fatalf("tick with its capture-pane refused: %v", err)
			}
			assertMovedKeptOnSaved(t, dir, move.saved, move.live)

			if _, err := state.RunCommitCycle(movedCommitNow(t, &movedClient{live: live}, dir)); err != nil {
				t.Fatalf("commit-now: %v", err)
			}
			assertMovedKeptOnSaved(t, dir, move.saved, move.live)

			capture, err = state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, capture.Index, hm, dumpsNothing))
			if err != nil {
				t.Fatalf("second refused tick: %v", err)
			}
			assertMovedKeptOnSaved(t, dir, move.saved, move.live)

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, capture.Index, nil, nil)); err != nil {
				t.Fatalf("shutdown flush with nothing dumped: %v", err)
			}
			assertMovedKeptOnSaved(t, dir, move.saved, move.live)
		})
	}
}

func TestRunCommitCycleFilesAMovedPaneAtItsLivePositionalFileOnceItsCaptureIsWritten(t *testing.T) {
	captures := []struct {
		name       string
		data       string
		laterReads int
	}{
		{"a non-empty capture", movedCapture, 0},
		{"an empty capture its own server confirms against the saved file", "", 1},
	}
	for _, move := range paneMoves() {
		for _, tc := range captures {
			t.Run(move.name+", "+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedMoved(t, dir, move.saved)
				client := &movedClient{live: []movedPane{move.live}}
				var written bool
				var writeErr error

				capture, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPane(move.live.key(), []byte(tc.data), &written, &writeErr)))
				if err != nil {
					t.Fatalf("RunCommitCycle: %v", err)
				}

				if !written || writeErr != nil {
					t.Errorf("Write = %t, %v; want a write", written, writeErr)
				}
				if client.laterReads != tc.laterReads {
					t.Errorf("confirmation reads after the capture = %d, want %d", client.laterReads, tc.laterReads)
				}
				if got := recordAt(t, capture.Index, move.live).ScrollbackFile; got != move.live.stored() {
					t.Errorf("returned index names %q, want the live positional file", got)
				}
				if got := recordAt(t, onDiskIndex(t, dir), move.live).ScrollbackFile; got != move.live.stored() {
					t.Errorf("sessions.json names %q, want the live positional file", got)
				}
				if got := readScrollback(t, dir, move.live.file()); got != tc.data {
					t.Errorf("live positional file = %q, want %q", got, tc.data)
				}
				if !scrollbackAbsent(t, dir, move.saved.file()) {
					t.Errorf("%s still on disk once a commit naming the live capture ran its housekeeping", move.saved.file())
				}
				if !scrollbackAbsent(t, dir, tokenFile(move.live.token)) {
					t.Errorf("token-named transcript still on disk once a commit naming the live capture ran its housekeeping")
				}
				assertSavedScrollbackPresent(t, dir)
			})
		}
	}
}

func TestRunCommitCycleHoldsAMovedPaneOnItsTranscriptWhenAnotherRecordNamesItsSavedFile(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	otherSaved := movedPane{"work", 3, 0, otherMovedToken}
	cases := []struct {
		name     string
		saved    []movedPane
		occupant movedPane
		heldOn   string
	}{
		{"a new pane occupies the saved address", []movedPane{saved}, movedPane{"work", 2, 0, ""}, ""},
		{"another moved pane is live at the saved address", []movedPane{saved, otherSaved}, movedPane{"work", 2, 0, otherMovedToken}, state.PendingScrollbackFile(otherMovedToken)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, tc.saved...)
			client := &movedClient{live: []movedPane{live, tc.occupant}, later: func() (int, error) { return ownServerPID + 1, nil }}
			var written bool
			var writeErr error

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPane(live.key(), nil, &written, &writeErr))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if written || !errors.Is(writeErr, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write = %t, %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", written, writeErr)
			}
			if client.laterReads != 1 {
				t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, saved, live)
			if !scrollbackAbsent(t, dir, live.file()) {
				t.Errorf("%s written at the moved pane's live positional path", live.file())
			}
			wantOccupant := tc.occupant.stored()
			if tc.heldOn != "" {
				wantOccupant = tc.heldOn
				if got := readScrollback(t, dir, tokenFile(otherMovedToken)); got != savedBytes(otherSaved) {
					t.Errorf("occupant's token-named transcript = %q, want its saved bytes", got)
				}
			}
			if got := recordAt(t, committed, tc.occupant).ScrollbackFile; got != wantOccupant {
				t.Errorf("sessions.json names %q for the occupant, want %q", got, wantOccupant)
			}
			assertNoFileOnTwoRecords(t, committed)
			assertSavedScrollbackPresent(t, dir)
		})
	}
}

func TestRunCommitCycleHoldsNeitherMovedPaneWhenBothLastRecordsNameOneFile(t *testing.T) {
	dir := t.TempDir()
	shared := movedPane{"work", 2, 0, ""}
	first := movedPane{"work", 1, 0, waitingPaneToken}
	second := movedPane{"work", 4, 0, otherMovedToken}
	seed := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{{
		Name: "work", Environment: map[string]string{},
		Windows: []state.Window{
			{Index: 2, Name: "main", Layout: "tiled", Active: true, Panes: []state.Pane{{Index: 0, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: shared.stored(), PortalPaneID: waitingPaneToken}}},
			{Index: 3, Name: "main", Layout: "tiled", Panes: []state.Pane{{Index: 0, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: shared.stored(), PortalPaneID: otherMovedToken}}},
		},
	}}}
	seedScrollback(t, dir, shared.file(), movedTranscript)
	if err := state.Commit(dir, seed, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	seed.Canonicalize()
	client := &movedClient{live: []movedPane{first, second}, later: func() (int, error) { return 0, errors.New("server exited") }}
	results := map[string]error{}

	_, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		for _, p := range []movedPane{first, second} {
			written, err := w.Write(p.key(), nil, xxhash.Sum64(nil))
			if err == nil && !written {
				err = errors.New("not written")
			}
			results[p.key()] = err
		}
		return true, nil
	}))
	if err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	for key, err := range results {
		if err != nil {
			t.Errorf("Write(%s) = %v; want a write", key, err)
		}
	}
	if client.laterReads != 0 {
		t.Errorf("confirmation reads after the captures = %d, want none", client.laterReads)
	}
	committed := onDiskIndex(t, dir)
	for _, p := range []movedPane{first, second} {
		if got := recordAt(t, committed, p).ScrollbackFile; got != p.stored() {
			t.Errorf("sessions.json names %q for %s, want its own positional file", got, p.key())
		}
	}
	assertNoFileOnTwoRecords(t, committed)
}
