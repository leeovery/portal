package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tui"
)

var transcriptCreated = time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)

// transcriptFile is the scrollback file an entry of name records for the
// creation the tests use.
func transcriptFile(t *testing.T, name string) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(name, transcriptCreated, 4242)
	if err != nil {
		t.Fatalf("scrollback file for %s: %v", name, err)
	}
	return file
}

// writeTranscript writes content to file, relative to the state dir dir.
func writeTranscript(t *testing.T, dir, file, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make scrollback dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

func TestIndexTranscripts_ReadsTheScrollbackFileTheEntryNames(t *testing.T) {
	file := transcriptFile(t, "api-x7Kd9a")
	dir := seedIndex(t, sessionindex.Entry{Name: "api-x7Kd9a", CreatedAt: transcriptCreated, PID: 4242, ScrollbackFile: file})
	writeTranscript(t, dir, file, "$ make\r\nbuilt\r\n")

	got, err := indexTranscripts{dir: dir}.Transcript("api-x7Kd9a")
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if string(got) != "$ make\r\nbuilt\r\n" {
		t.Errorf("Transcript = %q, want the file the entry names", got)
	}
}

func TestIndexTranscripts_HasNoneToRead(t *testing.T) {
	file := transcriptFile(t, "api-x7Kd9a")
	foreign := transcriptFile(t, "web-Qm2pLs")
	cases := []struct {
		name    string
		entries []sessionindex.Entry
	}{
		{"for a session with no entry", nil},
		{"for an entry naming no file", []sessionindex.Entry{{Name: "api-x7Kd9a", CreatedAt: transcriptCreated, PID: 4242}}},
		{"for a file that isn't there", []sessionindex.Entry{{Name: "api-x7Kd9a", CreatedAt: transcriptCreated, PID: 4242, ScrollbackFile: file}}},
		// Another session's transcript, there to be read, is never read as this one's.
		{"for a file that is another session's", []sessionindex.Entry{{Name: "api-x7Kd9a", CreatedAt: transcriptCreated, PID: 4242, ScrollbackFile: foreign}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := seedIndex(t, tc.entries...)
			writeTranscript(t, dir, foreign, "another session's")

			if got, err := (indexTranscripts{dir: dir}).Transcript("api-x7Kd9a"); err == nil {
				t.Errorf("Transcript = %q, want an error", got)
			}
		})
	}
}

func TestIndexTranscripts_FailsOnAnUnreadableIndex(t *testing.T) {
	dir := seedSessionsJSON(t, "{not json")

	if _, err := (indexTranscripts{dir: dir}).Transcript("api-x7Kd9a"); err == nil {
		t.Error("Transcript of an unreadable sessions.json returned no error")
	}
}

func TestPickerTranscripts_ReadsTheStateDir(t *testing.T) {
	dir := isolateStateDir(t)

	got, ok := pickerTranscripts().(indexTranscripts)
	if !ok || got.dir != dir {
		t.Errorf("pickerTranscripts() = %#v, want the state dir %s", got, dir)
	}
}

// previewedBody is the picker, its state reads wired as runPicker wires them,
// once its first listing lands and Space has previewed the session it lists.
func previewedBody(t *testing.T, s backend.Session, history string) string {
	t.Helper()
	b := backendtest.New(s)
	b.SetHistory(s.Name, []byte(history))
	cfg := defaultTestTUIConfig()
	cfg.lister = pickerLister{ctx: t.Context(), b: b}
	cfg.history = b
	cfg.wireStateReads()
	m := update(t, buildTUIModel(cfg, pickerLanding{}, nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, msg := range drainCmd(t, m.Init()) {
		m = update(t, m, msg)
	}
	updated, read := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	for _, msg := range drainCmd(t, read) {
		m = update(t, m, msg)
	}
	return ansi.Strip(m.View().Content)
}

func TestPicker_PreviewsAWaitingSessionsTranscript(t *testing.T) {
	const name = "api-x7Kd9a"
	session := backend.Session{Name: name, Created: transcriptCreated, PID: 4242}
	for _, tc := range []struct {
		desc    string
		waiting bool
		want    string
		notWant string
	}{
		{"while its resume-pending flag is raised", true, "saved transcript row", "resume panel screen"},
		{"and its History once the flag is down", false, "resume panel screen", "saved transcript row"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			file := transcriptFile(t, name)
			dir := seedIndex(t, sessionindex.Entry{Name: name, CreatedAt: transcriptCreated, PID: 4242, ScrollbackFile: file})
			writeTranscript(t, dir, file, "saved transcript row\r\n")
			if tc.waiting {
				if err := state.ResumePending.Set(dir, name); err != nil {
					t.Fatal(err)
				}
			}

			view := previewedBody(t, session, "resume panel screen")

			if !strings.Contains(view, tc.want) || strings.Contains(view, tc.notWant) {
				t.Errorf("preview wants %q and not %q, shows:\n%s", tc.want, tc.notWant, view)
			}
		})
	}
}
