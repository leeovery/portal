package tui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TakenRecheckEvery is how long the taken screen waits between reads of its
// session: often enough that it says so soon after the session ends, and one
// session's read, so a screen left open for days costs next to nothing.
const TakenRecheckEvery = 2 * time.Second

// SessionReader reads one session as the picker's rows show it. found is
// false for a session an attach of it would find gone; an error is a read
// that could tell neither.
type SessionReader interface {
	ReadSession(name string) (s Session, found bool, err error)
}

// takenWatch reads the taken screen's session again while the screen, or its
// watch, is open, so the screen says so once the session ends rather than
// offering to take back a session that is gone.
type takenWatch struct {
	reader SessionReader
	// every is how long it waits between reads; zero reads none.
	every time.Duration
	// gen numbers the watch, so a tick or a read an earlier one sent lands on
	// nothing.
	gen uint64
}

type takenWatchTickMsg struct{ gen uint64 }

type takenReadMsg struct {
	gen     uint64
	session Session
	found   bool
	err     error
}

// WithTakenWatch has the taken screen read its session through reader every
// on while it is open.
func WithTakenWatch(reader SessionReader, every time.Duration) Option {
	return func(m *Model) {
		m.takenWatch.reader, m.takenWatch.every = reader, every
	}
}

// WithInitialTakenEnded lands the taken screen as it shows once its session
// has ended, which a one-shot render can't reach by waiting for a read.
func WithInitialTakenEnded(ended bool) Option {
	return func(m *Model) {
		m.initialTakenEnded = ended
	}
}

// tick waits out the gap before the next read of the taken session; with no
// reader or no gap, there is none.
func (w takenWatch) tick() tea.Cmd {
	if w.reader == nil || w.every <= 0 {
		return nil
	}
	gen := w.gen
	return tea.Tick(w.every, func(time.Time) tea.Msg { return takenWatchTickMsg{gen: gen} })
}

// startTakenWatch begins a watch of the session the taken screen just opened
// on, ending any before it.
func (m *Model) startTakenWatch() tea.Cmd {
	m.takenWatch.gen++
	if m.takenEnded {
		return nil
	}
	return m.takenWatch.tick()
}

// takenLive reports whether the taken screen, or its watch, is open on a
// session not known to have ended.
func (m Model) takenLive() bool {
	return m.takenSession.Name != "" && !m.takenEnded
}

// readTaken reads the taken session again, off the update loop, once the
// watch's tick comes.
func (m Model) readTaken(msg takenWatchTickMsg) tea.Cmd {
	if msg.gen != m.takenWatch.gen || !m.takenLive() || m.takenWatch.reader == nil {
		return nil
	}
	reader, name, gen := m.takenWatch.reader, m.takenSession.Name, msg.gen
	return func() tea.Msg {
		s, found, err := reader.ReadSession(name)
		return takenReadMsg{gen: gen, session: s, found: found, err: err}
	}
}

// applyTakenRead takes in a read of the taken session: gone, the screen says
// it has ended; found, it shows the session as read, its label included; a
// read that could tell neither changes nothing. Until the session has ended,
// the watch reads it again.
func (m *Model) applyTakenRead(msg takenReadMsg) tea.Cmd {
	if msg.gen != m.takenWatch.gen || !m.takenLive() {
		return nil
	}
	switch {
	case msg.err != nil:
	case !msg.found:
		return m.endTaken()
	default:
		m.takenSession = msg.session
	}
	return m.takenWatch.tick()
}

// reconcileTaken holds the taken screen to the listing just taken in: a
// session it no longer lists has ended, and one it lists shows as listed.
func (m *Model) reconcileTaken() tea.Cmd {
	if !m.takenLive() {
		return nil
	}
	i := slices.IndexFunc(m.sessions, func(s Session) bool { return s.Name == m.takenSession.Name })
	if i < 0 {
		return m.endTaken()
	}
	m.takenSession = m.sessions[i]
	return nil
}

// endTaken turns the taken screen to say its session has ended, closing its
// watch first when that is open.
func (m *Model) endTaken() tea.Cmd {
	m.takenEnded = true
	if !m.watchingTaken {
		return nil
	}
	return m.exitPreviewToSessions("")
}
