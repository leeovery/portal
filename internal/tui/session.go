package tui

import (
	"cmp"

	"github.com/leeovery/portal/internal/resolver"
)

// Session is one picker row's session, in Portal's terms rather than any
// backend's. Name is the session's identity: selection, attach, kill and every
// set the picker keys rows by use it. Label is what the row shows.
type Session struct {
	Name string
	// Label is the user's text for the session; empty shows the name.
	Label string
	// Dir is the session's recorded directory, empty when none is recorded.
	Dir string
	// Cwd is the session's working directory as its backend reports it, empty
	// when unknown. Grouping alone reads it, for a session with no Dir; it is
	// never shown or searched.
	Cwd      string
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

// SearchSubject is what a filter or search matches the session by.
func (s Session) SearchSubject() resolver.SearchSubject {
	return resolver.SearchSubject{Label: s.Label, Name: s.Name, Dir: s.Dir}
}

// compareShown orders sessions by the text their rows show, then by name, so
// a labelled session sits where its text says and the order is stable.
func compareShown(a, b Session) int {
	return cmp.Or(cmp.Compare(a.DisplayName(), b.DisplayName()), cmp.Compare(a.Name, b.Name))
}
