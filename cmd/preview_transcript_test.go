package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
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
