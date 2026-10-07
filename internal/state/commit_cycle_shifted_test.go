package state_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

var errNotWritten = errors.New("not written")

// A shifted run: restore closed the gap at window 1, moving windows 2 and 3
// down by one, so the pane moved 3→2 is live at the moved 2→1 pane's saved
// address.
var (
	unmovedPane   = movedPane{"work", 0, 0, ""}
	shiftedASaved = movedPane{"work", 2, 0, waitingPaneToken}
	shiftedALive  = movedPane{"work", 1, 0, waitingPaneToken}
	shiftedBSaved = movedPane{"work", 3, 0, otherMovedToken}
	shiftedBLive  = movedPane{"work", 2, 0, otherMovedToken}
)

func seedShiftedRun(t *testing.T, dir string) state.Index {
	t.Helper()
	return seedMoved(t, dir, unmovedPane, shiftedASaved, shiftedBSaved)
}

func shiftedRunLive() []movedPane { return []movedPane{unmovedPane, shiftedALive, shiftedBLive} }

type paneWrite struct {
	pane movedPane
	data string
}

// writesPanes writes each capture in order, recording under the pane's key
// what its Write returned.
func writesPanes(writes []paneWrite, results map[string]error) dumpFunc {
	return func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		changed := false
		for _, pw := range writes {
			data := []byte(pw.data)
			written, err := w.Write(pw.pane.key(), data, xxhash.Sum64(data))
			if err == nil && !written {
				err = errNotWritten
			}
			results[pw.pane.key()] = err
			changed = changed || written
		}
		return changed, nil
	}
}

func assertWrittenAt(t *testing.T, dir string, committed state.Index, live movedPane, data string) {
	t.Helper()
	if got := recordAt(t, committed, live).ScrollbackFile; got != live.stored() {
		t.Errorf("sessions.json names %q for %s, want its live positional file", got, live.key())
	}
	if got := readScrollback(t, dir, live.file()); got != data {
		t.Errorf("%s = %q, want %q", live.file(), got, data)
	}
}

func TestRunCommitCycleKeepsEveryShiftedPanesTranscriptOverAnUnconfirmedEmptyCapture(t *testing.T) {
	bWrites := []struct {
		name   string
		writes []paneWrite
	}{
		{"the pane moved 3→2 is not written", nil},
		{"the pane moved 3→2 is written first", []paneWrite{{shiftedBLive, "b-capture"}}},
	}
	for _, refusal := range emptyCaptureRefusals {
		for _, b := range bWrites {
			t.Run(b.name+", the read after the empty capture "+refusal.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedShiftedRun(t, dir)
				client := &movedClient{live: shiftedRunLive(), later: refusal.later}
				results := map[string]error{}
				writes := append(append([]paneWrite{}, b.writes...), paneWrite{shiftedALive, ""})

				if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPanes(writes, results))); err != nil {
					t.Fatalf("RunCommitCycle: %v", err)
				}

				if err := results[shiftedALive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
					t.Errorf("Write(%s) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", shiftedALive.key(), err)
				}
				if client.laterReads != 1 {
					t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
				}
				if !scrollbackAbsent(t, dir, shiftedALive.file()) {
					t.Errorf("%s written at the moved pane's live positional path", shiftedALive.file())
				}
				committed := onDiskIndex(t, dir)
				assertHeldOnTranscript(t, dir, committed, shiftedASaved, shiftedALive)
				if b.writes != nil {
					if err := results[shiftedBLive.key()]; err != nil {
						t.Errorf("Write(%s) = %v; want a write", shiftedBLive.key(), err)
					}
					assertWrittenUnderToken(t, dir, committed, shiftedBLive, "b-capture")
				} else {
					assertHeldOnTranscript(t, dir, committed, shiftedBSaved, shiftedBLive)
				}
				assertSavedScrollbackPresent(t, dir)
				assertNoFileOnTwoRecords(t, committed)
			})
		}
	}
}

func TestRunCommitCycleKeepsNamingEveryShiftedPanesTranscriptUntilItsCaptureIsWritten(t *testing.T) {
	dir := t.TempDir()
	seed := seedShiftedRun(t, dir)
	live := shiftedRunLive()
	hm := state.HashMap{}
	assertAllKept := func(stage string) {
		t.Helper()
		committed := onDiskIndex(t, dir)
		assertHeldOnTranscript(t, dir, committed, shiftedASaved, shiftedALive)
		assertHeldOnTranscript(t, dir, committed, shiftedBSaved, shiftedBLive)
		if got := recordAt(t, committed, unmovedPane).ScrollbackFile; got != unmovedPane.stored() {
			t.Errorf("after %s: sessions.json names %q for the unmoved pane, want %q", stage, got, unmovedPane.stored())
		}
		if got := readScrollback(t, dir, unmovedPane.file()); got != savedBytes(unmovedPane) {
			t.Errorf("after %s: unmoved pane's file = %q, want its saved bytes", stage, got)
		}
		assertSavedScrollbackPresent(t, dir)
		assertNoFileOnTwoRecords(t, committed)
	}

	capture, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, seed, hm, dumpsNothing))
	if err != nil {
		t.Fatalf("tick with every capture-pane refused: %v", err)
	}
	assertAllKept("a tick with every capture-pane refused")

	if _, err := state.RunCommitCycle(movedCommitNow(t, &movedClient{live: live}, dir)); err != nil {
		t.Fatalf("commit-now: %v", err)
	}
	assertAllKept("commit-now")

	if _, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, capture.Index, nil, nil)); err != nil {
		t.Fatalf("shutdown flush with nothing dumped: %v", err)
	}
	assertAllKept("a shutdown flush with nothing dumped")
}

func TestRunCommitCycleKeepsEachSwappedPanesOwnTranscript(t *testing.T) {
	xSaved := movedPane{"work", 1, 0, waitingPaneToken}
	xLive := movedPane{"work", 2, 0, waitingPaneToken}
	ySaved := movedPane{"work", 2, 0, otherMovedToken}
	yLive := movedPane{"work", 1, 0, otherMovedToken}
	orders := []struct {
		name   string
		writes []paneWrite
	}{
		{"Y written first", []paneWrite{{yLive, "y-capture"}, {xLive, ""}}},
		{"X refused first", []paneWrite{{xLive, ""}, {yLive, "y-capture"}}},
	}
	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, xSaved, ySaved)
			client := &movedClient{live: []movedPane{yLive, xLive}, later: func() (int, error) { return 0, errors.New("server exited") }}
			results := map[string]error{}

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPanes(order.writes, results))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[xLive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write(X) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", err)
			}
			if err := results[yLive.key()]; err != nil {
				t.Errorf("Write(Y) = %v; want a write", err)
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, xSaved, xLive)
			assertWrittenUnderToken(t, dir, committed, yLive, "y-capture")
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleKeepsAMovedPanesTranscriptBesideANewPaneAtItsSavedAddress(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	newPane := movedPane{"work", 2, 0, ""}
	arrivals := []struct {
		name       string
		priorCycle bool
		newWritten bool
	}{
		{"in the same cycle", false, false},
		{"in a later cycle after the moved pane's record was committed", true, true},
	}
	for _, arrival := range arrivals {
		t.Run(arrival.name, func(t *testing.T) {
			dir := t.TempDir()
			prev := seedMoved(t, dir, saved)
			if arrival.priorCycle {
				capture, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{live}}, dir, prev, state.HashMap{}, dumpsNothing))
				if err != nil {
					t.Fatalf("cycle before the new pane: %v", err)
				}
				assertNoFileOnTwoRecords(t, onDiskIndex(t, dir))
				prev = capture.Index
			}
			client := &movedClient{live: []movedPane{live, newPane}, later: func() (int, error) { return ownServerPID + 1, nil }}
			results := map[string]error{}
			writes := []paneWrite{{newPane, "new-capture"}, {live, ""}}

			if _, err := state.RunCommitCycle(movedTick(client, dir, prev, state.HashMap{}, writesPanes(writes, results))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[live.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write(moved) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", err)
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, saved, live)
			if arrival.newWritten {
				if err := results[newPane.key()]; err != nil {
					t.Errorf("Write(new pane) = %v; want a write", err)
				}
				assertWrittenAt(t, dir, committed, newPane, "new-capture")
			} else {
				if err := results[newPane.key()]; !errors.Is(err, errNotWritten) {
					t.Errorf("Write(new pane) = %v; want it deferred", err)
				}
				if got := readScrollback(t, dir, newPane.file()); got != savedBytes(saved) {
					t.Errorf("%s = %q, want the moved pane's saved bytes left in place", newPane.file(), got)
				}
			}
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleEndingUncommittedAfterALinkKeepsTheMovedPanesTranscript(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	newPane := movedPane{"work", 2, 0, ""}
	ends := []struct {
		name string
		end  func(t *testing.T, dir string) (bool, error)
	}{
		{"its dump fails", func(*testing.T, string) (bool, error) {
			return true, errors.New("injected dump failure")
		}},
		{"its sessions.json write fails", func(t *testing.T, dir string) (bool, error) {
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatalf("chmod state dir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			return true, nil
		}},
	}
	for _, tc := range ends {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, saved)
			live2 := []movedPane{live, newPane}
			dump := func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				data := []byte("new-capture")
				if written, err := w.Write(newPane.key(), data, xxhash.Sum64(data)); written || err != nil {
					t.Fatalf("Write(new pane) = %t, %v; want it deferred", written, err)
				}
				return tc.end(t, dir)
			}

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: live2}, dir, seed, state.HashMap{}, dump)); err == nil {
				t.Fatal("cycle returned nil, want it to end uncommitted")
			}
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatalf("restore state dir mode: %v", err)
			}
			assertSavedScrollbackPresent(t, dir)
			if got := recordAt(t, onDiskIndex(t, dir), saved).ScrollbackFile; got != saved.stored() {
				t.Fatalf("sessions.json names %q for the moved pane, want its saved record %q", got, saved.stored())
			}
			if got := readScrollback(t, dir, saved.file()); got != savedBytes(saved) {
				t.Errorf("%s, which the moved pane's record names, = %q, want its saved bytes", saved.file(), got)
			}

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: live2}, dir, seed, state.HashMap{}, dumpsNothing)); err != nil {
				t.Fatalf("next committing cycle: %v", err)
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, saved, live)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleJudgesAMovedPaneAsBeforeWhenItsTranscriptCannotBeLinked(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	cases := []struct {
		name   string
		others []movedPane
		want   string
	}{
		{"no other record names its saved file", nil, saved.stored()},
		{"a new pane's record names its saved file", []movedPane{{"work", 2, 0, ""}}, live.stored()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, saved)
			denyScrollbackWrites(t, dir)
			logger, sink := openTempLogger(t)
			cycle := movedTick(&movedClient{live: append([]movedPane{live}, tc.others...)}, dir, seed, state.HashMap{}, dumpsNothing)
			cycle.Logger = logger

			if _, err := state.RunCommitCycle(cycle); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			committed := onDiskIndex(t, dir)
			if got := recordAt(t, committed, live).ScrollbackFile; got != tc.want {
				t.Errorf("sessions.json names %q for the moved pane, want %q", got, tc.want)
			}
			assertNoTokenFile(t, dir, waitingPaneToken)
			assertNoFileOnTwoRecords(t, committed)
			rec := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "link failure warning")
			if got, want := rec.AttrOrEmpty("pane_key"), live.key(); got != want {
				t.Errorf("pane_key = %q, want %q", got, want)
			}
			if got, want := rec.AttrOrEmpty("path"), saved.stored(); got != want {
				t.Errorf("path = %q, want %q", got, want)
			}
			if !rec.HasAttr("error") {
				t.Errorf("warning carries no error attr: %v", rec.Keys)
			}
		})
	}
}

func TestRunCommitCycleJudgesAMovedPaneWhoseTokenTheRuleRefusesAsBefore(t *testing.T) {
	const refused = "not-a-token"
	saved := movedPane{"work", 2, 0, refused}
	live := movedPane{"work", 1, 0, refused}
	cases := []struct {
		name   string
		others []movedPane
		want   string
	}{
		{"no other record names its saved file", nil, saved.stored()},
		{"a new pane's record names its saved file", []movedPane{{"work", 2, 0, ""}}, live.stored()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, saved)

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: append([]movedPane{live}, tc.others...)}, dir, seed, state.HashMap{}, dumpsNothing)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if got := recordAt(t, onDiskIndex(t, dir), live).ScrollbackFile; got != tc.want {
				t.Errorf("sessions.json names %q for the moved pane, want %q", got, tc.want)
			}
			entries, err := os.ReadDir(state.ScrollbackDir(dir))
			if err != nil {
				t.Fatalf("read scrollback dir: %v", err)
			}
			for _, e := range entries {
				if e.Name() != saved.file() {
					t.Errorf("scrollback dir holds %s, want only %s", e.Name(), saved.file())
				}
			}
		})
	}
}
