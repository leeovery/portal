package tui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// SessionReader reads one session as the picker's rows show it. found is
// false for a session an attach of it would find gone; an error is a read
// that could tell neither.
type SessionReader interface {
	ReadSession(name string) (s Session, found bool, err error)
}

// WithSessionReader has the picker read one session afresh through reader:
// the taken screen's watch does, and so does an open of a session its
// listing found not answering, since a listing is read once and a session it
// caught not answering may answer a moment later.
func WithSessionReader(reader SessionReader) Option {
	return func(m *Model) {
		m.sessionReader = reader
	}
}

// openReadMsg is a fresh read of name, which an open asked for because the
// listing found it not answering.
type openReadMsg struct {
	gen     uint64
	name    string
	session Session
	found   bool
	err     error
}

// openSession picks the session name to attach once the picker ends. Every
// open of a listed session comes through here. One the listing found not
// answering is read again first, since the listing may be long out of date:
// a picker opened by a take lists its sessions as the taking terminal's take
// detaches every client of the session, its probe among them. With no reader
// to ask, it is refused on the listing's word.
func (m Model) openSession(name string) (tea.Model, tea.Cmd) {
	if m.answering(name) {
		m.selected = name
		return m, tea.Quit
	}
	if m.sessionReader == nil {
		return m.refuseNotAnswering(name)
	}
	m.openReadGen++
	reader, gen := m.sessionReader, m.openReadGen
	return m, func() tea.Msg {
		s, found, err := reader.ReadSession(name)
		return openReadMsg{gen: gen, name: name, session: s, found: found, err: err}
	}
}

// landOpenRead acts on the read an open asked for, only if it is the last
// open asked and the picker is still on its list with nothing over it, so a
// read landing after the user moved on opens nothing. A session that
// answers is opened; so is one found gone, as an open of any row whose
// session has since ended is, which the attach reports. One that still gives
// no answer is refused, its row as the listing left it.
func (m Model) landOpenRead(msg openReadMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.openReadGen || m.activePage != PageSessions || m.modal != modalNone {
		return m, nil
	}
	switch {
	case msg.err != nil:
		return m.refuseNotAnswering(msg.name)
	case msg.found && msg.session.NotAnswering:
		return m.refuseNotAnswering(msg.name)
	}
	m.selected = msg.name
	return m, tea.Quit
}

// refuseNotAnswering flashes why the session name was not opened.
func (m Model) refuseNotAnswering(name string) (tea.Model, tea.Cmd) {
	return m.flashError(fmt.Errorf("session %s is not answering; nothing was opened", name))
}

// answering reports whether the listed session name answered its listing; one
// not listed is taken as answering, there being nothing to say otherwise.
func (m Model) answering(name string) bool {
	i := slices.IndexFunc(m.sessions, func(s Session) bool { return s.Name == name })
	return i < 0 || !m.sessions[i].NotAnswering
}

// takeInRead puts a fresh read of one session into its row, so the row shows
// what the read found rather than what the last listing did, the cursor kept
// on the session it was on. A session the listing doesn't hold gets no row:
// only a listing adds one.
func (m *Model) takeInRead(s Session) tea.Cmd {
	i := slices.IndexFunc(m.sessions, func(r Session) bool { return r.Name == s.Name })
	if i < 0 || m.sessions[i] == s {
		return nil
	}
	var held string
	if si, ok := m.selectedSessionItem(); ok {
		held = si.Session.Name
	}
	// A copy, since earlier values of the model share the listing's array.
	m.sessions = slices.Clone(m.sessions)
	m.sessions[i] = s
	cmd := m.rebuildSessionList()
	m.reanchorSessionCursor(held)
	return cmd
}
