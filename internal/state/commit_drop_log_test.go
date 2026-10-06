package state_test

import (
	"fmt"
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

// capturedSessionRecord is a session as a capture records it: its window
// layout carries its pane's tmux id, and its pane's scrollback path is spelled
// from the session's name.
func capturedSessionRecord(name string, paneID int) state.Session {
	return state.Session{
		Name:        name,
		Environment: map[string]string{},
		Windows: []state.Window{{
			Index: 0, Name: "zsh", Layout: fmt.Sprintf("b25d,80x24,0,0,%d", paneID), Active: true,
			Panes: []state.Pane{{
				Index: 0, CWD: "/work", Active: true, CurrentCommand: "zsh",
				ScrollbackFile: "scrollback/" + state.SanitizePaneKey(name, 0, 0) + ".bin",
			}},
		}},
	}
}

func indexOf(sessions ...state.Session) state.Index {
	return state.Index{Version: state.SchemaVersion, Sessions: sessions}
}

func seedIndex(t *testing.T, dir string, idx state.Index) {
	t.Helper()
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

func TestCommitLogsNoDropForARenamedSession(t *testing.T) {
	dir := t.TempDir()
	seedIndex(t, dir, indexOf(
		capturedSessionRecord("alpha", 1),
		capturedSessionRecord("bravo", 2),
	))
	logger, sink := logtest.NewCaptureLogger(t)

	renamed := indexOf(
		capturedSessionRecord("alpha", 1),
		capturedSessionRecord("zulu", 2),
	)
	if err := state.Commit(dir, renamed, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got := droppedSessionNames(t, sink); len(got) != 0 {
		t.Errorf("dropped sessions logged = %v, want none", got)
	}
	got := sessionNames(readIndexFile(t, dir))
	slices.Sort(got)
	if !slices.Equal(got, []string{"alpha", "zulu"}) {
		t.Errorf("committed sessions = %v, want [alpha zulu]", got)
	}
}

func TestCommitLogsADropBesideAnUnrelatedNewSession(t *testing.T) {
	dir := t.TempDir()
	seedIndex(t, dir, indexOf(
		capturedSessionRecord("alpha", 1),
		capturedSessionRecord("bravo", 2),
	))
	logger, sink := logtest.NewCaptureLogger(t)

	next := indexOf(
		capturedSessionRecord("alpha", 1),
		capturedSessionRecord("zulu", 3),
	)
	if err := state.Commit(dir, next, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got, want := droppedSessionNames(t, sink), []string{"bravo"}; !slices.Equal(got, want) {
		t.Errorf("dropped sessions logged = %v, want %v", got, want)
	}
}

func TestCommitTakesOneNewSessionAsTheRenameOfOneSavedSessionOnly(t *testing.T) {
	dir := t.TempDir()
	seedIndex(t, dir, indexOf(
		capturedSessionRecord("bravo", 2),
		capturedSessionRecord("charlie", 2),
	))
	logger, sink := logtest.NewCaptureLogger(t)

	if err := state.Commit(dir, indexOf(capturedSessionRecord("zulu", 2)), false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got := droppedSessionNames(t, sink)
	if len(got) != 1 || (got[0] != "bravo" && got[0] != "charlie") {
		t.Errorf("dropped sessions logged = %v, want exactly one of bravo or charlie", got)
	}
}

func TestCommitLogsNoDropForASessionRenamedAfterItsPaneMovedOn(t *testing.T) {
	dir := t.TempDir()
	seedIndex(t, dir, indexOf(capturedSessionRecord("proj-ab12cd", 1)))
	logger, sink := logtest.NewCaptureLogger(t)

	renamed := capturedSessionRecord("editing", 1)
	renamed.Environment = map[string]string{"EDITOR": "nvim"}
	renamed.Windows[0].Name = "nvim"
	renamed.Windows[0].Panes[0].CWD = "/work/proj"
	renamed.Windows[0].Panes[0].CurrentCommand = "nvim"
	if err := state.Commit(dir, indexOf(renamed), false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if got := droppedSessionNames(t, sink); len(got) != 0 {
		t.Errorf("dropped sessions logged = %v, want none", got)
	}
}
