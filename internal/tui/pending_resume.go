package tui

import "github.com/leeovery/portal/internal/tmux"

type PendingResumeReader interface {
	ListPendingResumePanes() (tmux.PendingResumeView, error)
}

func WithPendingResumeReader(r PendingResumeReader) Option {
	return func(m *Model) {
		m.pendingReader = r
	}
}

// readSessionList takes the pending read straight after the session
// enumeration so a row and its dot describe one moment. The error is the
// session read's alone: a failed pending read costs the dots, never the list.
func (m Model) readSessionList() ([]tmux.Session, map[string]struct{}, error) {
	sessions, err := m.sessionLister.ListSessions()
	if err != nil {
		return sessions, nil, err
	}
	return sessions, m.readPendingSessions(), nil
}

func (m Model) readPendingSessions() map[string]struct{} {
	if m.pendingReader == nil {
		return nil
	}
	view, err := m.pendingReader.ListPendingResumePanes()
	if err != nil {
		return nil
	}
	return view.Sessions
}
