package tui

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeTranscripts holds each session's saved transcript by name; a session it
// doesn't hold has none. It records every session read.
type fakeTranscripts struct {
	by    map[string]string
	err   error
	reads *[]string
}

func (f fakeTranscripts) Transcript(session string) ([]byte, error) {
	if f.reads != nil {
		*f.reads = append(*f.reads, session)
	}
	if f.err != nil {
		return nil, f.err
	}
	vt, ok := f.by[session]
	if !ok {
		return nil, errors.New("no saved transcript")
	}
	return []byte(vt), nil
}

// waitingModel is the picker on alpha, whose History is the panel's screen,
// with the given sessions waiting on their resume panel.
func waitingModel(t *testing.T, transcripts TranscriptReader, waiting ...string) Model {
	t.Helper()
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFor(t, "resume panel", sessions))
	m.transcripts = transcripts
	m.pendingSessions = map[string]struct{}{}
	for _, name := range waiting {
		m.pendingSessions[name] = struct{}{}
	}
	return m
}

func TestPreviewOfAWaitingSessionShowsTheTranscriptItWasRestoredFrom(t *testing.T) {
	var reads []string
	// A History capture's form: rows, then a closing state the preview never
	// draws.
	transcripts := fakeTranscripts{by: map[string]string{"alpha": "$ make\r\nbuilt\x1b[2;7H"}, reads: &reads}
	m := waitingModel(t, transcripts, "alpha")

	got := openPreviewOn(t, m)

	if body := previewBody(got.preview); !slices.Equal(body, []string{"$ make", "built"}) {
		t.Errorf("body = %q, want the transcript drawn as History is", body)
	}
	if !slices.Equal(reads, []string{"alpha"}) {
		t.Errorf("transcripts read %v, want alpha's once", reads)
	}
}

func TestPreviewOfAWaitingSessionDialsNoHistoryWhenItsTranscriptReads(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "resume panel", sessions)
	m := modelWithSeams(t, sessions, history)
	m.transcripts = fakeTranscripts{by: map[string]string{"alpha": "saved"}}
	m.pendingSessions = map[string]struct{}{"alpha": {}}

	openPreviewOn(t, m)

	if calls := historyCalls(history); len(calls) != 0 {
		t.Errorf("History read %v, want none for a waiting session with a transcript", calls)
	}
}

func TestPreviewOfASessionNotWaitingShowsItsHistory(t *testing.T) {
	var reads []string
	transcripts := fakeTranscripts{by: map[string]string{"alpha": "saved"}, reads: &reads}
	m := waitingModel(t, transcripts, "bravo")

	got := openPreviewOn(t, m)

	if body := lastBodyRow(got.preview); body != "resume panel" {
		t.Errorf("body = %q, want the History of a session not waiting", body)
	}
	if len(reads) != 0 {
		t.Errorf("transcripts read %v, want none for a session not waiting", reads)
	}
}

func TestPreviewOfAWaitingSessionFallsBackToItsHistory(t *testing.T) {
	cases := []struct {
		name        string
		transcripts TranscriptReader
	}{
		{"no transcript", fakeTranscripts{}},
		{"an unreadable transcript", fakeTranscripts{err: errors.New("permission denied")}},
		{"an empty transcript", fakeTranscripts{by: map[string]string{"alpha": ""}}},
		{"no transcript reader", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := openPreviewOn(t, waitingModel(t, tc.transcripts, "alpha"))

			if body := lastBodyRow(got.preview); body != "resume panel" {
				t.Errorf("body = %q, want the History", body)
			}
		})
	}
}

// endingTranscripts answers with a transcript, but ends the open it is read
// for first, as the next open does while a read is under way.
type endingTranscripts struct {
	end context.CancelFunc
}

func (e endingTranscripts) Transcript(string) ([]byte, error) {
	e.end()
	return []byte("drawn only if the read was still wanted"), nil
}

func TestPreviewOfAWaitingSessionWhoseOpenEndsDuringTheReadDrawsNothing(t *testing.T) {
	open, end := context.WithCancel(context.Background())
	m := waitingModel(t, endingTranscripts{end: end}, "alpha")

	if msg := m.previewRead(open, "alpha", 1)(); msg != nil {
		t.Errorf("read whose open ended mid-read returned %#v, want no message", msg)
	}
}
