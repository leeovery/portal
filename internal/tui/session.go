package tui

// Session is one picker row's session, in Portal's terms rather than any
// backend's. Name is the session's identity: selection, attach, kill and every
// set the picker keys rows by use it. Label is what the row shows.
type Session struct {
	Name string
	// Label is the user's text for the session; empty shows the name.
	Label string
	// Dir is the session's recorded directory, empty when none is recorded.
	Dir      string
	Attached bool
	// NotAnswering marks a session that is listed but did not answer, so
	// nothing beyond its name is known.
	NotAnswering bool
}

// DisplayName is the text the session's row shows.
func (s Session) DisplayName() string {
	if s.Label != "" {
		return s.Label
	}
	return s.Name
}
