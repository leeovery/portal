package state_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// A session saved with no tokens at work:0.0, work:2.0 and work:3.0, which
// restore brought back at work:0.0, work:1.0 and work:2.0, giving each pane a
// token tied to the record it was built from.
var (
	restoredUnmovedSaved = movedPane{"work", 0, 0, ""}
	restoredUnmovedLive  = movedPane{"work", 0, 0, "rstr0a"}
	restoredASaved       = movedPane{"work", 2, 0, ""}
	restoredALive        = movedPane{"work", 1, 0, "rstr1b"}
	restoredBSaved       = movedPane{"work", 3, 0, ""}
	restoredBLive        = movedPane{"work", 2, 0, "rstr2c"}
)

type restoredPair struct{ saved, live movedPane }

func restoredRun() []restoredPair {
	return []restoredPair{
		{restoredUnmovedSaved, restoredUnmovedLive},
		{restoredASaved, restoredALive},
		{restoredBSaved, restoredBLive},
	}
}

func restoredRunLive() []movedPane {
	return []movedPane{restoredUnmovedLive, restoredALive, restoredBLive}
}

// seedRestoredRun commits the tokenless saved index, then records the tokens
// restore gave its panes, returning sessions.json as restore left it.
func seedRestoredRun(t *testing.T, dir string) state.Index {
	t.Helper()
	ties := make([]state.RestoredPaneToken, 0, len(restoredRun()))
	saved := make([]movedPane, 0, len(restoredRun()))
	for _, p := range restoredRun() {
		saved = append(saved, p.saved)
		ties = append(ties, tieOf(p.saved, p.live.token))
	}
	seedMoved(t, dir, saved...)
	if err := state.RecordRestoredPaneTokens(dir, ties); err != nil {
		t.Fatalf("RecordRestoredPaneTokens: %v", err)
	}
	return onDiskIndex(t, dir)
}

// skeletonClient lists every live pane as still carrying its skeleton marker.
type skeletonClient struct{ *movedClient }

func (c skeletonClient) ShowAllServerOptions() (string, error) {
	lines := make([]string, 0, len(c.live))
	for _, p := range c.live {
		lines = append(lines, state.SkeletonMarkerPrefix+p.key()+" 1")
	}
	return strings.Join(lines, "\n"), nil
}

// assertRestoredRecordsHoldSavedBytes checks every restored pane's committed
// record carries the token restore gave it and names a file on disk holding
// the bytes that pane was saved with.
func assertRestoredRecordsHoldSavedBytes(t *testing.T, dir string) {
	t.Helper()
	committed := onDiskIndex(t, dir)
	for _, p := range restoredRun() {
		rec := recordAt(t, committed, p.live)
		if rec.PortalPaneID != p.live.token {
			t.Errorf("record at %s carries token %q, want %q", p.live.key(), rec.PortalPaneID, p.live.token)
		}
		name := strings.TrimPrefix(rec.ScrollbackFile, "scrollback/")
		if got := readScrollback(t, dir, name); got != savedBytes(p.saved) {
			t.Errorf("record at %s names %q holding %q, want the bytes saved at %s", p.live.key(), rec.ScrollbackFile, got, p.saved.key())
		}
	}
	assertSavedScrollbackPresent(t, dir)
	assertNoFileOnTwoRecords(t, committed)
}

func TestRunCommitCycleFirstCommitAfterRestoreKeepsEveryRestoredPanesSavedBytes(t *testing.T) {
	cases := []struct {
		name   string
		client func(*movedClient) state.CaptureCycleClient
	}{
		{"while every pane still carries its skeleton marker", func(c *movedClient) state.CaptureCycleClient { return skeletonClient{c} }},
		{"after hydration has cleared every skeleton marker", func(c *movedClient) state.CaptureCycleClient { return c }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedRestoredRun(t, dir)
			cycle := movedTick(&movedClient{live: restoredRunLive()}, dir, seed, state.HashMap{}, dumpsNothing)
			cycle.Client = tc.client(&movedClient{live: restoredRunLive()})

			if _, err := state.RunCommitCycle(cycle); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			assertRestoredRecordsHoldSavedBytes(t, dir)
		})
	}
}

func TestRunCommitCycleKeepsARestoredPanesTranscriptOverAnUnconfirmedEmptyCapture(t *testing.T) {
	for _, refusal := range emptyCaptureRefusals {
		t.Run("the read after the empty capture "+refusal.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedRestoredRun(t, dir)
			client := &movedClient{live: restoredRunLive(), later: refusal.later}
			results := map[string]error{}

			if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPanes([]paneWrite{{restoredALive, ""}}, results))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[restoredALive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
				t.Errorf("Write(%s) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", restoredALive.key(), err)
			}
			if client.laterReads != 1 {
				t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
			}
			if !scrollbackAbsent(t, dir, restoredALive.file()) {
				t.Errorf("%s written at the restored pane's live positional path", restoredALive.file())
			}
			committed := onDiskIndex(t, dir)
			assertHeldOnTranscript(t, dir, committed, restoredASaved, restoredALive)
			assertRestoredRecordsHoldSavedBytes(t, dir)
		})
	}
}

func TestRunCommitCycleKeepsNamingARestoredPanesTranscriptUntilItsCaptureIsWritten(t *testing.T) {
	dir := t.TempDir()
	seed := seedRestoredRun(t, dir)
	live := restoredRunLive()
	hm := state.HashMap{}

	capture, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, seed, hm, dumpsNothing))
	if err != nil {
		t.Fatalf("tick with every capture-pane refused: %v", err)
	}
	assertRestoredRecordsHoldSavedBytes(t, dir)

	if _, err := state.RunCommitCycle(movedCommitNow(t, &movedClient{live: live}, dir)); err != nil {
		t.Fatalf("commit-now: %v", err)
	}
	assertRestoredRecordsHoldSavedBytes(t, dir)

	if _, err := state.RunCommitCycle(movedTick(&movedClient{live: live}, dir, capture.Index, nil, nil)); err != nil {
		t.Fatalf("shutdown flush with nothing dumped: %v", err)
	}
	assertRestoredRecordsHoldSavedBytes(t, dir)
	committed := onDiskIndex(t, dir)
	assertHeldOnTranscript(t, dir, committed, restoredASaved, restoredALive)
	assertHeldOnTranscript(t, dir, committed, restoredBSaved, restoredBLive)
}

func TestRunCommitCycleFilesARestoredPaneAtItsLivePositionalFileOnceItsCaptureIsWritten(t *testing.T) {
	captures := []struct {
		name       string
		data       string
		laterReads int
	}{
		{"a non-empty capture", movedCapture, 0},
		{"an empty capture its own server confirms against the saved transcript", "", 1},
	}
	for _, tc := range captures {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seed := seedRestoredRun(t, dir)
			hm := state.HashMap{}
			capture, err := state.RunCommitCycle(movedTick(&movedClient{live: restoredRunLive()}, dir, seed, hm, dumpsNothing))
			if err != nil {
				t.Fatalf("first commit after hydration: %v", err)
			}
			client := &movedClient{live: restoredRunLive()}
			results := map[string]error{}

			if _, err := state.RunCommitCycle(movedTick(client, dir, capture.Index, hm, writesPanes([]paneWrite{{restoredALive, tc.data}}, results))); err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}

			if err := results[restoredALive.key()]; err != nil {
				t.Errorf("Write(%s) = %v; want a write", restoredALive.key(), err)
			}
			if client.laterReads != tc.laterReads {
				t.Errorf("confirmation reads after the capture = %d, want %d", client.laterReads, tc.laterReads)
			}
			committed := onDiskIndex(t, dir)
			assertWrittenAt(t, dir, committed, restoredALive, tc.data)
			if !scrollbackAbsent(t, dir, tokenFile(restoredALive.token)) {
				t.Errorf("token-named transcript still on disk once a commit naming the live capture ran its housekeeping")
			}
			assertHeldOnTranscript(t, dir, committed, restoredBSaved, restoredBLive)
			assertSavedScrollbackPresent(t, dir)
			assertNoFileOnTwoRecords(t, committed)
		})
	}
}

func TestRunCommitCycleJudgesARestoredPaneAtItsOwnSavedAddressAgainstItsPositionalFile(t *testing.T) {
	dir := t.TempDir()
	seed := seedRestoredRun(t, dir)
	client := &movedClient{live: restoredRunLive(), later: func() (int, error) { return ownServerPID + 1, nil }}
	results := map[string]error{}

	if _, err := state.RunCommitCycle(movedTick(client, dir, seed, state.HashMap{}, writesPanes([]paneWrite{{restoredUnmovedLive, ""}}, results))); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	if err := results[restoredUnmovedLive.key()]; !errors.Is(err, state.ErrUnconfirmedEmptyCapture) {
		t.Errorf("Write(%s) = %v; want a refusal wrapping ErrUnconfirmedEmptyCapture", restoredUnmovedLive.key(), err)
	}
	if client.laterReads != 1 {
		t.Errorf("confirmation reads after the capture = %d, want 1", client.laterReads)
	}
	committed := onDiskIndex(t, dir)
	if got := recordAt(t, committed, restoredUnmovedLive).ScrollbackFile; got != restoredUnmovedLive.stored() {
		t.Errorf("sessions.json names %q, want its own positional file %q", got, restoredUnmovedLive.stored())
	}
	if got := readScrollback(t, dir, restoredUnmovedLive.file()); got != savedBytes(restoredUnmovedSaved) {
		t.Errorf("%s = %q, want its saved bytes", restoredUnmovedLive.file(), got)
	}
	if !scrollbackAbsent(t, dir, tokenFile(restoredUnmovedLive.token)) {
		t.Errorf("a token-named transcript was filed for the pane at its own saved address")
	}
}
