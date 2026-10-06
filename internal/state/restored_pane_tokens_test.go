package state_test

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

func tieOf(saved movedPane, token string) state.RestoredPaneToken {
	return state.RestoredPaneToken{Session: saved.session, Window: saved.window, Pane: saved.pane, Token: token}
}

func TestRecordRestoredPaneTokens(t *testing.T) {
	t.Run("it writes each token onto the record at its saved address and leaves every other record as it was", func(t *testing.T) {
		dir := t.TempDir()
		first := movedPane{"work", 0, 0, ""}
		second := movedPane{"work", 2, 0, ""}
		untied := movedPane{"logs", 1, 0, ""}
		seedMoved(t, dir, first, second, untied)

		if err := state.RecordRestoredPaneTokens(dir, []state.RestoredPaneToken{tieOf(first, "rstr0a"), tieOf(second, "rstr1b")}); err != nil {
			t.Fatalf("RecordRestoredPaneTokens: %v", err)
		}

		idx := onDiskIndex(t, dir)
		for _, tc := range []struct {
			pane  movedPane
			token string
		}{{first, "rstr0a"}, {second, "rstr1b"}, {untied, ""}} {
			rec := recordAt(t, idx, tc.pane)
			if rec.PortalPaneID != tc.token {
				t.Errorf("record at %s carries token %q, want %q", tc.pane.key(), rec.PortalPaneID, tc.token)
			}
			if rec.ScrollbackFile != tc.pane.stored() {
				t.Errorf("record at %s names %q, want %q", tc.pane.key(), rec.ScrollbackFile, tc.pane.stored())
			}
			if got := readScrollback(t, dir, tc.pane.file()); got != savedBytes(tc.pane) {
				t.Errorf("%s = %q, want its saved bytes", tc.pane.file(), got)
			}
		}
	})

	t.Run("it keeps a token a record already carries", func(t *testing.T) {
		dir := t.TempDir()
		saved := movedPane{"work", 0, 0, waitingPaneToken}
		seedMoved(t, dir, saved)

		if err := state.RecordRestoredPaneTokens(dir, []state.RestoredPaneToken{tieOf(saved, "rstr0a")}); err != nil {
			t.Fatalf("RecordRestoredPaneTokens: %v", err)
		}

		if got := recordAt(t, onDiskIndex(t, dir), saved).PortalPaneID; got != waitingPaneToken {
			t.Errorf("record carries token %q, want its own %q", got, waitingPaneToken)
		}
	})

	t.Run("it writes nothing when no tie names a record", func(t *testing.T) {
		dir := t.TempDir()
		seedMoved(t, dir, movedPane{"work", 0, 0, ""})
		before := sessionsJSONBytes(t, dir)
		info, err := os.Stat(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("stat sessions.json: %v", err)
		}

		if err := state.RecordRestoredPaneTokens(dir, []state.RestoredPaneToken{tieOf(movedPane{"work", 5, 0, ""}, "rstr0a"), tieOf(movedPane{"gone", 0, 0, ""}, "rstr1b")}); err != nil {
			t.Fatalf("RecordRestoredPaneTokens: %v", err)
		}

		if after := sessionsJSONBytes(t, dir); !bytes.Equal(before, after) {
			t.Errorf("sessions.json changed:\nbefore %s\nafter  %s", before, after)
		}
		after, err := os.Stat(state.SessionsJSON(dir))
		if err != nil {
			t.Fatalf("stat sessions.json: %v", err)
		}
		if !os.SameFile(info, after) {
			t.Error("sessions.json was rewritten with nothing to record")
		}
	})

	t.Run("it returns an error and creates nothing when sessions.json is absent", func(t *testing.T) {
		dir := t.TempDir()

		err := state.RecordRestoredPaneTokens(dir, []state.RestoredPaneToken{tieOf(movedPane{"work", 0, 0, ""}, "rstr0a")})

		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error = %v, want one wrapping os.ErrNotExist", err)
		}
		if _, statErr := os.Stat(state.SessionsJSON(dir)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("sessions.json stat = %v, want it still absent", statErr)
		}
	})

	t.Run("it gives up after the commit lock's bound with sessions.json untouched", func(t *testing.T) {
		state.SetCommitLockTimeoutForTest(t, 40*time.Millisecond)
		dir := t.TempDir()
		seedMoved(t, dir, movedPane{"work", 0, 0, ""})
		holdCommitLock(t, dir)
		before := sessionsJSONBytes(t, dir)

		err := state.RecordRestoredPaneTokens(dir, []state.RestoredPaneToken{tieOf(movedPane{"work", 0, 0, ""}, "rstr0a")})

		if !errors.Is(err, state.ErrCommitLockHeld) {
			t.Errorf("error = %v, want ErrCommitLockHeld", err)
		}
		if after := sessionsJSONBytes(t, dir); !bytes.Equal(before, after) {
			t.Errorf("sessions.json changed under a held lock:\nbefore %s\nafter  %s", before, after)
		}
	})

	t.Run("it takes no lock and reads nothing for no ties", func(t *testing.T) {
		dir := t.TempDir()

		if err := state.RecordRestoredPaneTokens(dir, nil); err != nil {
			t.Fatalf("RecordRestoredPaneTokens: %v", err)
		}

		if _, err := os.Stat(state.CommitLock(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("commit lock stat = %v, want no lock file created", err)
		}
	})
}

func sessionsJSONBytes(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return data
}
