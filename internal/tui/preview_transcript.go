package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// TranscriptReader reads the transcript a session was restored from: the
// saved scrollback its index entry names, which the hydrate helper replayed
// into it, a History capture in the same form. An error means it has none or
// it can't be read.
type TranscriptReader interface {
	Transcript(session string) ([]byte, error)
}

// WithTranscripts is where the preview reads a waiting session's transcript.
// Without it, a waiting session previews its History.
func WithTranscripts(r TranscriptReader) Option {
	return func(m *Model) {
		m.transcripts = r
	}
}

// previewRead is the command that reads and draws session's preview body. A
// session waiting on its resume panel previews the transcript it was restored
// from: the panel draws on the alternate screen, and History of a session
// there holds only that screen, so the replayed transcript beneath it can't
// be read until the panel closes. Whether it waits is read with the
// transcript, never taken from the listing's snapshot: a panel answered since
// leaves a live session that has moved on from it. With no transcript, or one
// that can't be read, it previews its History as any other session does.
func (m Model) previewRead(open context.Context, session string, gen uint64) tea.Cmd {
	if m.transcripts == nil || m.pendingReader == nil {
		return m.history.read(open, session, gen)
	}
	pending, transcripts, history := m.pendingReader, m.transcripts, m.history
	return func() tea.Msg {
		if open.Err() != nil {
			return nil
		}
		vt, ok := waitingTranscript(pending, transcripts, session)
		switch {
		case open.Err() != nil:
			return nil
		case ok:
			return previewBodyMsg{gen: gen, body: renderHistory(vt)}
		}
		return history.read(open, session, gen)()
	}
}

// waitingTranscript is session's transcript while its resume-pending flag is
// raised as it stands now. ok is false when it isn't waiting, the flags can't
// be read, or it has no transcript that reads.
func waitingTranscript(pending PendingResumeReader, transcripts TranscriptReader, session string) (vt []byte, ok bool) {
	raised, err := pending.ListPendingResumeSessions()
	if _, waiting := raised[session]; err != nil || !waiting {
		return nil, false
	}
	vt, err = transcripts.Transcript(session)
	return vt, err == nil && len(vt) > 0
}
