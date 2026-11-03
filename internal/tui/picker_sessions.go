package tui

// PickerSessions returns the sessions the picker lists: the enumeration less the
// session the caller is attached to. An empty currentSession drops nothing, so a
// caller outside tmux — or one whose current-session read answered nothing —
// passes "" rather than branching.
//
// It never deletes in place: callers hand it an enumeration they go on reading,
// and an in-place delete would zero its tail. A filtered result is a fresh
// slice; an unfiltered one is the caller's own.
func PickerSessions(sessions []Session, currentSession string) []Session {
	if currentSession == "" {
		return sessions
	}
	picker := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.Name != currentSession {
			picker = append(picker, s)
		}
	}
	return picker
}
