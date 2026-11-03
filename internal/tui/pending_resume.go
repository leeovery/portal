package tui

// PendingResumeReader names the sessions with anything awaiting a resume
// decision; a session carries one dot however much of it waits.
type PendingResumeReader interface {
	ListPendingResumeSessions() (map[string]struct{}, error)
}

func WithPendingResumeReader(r PendingResumeReader) Option {
	return func(m *Model) {
		m.pendingReader = r
	}
}

// readSessionList takes the pending read straight after the session
// enumeration so a row and its dot describe one moment. The error is the
// session read's alone: a failed pending read costs the dots, never the list.
func (m Model) readSessionList() ([]Session, map[string]struct{}, error) {
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
	pending, err := m.pendingReader.ListPendingResumeSessions()
	if err != nil {
		return nil
	}
	return pending
}
