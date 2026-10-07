package state_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/state"
)

// The contested layout: restore closed the gap at window 1, so Y, saved at
// work:3.0, is live at work:2.0 — the address X's saved record names.
var (
	contestedXSaved = movedPane{"work", 2, 0, waitingPaneToken}
	contestedXLive  = movedPane{"work", 1, 0, waitingPaneToken}
	contestedYSaved = movedPane{"work", 3, 0, otherMovedToken}
	contestedYLive  = movedPane{"work", 2, 0, otherMovedToken}
)

func contestedLive() []movedPane { return []movedPane{contestedXLive, contestedYLive} }

// xWaitingOrNot are the two states X can come back in.
var xWaitingOrNot = []struct {
	name    string
	waiting map[string]bool
}{
	{"X not waiting", nil},
	{"X waiting", map[string]bool{contestedXLive.key(): true}},
}

// uncommittedEnds end a cycle uncommitted after its dump has written.
var uncommittedEnds = []struct {
	name string
	end  func(t *testing.T, dir string) (bool, error)
}{
	{"its dump fails after the write", func(*testing.T, string) (bool, error) {
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

// writesThen writes each capture in order, failing the test on any write
// whose outcome is not want, then ends the dump with end.
func writesThen(t *testing.T, writes []paneWrite, want bool, end func() (bool, error)) dumpFunc {
	return func(_ state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		for _, pw := range writes {
			data := []byte(pw.data)
			written, err := w.Write(pw.pane.key(), data, xxhash.Sum64(data))
			if written != want || err != nil {
				t.Fatalf("Write(%s) = %t, %v; want %t, nil", pw.pane.key(), written, err, want)
			}
		}
		return end()
	}
}

func commits() (bool, error) { return true, nil }

func assertWrittenUnderToken(t *testing.T, dir string, committed state.Index, live movedPane, data string) {
	t.Helper()
	if got, want := recordAt(t, committed, live).ScrollbackFile, state.PendingScrollbackFile(live.token); got != want {
		t.Errorf("sessions.json names %q for %s, want its token-named transcript %q", got, live.key(), want)
	}
	if got := readScrollback(t, dir, tokenFile(live.token)); got != data {
		t.Errorf("token-named transcript of %s = %q, want %q", live.key(), got, data)
	}
}

func assertScrollbackHolds(t *testing.T, dir, name, want string) {
	t.Helper()
	if got := readScrollback(t, dir, name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func TestRunCommitCycleWritesAPaneAtAContestedNameUnderItsToken(t *testing.T) {
	for _, x := range xWaitingOrNot {
		t.Run(x.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, contestedXSaved, contestedYSaved)
			client := &movedClient{live: contestedLive(), waiting: x.waiting}
			dump := writesThen(t, []paneWrite{{contestedYLive, "y-capture"}}, true, func() (bool, error) {
				assertScrollbackHolds(t, dir, contestedXSaved.file(), savedBytes(contestedXSaved))
				return true, nil
			})

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, dump)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			committed := onDiskIndex(t, dir)
			assertWrittenUnderToken(t, dir, committed, contestedYLive, "y-capture")
			assertHeldOnTranscript(t, dir, committed, contestedXSaved, contestedXLive)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleEndingUncommittedAfterAContestedWriteKeepsEveryRecordsTranscript(t *testing.T) {
	for _, x := range xWaitingOrNot {
		for _, end := range uncommittedEnds {
			t.Run(x.name+", "+end.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedMoved(t, dir, contestedXSaved, contestedYSaved)
				before := sessionsJSONBytes(t, dir)
				client := &movedClient{live: contestedLive(), waiting: x.waiting}
				dump := writesThen(t, []paneWrite{{contestedYLive, "y-capture"}}, true, func() (bool, error) { return end.end(t, dir) })

				if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, dump)); err == nil {
					t.Fatal("cycle returned nil, want it to end uncommitted")
				}
				if err := os.Chmod(dir, 0o700); err != nil {
					t.Fatalf("restore state dir mode: %v", err)
				}

				if !bytes.Equal(sessionsJSONBytes(t, dir), before) {
					t.Errorf("sessions.json changed by a cycle that ended uncommitted")
				}
				committed := onDiskIndex(t, dir)
				for _, p := range []movedPane{contestedXSaved, contestedYSaved} {
					if got := recordAt(t, committed, p).ScrollbackFile; got != p.stored() {
						t.Errorf("sessions.json names %q for %s, want %q", got, p.key(), p.stored())
					}
					assertScrollbackHolds(t, dir, p.file(), savedBytes(p))
				}
				assertSavedScrollbackPresent(t, dir)
			})
		}
	}
}

func TestRunCommitCycleFilesAContestedPaneAtItsPositionalNameOnceTheTokenCommitHasLanded(t *testing.T) {
	dir := t.TempDir()
	seed := seedMoved(t, dir, contestedXSaved, contestedYSaved)
	hm := state.HashMap{}
	first := writesThen(t, []paneWrite{{contestedYLive, "y-capture"}}, true, commits)
	capture, err := state.RunCommitCycle(movedTick(&movedClient{live: contestedLive()}, dir, seed, hm, first))
	if err != nil {
		t.Fatalf("first cycle: %v", err)
	}
	assertWrittenUnderToken(t, dir, onDiskIndex(t, dir), contestedYLive, "y-capture")

	second := writesThen(t, []paneWrite{{contestedYLive, "y-capture-2"}}, true, commits)
	if _, err := state.RunCommitCycle(movedTick(&movedClient{live: contestedLive()}, dir, capture.Index, hm, second)); err != nil {
		t.Fatalf("second cycle: %v", err)
	}

	committed := onDiskIndex(t, dir)
	assertWrittenAt(t, dir, committed, contestedYLive, "y-capture-2")
	if !scrollbackAbsent(t, dir, tokenFile(contestedYLive.token)) {
		t.Errorf("Y's token-named transcript still on disk once a commit naming its positional file ran its housekeeping")
	}
	assertHeldOnTranscript(t, dir, committed, contestedXSaved, contestedXLive)
	assertSavedScrollbackPresent(t, dir)
	assertNoFileOnTwoRecords(t, committed)
}

func TestRunCommitCycleWritesEachSwappedPaneUnderItsOwnToken(t *testing.T) {
	xSaved := movedPane{"work", 1, 0, waitingPaneToken}
	xLive := movedPane{"work", 2, 0, waitingPaneToken}
	ySaved := movedPane{"work", 2, 0, otherMovedToken}
	yLive := movedPane{"work", 1, 0, otherMovedToken}
	orders := []struct {
		name   string
		writes []paneWrite
	}{
		{"X written first", []paneWrite{{xLive, "x-capture"}, {yLive, "y-capture"}}},
		{"Y written first", []paneWrite{{yLive, "y-capture"}, {xLive, "x-capture"}}},
	}
	assertPositionalUntouched := func(t *testing.T, dir string) {
		t.Helper()
		assertScrollbackHolds(t, dir, xSaved.file(), savedBytes(xSaved))
		assertScrollbackHolds(t, dir, ySaved.file(), savedBytes(ySaved))
	}
	for _, order := range orders {
		t.Run(order.name+", the cycle commits", func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, xSaved, ySaved)
			dump := writesThen(t, order.writes, true, func() (bool, error) {
				assertPositionalUntouched(t, dir)
				return true, nil
			})

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{yLive, xLive}}, dir, seed, state.HashMap{}, dump)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			committed := onDiskIndex(t, dir)
			assertWrittenUnderToken(t, dir, committed, xLive, "x-capture")
			assertWrittenUnderToken(t, dir, committed, yLive, "y-capture")
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
		for _, end := range uncommittedEnds {
			t.Run(order.name+", "+end.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedMoved(t, dir, xSaved, ySaved)
				dump := writesThen(t, order.writes, true, func() (bool, error) { return end.end(t, dir) })

				if _, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{yLive, xLive}}, dir, seed, state.HashMap{}, dump)); err == nil {
					t.Fatal("cycle returned nil, want it to end uncommitted")
				}
				if err := os.Chmod(dir, 0o700); err != nil {
					t.Fatalf("restore state dir mode: %v", err)
				}

				committed := onDiskIndex(t, dir)
				for _, p := range []movedPane{xSaved, ySaved} {
					if got := recordAt(t, committed, p).ScrollbackFile; got != p.stored() {
						t.Errorf("sessions.json names %q for %s, want %q", got, p.key(), p.stored())
					}
				}
				assertPositionalUntouched(t, dir)
				assertSavedScrollbackPresent(t, dir)
			})
		}
	}
}

// noUsableTokenOccupants are the new panes at an address a tokened pane
// vacated that have no token the pane-token rule accepts.
var noUsableTokenOccupants = []struct {
	name string
	pane movedPane
}{
	{"a new pane with no token", movedPane{"work", 2, 0, ""}},
	{"a new pane whose token the rule refuses", movedPane{"work", 2, 0, "not-a-token"}},
}

func TestRunCommitCycleWritesADeferredPaneOnceItsCommitHasLanded(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	const staleHash = uint64(0xfeed)
	for _, occ := range noUsableTokenOccupants {
		t.Run(occ.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, saved)
			hm := state.HashMap{occ.pane.key(): staleHash}
			client := func() *movedClient { return &movedClient{live: []movedPane{live, occ.pane}} }
			dump := writesThen(t, []paneWrite{{occ.pane, "new-capture"}}, false, func() (bool, error) {
				if got := hm[occ.pane.key()]; got != staleHash {
					t.Errorf("dedup entry of the deferred pane = %x, want %x left as it was", got, staleHash)
				}
				assertScrollbackHolds(t, dir, saved.file(), savedBytes(saved))
				return false, nil
			})

			capture, err := state.RunCommitCycle(movedTick(client(), dir, seed, hm, dump))
			if err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			committed := onDiskIndex(t, dir)
			assertWrittenAt(t, dir, committed, occ.pane, "new-capture")
			if got, want := hm[occ.pane.key()], xxhash.Sum64String("new-capture"); got != want {
				t.Errorf("dedup entry of the deferred pane = %x, want %x describing its written capture", got, want)
			}
			assertHeldOnTranscript(t, dir, committed, saved, live)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)

			next := writesThen(t, []paneWrite{{occ.pane, "new-capture"}}, false, commits)
			if _, err := state.RunCommitCycle(movedTick(client(), dir, capture.Index, hm, next)); err != nil {
				t.Fatalf("next cycle: %v", err)
			}
			committed = onDiskIndex(t, dir)
			assertWrittenAt(t, dir, committed, occ.pane, "new-capture")
			assertHeldOnTranscript(t, dir, committed, saved, live)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleEndingUncommittedWritesNoDeferredPane(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	for _, occ := range noUsableTokenOccupants {
		for _, end := range uncommittedEnds {
			t.Run(occ.name+", "+end.name, func(t *testing.T) {
				dir := t.TempDir()
				seed := seedMoved(t, dir, saved)
				before := sessionsJSONBytes(t, dir)
				dump := writesThen(t, []paneWrite{{occ.pane, "new-capture"}}, false, func() (bool, error) { return end.end(t, dir) })

				if _, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{live, occ.pane}}, dir, seed, state.HashMap{}, dump)); err == nil {
					t.Fatal("cycle returned nil, want it to end uncommitted")
				}
				if err := os.Chmod(dir, 0o700); err != nil {
					t.Fatalf("restore state dir mode: %v", err)
				}

				if !bytes.Equal(sessionsJSONBytes(t, dir), before) {
					t.Errorf("sessions.json changed by a cycle that ended uncommitted")
				}
				assertScrollbackHolds(t, dir, saved.file(), savedBytes(saved))
				assertSavedScrollbackPresent(t, dir)
			})
		}
	}
}

// The moved-tokenless layout: restore closed the gap at window 1, so tokenless
// A, saved at work:3.0, is live at work:2.0 — the address tokened B's saved
// record names — and B, saved at work:2.0, is live at work:1.0.
var (
	tokenlessASaved = movedPane{"work", 3, 0, ""}
	tokenlessALive  = movedPane{"work", 2, 0, ""}
	tokenedBSaved   = movedPane{"work", 2, 0, otherMovedToken}
	tokenedBLive    = movedPane{"work", 1, 0, otherMovedToken}
)

func TestRunCommitCycleWritesAMovedTokenlessPaneAtAContestedNameOnceItsCommitHasLanded(t *testing.T) {
	bWrites := []struct {
		name   string
		writes []paneWrite
	}{
		{"B is not written", nil},
		{"B is written", []paneWrite{{tokenedBLive, "b-capture"}}},
	}
	for _, b := range bWrites {
		t.Run(b.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, tokenlessASaved, tokenedBSaved)
			results := map[string]error{}
			writes := append(append([]paneWrite{}, b.writes...), paneWrite{tokenlessALive, "a-capture"})
			dump := func(c state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				changed, err := writesPanes(writes, results)(c, w)
				assertScrollbackHolds(t, dir, tokenedBSaved.file(), savedBytes(tokenedBSaved))
				return changed, err
			}

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{tokenedBLive, tokenlessALive}}, dir, seed, state.HashMap{}, dump)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[tokenlessALive.key()]; !errors.Is(err, errNotWritten) {
				t.Errorf("Write(A) = %v; want it deferred", err)
			}
			committed := onDiskIndex(t, dir)
			assertWrittenAt(t, dir, committed, tokenlessALive, "a-capture")
			if !scrollbackAbsent(t, dir, tokenlessASaved.file()) {
				t.Errorf("%s still on disk once a commit naming A's capture ran its housekeeping", tokenlessASaved.file())
			}
			if b.writes != nil {
				assertWrittenAt(t, dir, committed, tokenedBLive, "b-capture")
			} else {
				assertHeldOnTranscript(t, dir, committed, tokenedBSaved, tokenedBLive)
			}
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleRefusesADeferredPanesUnconfirmedEmptyCapture(t *testing.T) {
	for _, refusal := range emptyCaptureRefusals {
		t.Run("the read after the empty capture "+refusal.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, tokenlessASaved, tokenedBSaved)
			client := &movedClient{live: []movedPane{tokenedBLive, tokenlessALive}, later: refusal.later}
			results := map[string]error{}

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPanes([]paneWrite{{tokenlessALive, ""}}, results))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[tokenlessALive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write(A) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", err)
			}
			if client.laterReads != 1 {
				t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
			}
			assertScrollbackHolds(t, dir, tokenlessALive.file(), savedBytes(tokenedBSaved))
			assertHeldOnTranscript(t, dir, onDiskIndex(t, dir), tokenedBSaved, tokenedBLive)
			assertSavedScrollbackPresent(t, dir)
		})
	}
}

// stageUnlinkedContest runs one committing cycle over the moved-tokenless
// layout with B skeleton-marked and its link onto its token refused, so the
// index it commits names work__2.0.bin on both B's record and A's. The
// scrollback directory is left denied; dumpAllowingWrites lifts it.
func stageUnlinkedContest(t *testing.T, dir string) (*movedClient, state.CaptureCycle) {
	t.Helper()
	seed := seedMoved(t, dir, tokenlessASaved, tokenedBSaved)
	client := &movedClient{live: []movedPane{tokenedBLive, tokenlessALive}, skeleton: map[string]bool{tokenedBLive.key(): true}}
	denyScrollbackWrites(t, dir)
	capture, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, dumpsNothing))
	if err != nil {
		t.Fatalf("staging cycle: %v", err)
	}
	committed := onDiskIndex(t, dir)
	for _, p := range []movedPane{tokenedBLive, tokenlessALive} {
		if got := recordAt(t, committed, p).ScrollbackFile; got != tokenlessALive.stored() {
			t.Fatalf("staged sessions.json names %q for %s, want %q", got, p.key(), tokenlessALive.stored())
		}
	}
	return client, capture
}

// dumpAllowingWrites lifts the denial on the scrollback directory once the
// capture's links have been refused, then writes A's capture, which the writer
// defers.
func dumpAllowingWrites(t *testing.T, dir string) dumpFunc {
	return func(c state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
		if err := os.Chmod(state.ScrollbackDir(dir), 0o700); err != nil {
			t.Fatalf("allow scrollback writes: %v", err)
		}
		return writesThen(t, []paneWrite{{tokenlessALive, "a-capture"}}, false, func() (bool, error) { return false, nil })(c, w)
	}
}

func TestRunCommitCycleLeavesADeferredPaneUnwrittenWhileATokenedRecordStillNamesItsName(t *testing.T) {
	dir := t.TempDir()
	seed := seedMoved(t, dir, tokenlessASaved, tokenedBSaved)
	client := &movedClient{live: []movedPane{tokenedBLive, tokenlessALive}, skeleton: map[string]bool{tokenedBLive.key(): true}}
	denyScrollbackWrites(t, dir)
	before := sessionsJSONBytes(t, dir)

	if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, dumpAllowingWrites(t, dir))); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	if bytes.Equal(sessionsJSONBytes(t, dir), before) {
		t.Fatal("sessions.json unchanged, want the cycle to have committed")
	}
	committed := onDiskIndex(t, dir)
	if got := recordAt(t, committed, tokenedBLive).ScrollbackFile; got != tokenlessALive.stored() {
		t.Fatalf("sessions.json names %q for B, want %q its failed link left it on", got, tokenlessALive.stored())
	}
	assertScrollbackHolds(t, dir, tokenlessALive.file(), savedBytes(tokenedBSaved))
	assertSavedScrollbackPresent(t, dir)
}

func TestRunCommitCycleSkippingItsSessionsJSONWriteWritesNoDeferredPane(t *testing.T) {
	dir := t.TempDir()
	client, capture := stageUnlinkedContest(t, dir)
	before := sessionsJSONBytes(t, dir)

	if _, err := state.RunCommitCycle(movedTick(client, dir, capture.Index, state.HashMap{}, dumpAllowingWrites(t, dir))); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	if !bytes.Equal(sessionsJSONBytes(t, dir), before) {
		t.Fatal("sessions.json changed, want the cycle to have skipped its write")
	}
	assertScrollbackHolds(t, dir, tokenlessALive.file(), savedBytes(tokenedBSaved))
	assertSavedScrollbackPresent(t, dir)
}

func TestRunCommitCycleWritesANewTokenedPaneAtAContestedNameUnderItsToken(t *testing.T) {
	saved := movedPane{"work", 2, 0, waitingPaneToken}
	live := movedPane{"work", 1, 0, waitingPaneToken}
	newPane := movedPane{"work", 2, 0, otherMovedToken}
	dir := t.TempDir()
	seed := seedMoved(t, dir, saved)
	dump := writesThen(t, []paneWrite{{newPane, "new-capture"}}, true, func() (bool, error) {
		assertScrollbackHolds(t, dir, saved.file(), savedBytes(saved))
		return true, nil
	})

	if _, err := state.RunCommitCycle(movedTick(&movedClient{live: []movedPane{live, newPane}}, dir, seed, state.HashMap{}, dump)); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	committed := onDiskIndex(t, dir)
	assertWrittenUnderToken(t, dir, committed, newPane, "new-capture")
	assertHeldOnTranscript(t, dir, committed, saved, live)
	assertSavedScrollbackPresent(t, dir)
	assertNoFileOnTwoRecords(t, committed)
}

func TestRunCommitCycleWritesAtThePositionalNameWhenNothingContestsIt(t *testing.T) {
	cases := []struct {
		name  string
		saved []movedPane
		live  []movedPane
		write movedPane
		stage func(t *testing.T, dir string)
	}{
		{
			name:  "sessions.json is absent, whatever the fallback index names",
			saved: []movedPane{contestedXSaved, contestedYSaved},
			live:  contestedLive(),
			write: contestedYLive,
			stage: func(t *testing.T, dir string) {
				if err := os.Remove(state.SessionsJSON(dir)); err != nil {
					t.Fatalf("remove sessions.json: %v", err)
				}
			},
		},
		{
			name:  "sessions.json is unreadable, whatever the fallback index names",
			saved: []movedPane{contestedXSaved, contestedYSaved},
			live:  contestedLive(),
			write: contestedYLive,
			stage: func(t *testing.T, dir string) {
				if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not json"), 0o600); err != nil {
					t.Fatalf("corrupt sessions.json: %v", err)
				}
			},
		},
		{
			name:  "the committed record naming it carries no token",
			saved: []movedPane{{"work", 2, 0, ""}, contestedYSaved},
			live:  []movedPane{{"work", 1, 0, ""}, contestedYLive},
			write: contestedYLive,
		},
		{
			name:  "the committed record naming it carries the pane's own token",
			saved: []movedPane{contestedYLive},
			live:  []movedPane{contestedYLive},
			write: contestedYLive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, tc.saved...)
			if tc.stage != nil {
				tc.stage(t, dir)
			}
			dump := writesThen(t, []paneWrite{{tc.write, "capture"}}, true, commits)

			if _, err := state.RunCommitCycle(movedTick(&movedClient{live: tc.live}, dir, seed, state.HashMap{}, dump)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			committed := onDiskIndex(t, dir)
			assertWrittenAt(t, dir, committed, tc.write, "capture")
			if !scrollbackAbsent(t, dir, tokenFile(tc.write.token)) {
				t.Errorf("token-named transcript of %s on disk, want the write at its positional name", tc.write.key())
			}
		})
	}
}

func TestRunCommitCycleRefusesAContestedPanesUnconfirmedEmptyCaptureAtEitherName(t *testing.T) {
	for _, refusal := range emptyCaptureRefusals {
		t.Run("the read after the empty capture "+refusal.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedMoved(t, dir, contestedXSaved, contestedYSaved)
			client := &movedClient{live: contestedLive(), later: refusal.later}
			results := map[string]error{}
			dump := func(c state.CaptureCycle, w state.ScrollbackWriter) (bool, error) {
				changed, err := writesPanes([]paneWrite{{contestedYLive, ""}}, results)(c, w)
				assertScrollbackHolds(t, dir, contestedXSaved.file(), savedBytes(contestedXSaved))
				assertScrollbackHolds(t, dir, tokenFile(contestedYLive.token), savedBytes(contestedYSaved))
				return changed, err
			}

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, dump)); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[contestedYLive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write(Y) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", err)
			}
			if client.laterReads != 1 {
				t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, contestedYSaved, contestedYLive)
			assertHeldOnTranscript(t, dir, committed, contestedXSaved, contestedXLive)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}
