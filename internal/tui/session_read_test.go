package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// rereadModel is rowActionModel reading one session afresh through reader.
func rereadModel(t *testing.T, sessions []Session, reader SessionReader) Model {
	t.Helper()
	m := New(&rowActionLister{sessions: sessions}, WithKiller(&killerStub{}), WithLabeller(&recordingLabeller{}), WithSessionReader(reader))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(SessionsMsg{Sessions: sessions})
	return updated.(Model)
}

// landReread runs cmd, which must be the read an open asked for, and delivers
// what it read.
func landReread(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil || quits(cmd) {
		t.Fatal("want the session read again, got no read")
	}
	msg, ok := cmd().(openReadMsg)
	if !ok {
		t.Fatalf("the command read %T, want the session read again", msg)
	}
	updated, next := m.Update(msg)
	return updated.(Model), next
}

func TestOpen_RereadsASessionListedNotAnswering(t *testing.T) {
	sessions := []Session{{Name: "wedged-Lp4s", NotAnswering: true}, {Name: "zulu"}}

	t.Run("Enter reads it again, attaching nothing until the read lands", func(t *testing.T) {
		reader := &takenReader{found: true}
		m := rereadModel(t, sessions, reader)

		m, cmd := pressKeys(m, keyEnter)

		if quits(cmd) || m.Selected() != "" || m.flashText != "" {
			t.Errorf("quits %v, selected %q, flash %q; want only a read", quits(cmd), m.Selected(), m.flashText)
		}
		if reader.reads != 0 {
			t.Errorf("read %d times on the update loop, want the read left to its command", reader.reads)
		}
		landReread(t, m, cmd)
		if reader.reads != 1 {
			t.Errorf("read %d times, want once", reader.reads)
		}
	})

	t.Run("a session that answers now is attached", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{found: true})

		m, cmd := pressKeys(m, keyEnter)
		m, next := landReread(t, m, cmd)

		if m.Selected() != "wedged-Lp4s" || !quits(next) {
			t.Errorf("selected %q, quits %v; want the session picked", m.Selected(), quits(next))
		}
	})

	t.Run("a session that still gives no answer flashes why and attaches nothing", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{err: errors.New("session wedged-Lp4s is not answering")})

		m, cmd := pressKeys(m, keyEnter)
		m, next := landReread(t, m, cmd)

		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; nothing was opened", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("a read found not answering flashes why and attaches nothing", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{session: Session{Name: "wedged-Lp4s", NotAnswering: true}, found: true})

		m, cmd := pressKeys(m, keyEnter)
		m, next := landReread(t, m, cmd)

		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; nothing was opened", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("a session read gone is opened, for the attach to say so as it does of any ended row", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{})

		m, cmd := pressKeys(m, keyEnter)
		m, next := landReread(t, m, cmd)

		if m.Selected() != "wedged-Lp4s" || !quits(next) {
			t.Errorf("selected %q, quits %v; want the session picked", m.Selected(), quits(next))
		}
	})

	t.Run("only the last open asked acts", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{found: true})

		m, first := pressKeys(m, keyEnter)
		m, second := pressKeys(m, keyEnter)

		m, next := landReread(t, m, first)
		if quits(next) || m.Selected() != "" {
			t.Errorf("quits %v, selected %q; want an earlier open's read to act on nothing", quits(next), m.Selected())
		}
		m, next = landReread(t, m, second)
		if m.Selected() != "wedged-Lp4s" || !quits(next) {
			t.Errorf("selected %q, quits %v; want the last open's read to pick the session", m.Selected(), quits(next))
		}
	})

	t.Run("a read landing once the user has moved on opens nothing", func(t *testing.T) {
		m := rereadModel(t, sessions, &takenReader{found: true})

		m, cmd := pressKeys(m, keyEnter)
		m, _ = pressKeys(m, press('k'))
		m, next := landReread(t, m, cmd)

		if quits(next) || m.Selected() != "" || m.modal != modalKillConfirm {
			t.Errorf("quits %v, selected %q, modal %v; want the kill prompt left open and nothing opened", quits(next), m.Selected(), m.modal)
		}
	})

	t.Run("a session listed as answering is attached without a read", func(t *testing.T) {
		reader := &takenReader{found: true}
		m := rereadModel(t, sessions, reader)

		m, cmd := pressKeys(m, keyDown, keyEnter)

		if m.Selected() != "zulu" || !quits(cmd) || reader.reads != 0 {
			t.Errorf("selected %q, quits %v, reads %d; want zulu picked unread", m.Selected(), quits(cmd), reader.reads)
		}
	})
}

// A picker a take opened lists its sessions as the taking terminal's take
// detaches every client of the session, so the listing can find the taken
// session not answering. Taking it back must not trust that listing.
func TestTaken_TakesBackASessionListedNotAnswering(t *testing.T) {
	listed := []Session{{Name: "alpha-a1b2c3"}, {Name: "bravo-d4e5f6", NotAnswering: true}, {Name: "charlie-g7h8i9"}}
	answering := Session{Name: "bravo-d4e5f6", Label: "release checklist"}

	t.Run("Enter on the modal reads it again and takes it back once it answers", func(t *testing.T) {
		lister := &rowActionLister{sessions: listed}
		m := watchedTakenModel(t, takenOverSSHAsk, lister, &takenReader{session: answering, found: true})

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		m, next := landReread(t, m, cmd)

		if m.Selected() != "bravo-d4e5f6" || !quits(next) {
			t.Errorf("selected %q, quits %v; want the taken session picked", m.Selected(), quits(next))
		}
	})

	t.Run("the watch's read puts the answering session into its row, the cursor kept on it", func(t *testing.T) {
		lister := &rowActionLister{sessions: listed}
		m := watchedTakenModel(t, takenOverSSHAsk, lister, &takenReader{session: answering, found: true})

		m, _ = recheckTaken(t, m)

		var row Session
		for _, it := range m.sessionList.Items() {
			if si, ok := it.(SessionItem); ok && si.Session.Name == "bravo-d4e5f6" {
				row = si.Session
			}
		}
		if row != answering {
			t.Errorf("the row shows %+v, want the session as read, %+v", row, answering)
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want it kept on the taken session's row", got)
		}

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape}, keyEnter)
		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want Enter on the row to pick it unread", m.Selected(), quits(cmd))
		}
	})
}
