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
// be read until the panel closes. With no transcript, or one that can't be
// read, it previews its History as any other session does.
func (m Model) previewRead(open context.Context, session string, gen uint64) tea.Cmd {
	_, waiting := m.pendingSessions[session]
	if !waiting || m.transcripts == nil {
		return m.history.read(open, session, gen)
	}
	transcripts, history := m.transcripts, m.history
	return func() tea.Msg {
		vt, err := transcripts.Transcript(session)
		switch {
		case open.Err() != nil:
			return nil
		case err == nil && len(vt) > 0:
			return previewBodyMsg{gen: gen, body: renderHistory(vt)}
		}
		return history.read(open, session, gen)()
	}
}
