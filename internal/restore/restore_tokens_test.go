package restore_test

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// shiftedSavedSession was saved at windows 0, 2 and 3; restore brings its
// panes back at windows 0, 1 and 2. Only the middle pane was saved with a token.
func shiftedSavedSession() state.Session {
	return newSession("work", nil,
		newWindow(0, "main", newPane(0, "/work", "scrollback/work__0.0.bin")),
		newWindow(2, "mid", newPaneWithToken(0, "/work", "scrollback/work__2.0.bin", "tokA00")),
		newWindow(3, "last", newPane(0, "/work", "scrollback/work__3.0.bin")),
	)
}

const shiftedLivePanes = "0:0\n1:0\n2:0"

// stampedTokens maps each pane target restore stamped to the token it stamped.
func stampedTokens(calls [][]string) map[string]string {
	out := map[string]string{}
	for _, c := range setPaneOptionCalls(calls) {
		out[c[3]] = c[5]
	}
	return out
}

func savedPaneAt(t *testing.T, idx state.Index, session string, window, pane int) state.Pane {
	t.Helper()
	for _, s := range idx.Sessions {
		if s.Name != session {
			continue
		}
		for _, w := range s.Windows {
			if w.Index != window {
				continue
			}
			for _, p := range w.Panes {
				if p.Index == pane {
					return p
				}
			}
		}
	}
	t.Fatalf("sessions.json holds no pane at %s:%d.%d", session, window, pane)
	return state.Pane{}
}

func readSavedIndex(t *testing.T, dir string) state.Index {
	t.Helper()
	idx, skip, err := state.ReadIndex(dir)
	if err != nil || skip {
		t.Fatalf("ReadIndex = (skip %v, err %v)", skip, err)
	}
	return idx
}

func TestOrchestrator_RecordsEachMintedTokenOnTheSavedRecordItsPaneWasBuiltFrom(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, []state.Session{shiftedSavedSession()})
	mock := commandertest.FromFunc((&orchestratorRunFunc{listPanesOut: shiftedLivePanes}).run)
	logger, sink := logtest.NewCaptureLogger(t)

	if _, err := newOrchestrator(t, mock, dir, logger).Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	stamped := stampedTokens(mock.Calls())
	if len(stamped) != 3 {
		t.Fatalf("stamps = %v, want one per restored pane", stamped)
	}
	if got := stamped["=work:1.0"]; got != "tokA00" {
		t.Errorf("pane restored from the saved token's record stamped %q, want %q", got, "tokA00")
	}
	if stamped["=work:0.0"] == stamped["=work:2.0"] {
		t.Errorf("both untokened panes stamped %q, want a token each", stamped["=work:0.0"])
	}
	saved := readSavedIndex(t, dir)
	for _, tc := range []struct {
		savedWindow int
		liveTarget  string
		file        string
	}{
		{0, "=work:0.0", "scrollback/work__0.0.bin"},
		{2, "=work:1.0", "scrollback/work__2.0.bin"},
		{3, "=work:2.0", "scrollback/work__3.0.bin"},
	} {
		rec := savedPaneAt(t, saved, "work", tc.savedWindow, 0)
		if rec.PortalPaneID != stamped[tc.liveTarget] {
			t.Errorf("saved record at work:%d.0 carries %q, want the token stamped on %s (%q)", tc.savedWindow, rec.PortalPaneID, tc.liveTarget, stamped[tc.liveTarget])
		}
		if rec.ScrollbackFile != tc.file {
			t.Errorf("saved record at work:%d.0 names %q, want %q", tc.savedWindow, rec.ScrollbackFile, tc.file)
		}
	}
	for _, rec := range sink.Records() {
		if rec.Level >= slog.LevelWarn {
			t.Errorf("unexpected %s %q on a clean restore", rec.Level, rec.Msg)
		}
	}
}

func TestOrchestrator_RecordsNoTokenForAPaneWhoseStampFailed(t *testing.T) {
	dir := t.TempDir()
	writeValidIndex(t, dir, []state.Session{shiftedSavedSession()})
	mock := commandertest.FromFunc(failOnPaneOptionTarget(shiftedLivePanes, "=work:2.0"))
	logger, _ := logtest.NewCaptureLogger(t)

	if _, err := newOrchestrator(t, mock, dir, logger).Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	saved := readSavedIndex(t, dir)
	if got := savedPaneAt(t, saved, "work", 3, 0).PortalPaneID; got != "" {
		t.Errorf("saved record of the pane whose stamp failed carries %q, want none", got)
	}
	if got, want := savedPaneAt(t, saved, "work", 0, 0).PortalPaneID, stampedTokens(mock.Calls())["=work:0.0"]; got == "" || got != want {
		t.Errorf("saved record at work:0.0 carries %q, want the token stamped on its pane %q", got, want)
	}
}

func TestOrchestrator_WarnsAndLeavesSessionsJSONWhenTheTokensCannotBeRecorded(t *testing.T) {
	state.SetCommitLockTimeoutForTest(t, 40*time.Millisecond)
	dir := t.TempDir()
	writeValidIndex(t, dir, []state.Session{shiftedSavedSession()})
	holdCommitLock(t, dir)
	before, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	mock := commandertest.FromFunc((&orchestratorRunFunc{listPanesOut: shiftedLivePanes}).run)
	logger, sink := logtest.NewCaptureLogger(t)

	corrupt, err := newOrchestrator(t, mock, dir, logger).Restore()

	if corrupt || err != nil {
		t.Fatalf("Restore = (%t, %v), want (false, nil)", corrupt, err)
	}
	if got := len(findAllCalls(mock.Calls(), "respawn-pane")); got != 3 {
		t.Errorf("respawn-pane calls = %d, want every pane armed", got)
	}
	after, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("sessions.json changed under a held commit lock")
	}
	warn := sink.Records().WithMessage("record restored pane tokens failed").Only(t, "record failure warning")
	if warn.Level != slog.LevelWarn {
		t.Errorf("level = %v, want WARN", warn.Level)
	}
	if got := warn.ErrorAttr(t, "error"); !errors.Is(got, state.ErrCommitLockHeld) {
		t.Errorf("error attr = %v, want ErrCommitLockHeld", got)
	}
}

func holdCommitLock(t *testing.T, dir string) {
	t.Helper()
	f, err := os.OpenFile(state.CommitLock(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock commit lock: %v", err)
	}
}
