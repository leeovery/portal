package state_test

import (
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const droppedSessionMsg = "session dropped"

func namedSessionsIndex(names ...string) state.Index {
	sessions := make([]state.Session, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, state.Session{
			Name: name,
			Windows: []state.Window{{
				Index: 0, Name: "main", Active: true,
				Panes: []state.Pane{{Index: 0, CWD: "/tmp", Active: true, CurrentCommand: "zsh"}},
			}},
		})
	}
	return state.Index{Version: state.SchemaVersion, Sessions: sessions}
}

func seedSessionsJSON(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := state.Commit(dir, namedSessionsIndex(names...), false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

func readIndexFile(t *testing.T, dir string) state.Index {
	t.Helper()
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	idx, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("decode sessions.json: %v", err)
	}
	return idx
}

// Read-only, so the atomic write cannot create its temp file.
func denyStateDirWrites(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("cannot make a directory unwritable as root")
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod 0500: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func droppedSessionNames(t *testing.T, sink *logtest.Sink) []string {
	t.Helper()
	names := []string{}
	for _, rec := range sink.Records().WithMessage(droppedSessionMsg) {
		if rec.Level != slog.LevelInfo {
			t.Errorf("drop line level = %v, want INFO", rec.Level)
		}
		names = append(names, rec.AttrString(t, "session"))
	}
	slices.Sort(names)
	return names
}

func TestCommitLogsEachDroppedSessionByName(t *testing.T) {
	dir := t.TempDir()
	seedSessionsJSON(t, dir, "alpha", "bravo", "charlie", "delta")
	logger, sink := logtest.NewCaptureLogger(t)

	if err := state.Commit(dir, namedSessionsIndex("alpha", "charlie"), false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got, want := droppedSessionNames(t, sink), []string{"bravo", "delta"}; !slices.Equal(got, want) {
		t.Errorf("dropped sessions logged = %v, want %v", got, want)
	}
}

func TestCommitLogsNoDropWhenEverySavedSessionIsKept(t *testing.T) {
	cases := map[string][]string{
		"the same sessions":           {"alpha", "bravo"},
		"new sessions beside them":    {"alpha", "bravo", "charlie"},
		"the same sessions reordered": {"bravo", "alpha"},
	}
	for name, kept := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			seedSessionsJSON(t, dir, "alpha", "bravo")
			logger, sink := logtest.NewCaptureLogger(t)

			if err := state.Commit(dir, namedSessionsIndex(kept...), true, logger); err != nil {
				t.Fatalf("Commit: %v", err)
			}

			if got := droppedSessionNames(t, sink); len(got) != 0 {
				t.Errorf("dropped sessions logged = %v, want none", got)
			}
		})
	}
}

func TestCommitLogsTheLastSessionDroppedByAnEmptyIndex(t *testing.T) {
	dir := t.TempDir()
	seedSessionsJSON(t, dir, "last")
	logger, sink := logtest.NewCaptureLogger(t)

	if err := state.Commit(dir, namedSessionsIndex(), false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got, want := droppedSessionNames(t, sink), []string{"last"}; !slices.Equal(got, want) {
		t.Errorf("dropped sessions logged = %v, want %v", got, want)
	}
	if got := len(readIndexFile(t, dir).Sessions); got != 0 {
		t.Errorf("committed sessions = %d, want 0", got)
	}
}

func TestCommitLogsNoDropOnItsFirstWrite(t *testing.T) {
	dir := t.TempDir()
	logger, sink := logtest.NewCaptureLogger(t)

	if err := state.Commit(dir, namedSessionsIndex("alpha"), false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got := droppedSessionNames(t, sink); len(got) != 0 {
		t.Errorf("dropped sessions logged = %v, want none", got)
	}
}

func TestCommitLogsNoDropWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	seedSessionsJSON(t, dir, "alpha", "bravo")
	denyStateDirWrites(t, dir)
	logger, sink := logtest.NewCaptureLogger(t)

	if err := state.Commit(dir, namedSessionsIndex("alpha"), false, logger); err == nil {
		t.Fatal("Commit: want an error from a denied write")
	}

	if got := droppedSessionNames(t, sink); len(got) != 0 {
		t.Errorf("dropped sessions logged = %v, want none", got)
	}
}
