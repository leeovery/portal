package state_test

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const (
	absentIndexWarn     = "sessions.json absent; committing without the saved index"
	unreadableIndexWarn = "read sessions.json failed; committing without the saved index"
)

func stageUndecodableIndex(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed undecodable sessions.json: %v", err)
	}
}

// stageUnreadableIndex leaves sessions.json present as a directory, which every
// read of it fails on.
func stageUnreadableIndex(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(state.SessionsJSON(dir), 0o700); err != nil {
		t.Fatalf("stage unreadable sessions.json: %v", err)
	}
}

func unreadIndexWarnCounts(sink *logtest.Sink) (absent, unreadable int) {
	warns := sink.Records().AtExactLevel(slog.LevelWarn)
	return len(warns.WithMessage(absentIndexWarn)), len(warns.WithMessage(unreadableIndexWarn))
}

func TestRunCommitCycleReportsASessionsJSONItCouldNotRead(t *testing.T) {
	cases := []struct {
		name           string
		stage          func(t *testing.T, dir string)
		wantAbsent     int
		wantUnreadable int
		wantCommit     bool
	}{
		{name: "absent", stage: func(*testing.T, string) {}, wantAbsent: 1, wantCommit: true},
		{name: "undecodable", stage: stageUndecodableIndex, wantUnreadable: 1, wantCommit: true},
		{name: "present but unreadable", stage: stageUnreadableIndex, wantUnreadable: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.stage(t, dir)
			logger, sink := logtest.NewCaptureLogger(t)
			loads := 0
			var loggedBeforeLoad bool
			fallback := waitingIndex(waitingPaneToken, "scrollback/"+handOverPositionalFile())

			_, err := state.RunCommitCycle(state.CommitCycle{
				OwnServer: ownServerPID,
				Client:    &worldClient{world: newHandOverWorld()},
				Dir:       dir,
				LoadPrev: func() *state.Index {
					loads++
					absent, unreadable := unreadIndexWarnCounts(sink)
					loggedBeforeLoad = absent+unreadable == 1
					return &fallback
				},
				Logger: logger,
			})

			absent, unreadable := unreadIndexWarnCounts(sink)
			if absent != tc.wantAbsent || unreadable != tc.wantUnreadable {
				t.Errorf("absent WARNs = %d, unreadable WARNs = %d, want %d and %d in:\n%s", absent, unreadable, tc.wantAbsent, tc.wantUnreadable, sink.Body())
			}
			if loads != 1 {
				t.Errorf("LoadPrev calls = %d, want 1", loads)
			}
			if !loggedBeforeLoad {
				t.Error("LoadPrev was called before the cycle reported the unread sessions.json")
			}
			if tc.wantUnreadable == 1 {
				rec := sink.Records().WithMessage(unreadableIndexWarn).Only(t, "unreadable sessions.json WARN")
				if cause := rec.ErrorAttr(t, "error"); cause == nil {
					t.Error("unreadable sessions.json WARN carries no cause in error")
				}
			}
			if !tc.wantCommit {
				return
			}
			if err != nil {
				t.Fatalf("RunCommitCycle: %v", err)
			}
			if got := sessionNamesOf(onDiskIndex(t, dir)); len(got) != 1 || got[0] != "work" {
				t.Errorf("committed sessions = %v, want [work]", got)
			}
		})
	}
}

func TestRunCommitCycleCarriesTheDecodeFailureAsTheCause(t *testing.T) {
	dir := t.TempDir()
	stageUndecodableIndex(t, dir)
	logger, sink := logtest.NewCaptureLogger(t)

	if _, err := state.RunCommitCycle(state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    &worldClient{world: newHandOverWorld()},
		Dir:       dir,
		LoadPrev:  func() *state.Index { return &state.Index{} },
		Logger:    logger,
	}); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	rec := sink.Records().WithMessage(unreadableIndexWarn).Only(t, "unreadable sessions.json WARN")
	if cause := rec.ErrorAttr(t, "error"); !errors.Is(cause, state.ErrCorruptIndex) {
		t.Errorf("error = %v, want the decode failure wrapping ErrCorruptIndex", cause)
	}
}

func TestRunCommitCycleReportsAnUnreadSessionsJSONBeforeAStandDown(t *testing.T) {
	dir := t.TempDir()
	logger, sink := logtest.NewCaptureLogger(t)
	listErr := errors.New("tmux gone")

	_, err := state.RunCommitCycle(state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    &failFastCaptureClient{t: t, listSessionNamesErr: listErr},
		Dir:       dir,
		LoadPrev:  func() *state.Index { return &state.Index{} },
		Logger:    logger,
	})

	if !errors.Is(err, listErr) {
		t.Fatalf("error = %v, want the failed listing", err)
	}
	if absent, unreadable := unreadIndexWarnCounts(sink); absent != 1 || unreadable != 0 {
		t.Errorf("absent WARNs = %d, unreadable WARNs = %d, want 1 and 0 in:\n%s", absent, unreadable, sink.Body())
	}
}

func TestRunCommitCycleOverAReadableSessionsJSONReportsNothing(t *testing.T) {
	dir := t.TempDir()
	handOverSeed(t, dir)
	logger, sink := logtest.NewCaptureLogger(t)
	loads := 0

	if _, err := state.RunCommitCycle(state.CommitCycle{
		OwnServer: ownServerPID,
		Client:    &worldClient{world: newHandOverWorld()},
		Dir:       dir,
		LoadPrev:  func() *state.Index { loads++; return &state.Index{} },
		Logger:    logger,
	}); err != nil {
		t.Fatalf("RunCommitCycle: %v", err)
	}

	if absent, unreadable := unreadIndexWarnCounts(sink); absent != 0 || unreadable != 0 {
		t.Errorf("absent WARNs = %d, unreadable WARNs = %d, want neither in:\n%s", absent, unreadable, sink.Body())
	}
	if loads != 0 {
		t.Errorf("LoadPrev calls = %d, want 0", loads)
	}
}

func TestCommitOverAnUnreadSessionsJSONReportsNothingAndWrites(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"absent":      func(*testing.T, string) {},
		"undecodable": stageUndecodableIndex,
	}
	for name, stage := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			stage(t, dir)
			logger, sink := logtest.NewCaptureLogger(t)
			idx := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{{Name: "work", Environment: map[string]string{}}}}

			if err := state.Commit(dir, idx, false, logger); err != nil {
				t.Fatalf("Commit: %v", err)
			}

			if absent, unreadable := unreadIndexWarnCounts(sink); absent != 0 || unreadable != 0 {
				t.Errorf("absent WARNs = %d, unreadable WARNs = %d, want neither in:\n%s", absent, unreadable, sink.Body())
			}
			if got := sessionNamesOf(onDiskIndex(t, dir)); len(got) != 1 || got[0] != "work" {
				t.Errorf("committed sessions = %v, want [work]", got)
			}
		})
	}
}
