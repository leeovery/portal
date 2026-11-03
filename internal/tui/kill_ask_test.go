package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

var killAskSessions = []Session{
	{Name: "alpha-a1b2c3"},
	{Name: "bravo-d4e5f6", Label: "release checklist"},
	{Name: "charlie-g7h8i9"},
}

// killAskModel is the picker the kill key's ask opens, on asked, landed on
// sessions.
func killAskModel(t *testing.T, asked string, sessions []Session, killer SessionKiller) Model {
	t.Helper()
	m := Build(Deps{
		Lister:  &rowActionLister{sessions: sessions},
		Killer:  killer,
		Theme:   theme.ConstantNomination(testDarkTheme(t)),
		KillAsk: asked,
	})
	var updated tea.Model = m
	updated, _ = updated.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(SessionsMsg{Sessions: sessions})
	updated, _ = updated.Update(ProjectsLoadedMsg{})
	return updated.(Model)
}

func selectedSessionName(t *testing.T, m Model) string {
	t.Helper()
	si, ok := m.selectedSessionItem()
	if !ok {
		t.Fatal("no session row is selected")
	}
	return si.Session.Name
}

func TestKillAsk_Opens(t *testing.T) {
	t.Run("on the kill prompt for the session, its cursor on the session's row, offering Enter", func(t *testing.T) {
		m := killAskModel(t, "bravo-d4e5f6", killAskSessions, &killerStub{})

		if m.modal != modalKillConfirm || !m.killOffersAttach {
			t.Fatalf("modal = %v, offers attach %v; want the kill prompt offering Enter", m.modal, m.killOffersAttach)
		}
		if m.pendingKill != killAskSessions[1] {
			t.Errorf("pending kill = %+v, want the listed %+v", m.pendingKill, killAskSessions[1])
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the asked session's row", got)
		}
		view := ansi.Strip(m.View().Content)
		for _, want := range []string{"Kill session?", "release checklist", "y kill", "⏎ attach", "esc cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("the frame shows no %q:\n%s", want, view)
			}
		}
	})

	t.Run("on nothing for a session no longer listed", func(t *testing.T) {
		m := killAskModel(t, "gone-x9y8z7", killAskSessions, &killerStub{})

		if m.modal != modalNone || m.killOffersAttach {
			t.Errorf("modal = %v, offers attach %v; want the plain picker", m.modal, m.killOffersAttach)
		}
		if m.activePage != PageSessions {
			t.Errorf("page = %v, want the sessions page", m.activePage)
		}
	})

	t.Run("on nothing with no ask", func(t *testing.T) {
		if m := killAskModel(t, "", killAskSessions, &killerStub{}); m.modal != modalNone {
			t.Errorf("modal = %v, want none", m.modal)
		}
	})
}

func TestKillAsk_Keys(t *testing.T) {
	t.Run("y kills it and stays in the picker", func(t *testing.T) {
		killer := &killerStub{}
		m := killAskModel(t, "bravo-d4e5f6", killAskSessions, killer)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
		m, next := land(t, m, cmd)

		if killer.killedName != "bravo-d4e5f6" {
			t.Errorf("killed %q, want the asked session", killer.killedName)
		}
		if m.modal != modalNone || m.killOffersAttach || quits(next) || m.Selected() != "" {
			t.Errorf("modal %v, offers attach %v, quits %v, selected %q; want the plain picker, open", m.modal, m.killOffersAttach, quits(next), m.Selected())
		}
	})

	t.Run("Esc keeps it and stays in the picker, the cursor on it", func(t *testing.T) {
		killer := &killerStub{}
		m := killAskModel(t, "bravo-d4e5f6", killAskSessions, killer)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if killer.killedName != "" || cmd != nil {
			t.Errorf("killed %q, cmd %v; want nothing done", killer.killedName, cmd != nil)
		}
		if m.modal != modalNone || m.killOffersAttach || m.Selected() != "" {
			t.Errorf("modal %v, offers attach %v, selected %q; want the plain picker", m.modal, m.killOffersAttach, m.Selected())
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the kept session's row", got)
		}
	})

	t.Run("Enter keeps it and attaches it again", func(t *testing.T) {
		killer := &killerStub{}
		m := killAskModel(t, "bravo-d4e5f6", killAskSessions, killer)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		if killer.killedName != "" {
			t.Errorf("killed %q, want nothing killed", killer.killedName)
		}
		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want the session picked", m.Selected(), quits(cmd))
		}
	})

	t.Run("Enter on a session not answering flashes why, as picking it would", func(t *testing.T) {
		sessions := []Session{{Name: "alpha-a1b2c3"}, {Name: "bravo-d4e5f6", NotAnswering: true}}
		m := killAskModel(t, "bravo-d4e5f6", sessions, &killerStub{})

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		assertFlashedAndOpen(t, m, cmd, "session bravo-d4e5f6 is not answering; nothing was opened", []string{"alpha-a1b2c3", "bravo-d4e5f6"})
	})

	t.Run("k's own prompt takes no Enter, after an ask as before one", func(t *testing.T) {
		killer := &killerStub{}
		m := killAskModel(t, "bravo-d4e5f6", killAskSessions, killer)

		m, _ = pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape}, tea.KeyPressMsg{Code: 'k', Text: "k"})
		if m.modal != modalKillConfirm || m.killOffersAttach || m.pendingKill.Name != "bravo-d4e5f6" {
			t.Fatalf("modal %v, offers attach %v, pending %q; want k's prompt on the kept session", m.modal, m.killOffersAttach, m.pendingKill.Name)
		}
		if view := ansi.Strip(m.View().Content); strings.Contains(view, "attach") {
			t.Errorf("k's prompt offers to attach:\n%s", view)
		}
		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		if m.modal != modalKillConfirm || m.Selected() != "" || cmd != nil {
			t.Errorf("modal %v, selected %q, cmd %v; want Enter ignored", m.modal, m.Selected(), cmd != nil)
		}
	})
}
