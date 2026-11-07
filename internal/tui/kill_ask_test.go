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

// askKillers are the picker's killers: k's own, and the one y at the kill
// key's prompt kills through.
type askKillers struct{ k, asked *killerStub }

// killAskPicker is the picker the kill key's ask opens on asked, sized, its
// rows not yet listed.
func killAskPicker(t *testing.T, asked string, sessions []Session) (Model, askKillers) {
	t.Helper()
	killers := askKillers{k: &killerStub{}, asked: &killerStub{}}
	m := Build(Deps{
		Lister:      &rowActionLister{sessions: sessions},
		Killer:      killers.k,
		AskedKiller: killers.asked,
		Theme:       theme.ConstantNomination(testDarkTheme(t)),
		KillAsk:     asked,
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model), killers
}

// killAskModel is killAskPicker landed on sessions and the projects.
func killAskModel(t *testing.T, asked string, sessions []Session) (Model, askKillers) {
	t.Helper()
	m, killers := killAskPicker(t, asked, sessions)
	m = deliver(t, m, SessionsMsg{Sessions: sessions})
	return deliver(t, m, ProjectsLoadedMsg{}), killers
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
	t.Run("on the kill prompt for the session, its cursor on the session's row, offering d", func(t *testing.T) {
		m, _ := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		if m.modal != modalKillConfirm || !m.killFromInside {
			t.Fatalf("modal = %v, from inside %v; want the kill key's prompt", m.modal, m.killFromInside)
		}
		if m.pendingKill != killAskSessions[1] {
			t.Errorf("pending kill = %+v, want the listed %+v", m.pendingKill, killAskSessions[1])
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the asked session's row", got)
		}
		view := ansi.Strip(m.View().Content)
		for _, want := range []string{"Kill session?", "release checklist", "y kill   d detach   esc cancel"} {
			if !strings.Contains(view, want) {
				t.Errorf("the frame shows no %q:\n%s", want, view)
			}
		}
	})

	t.Run("on nothing for a session no longer listed", func(t *testing.T) {
		m, _ := killAskModel(t, "gone-x9y8z7", killAskSessions)

		if m.modal != modalNone || m.killFromInside {
			t.Errorf("modal = %v, from inside %v; want the plain picker", m.modal, m.killFromInside)
		}
		if m.activePage != PageSessions {
			t.Errorf("page = %v, want the sessions page", m.activePage)
		}
	})

	t.Run("on nothing with nothing to kill the session", func(t *testing.T) {
		m := Build(Deps{Lister: &rowActionLister{sessions: killAskSessions}, Killer: &killerStub{}, KillAsk: "bravo-d4e5f6"})
		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})

		if m.modal != modalNone || m.killAsk != "" {
			t.Errorf("modal = %v, ask pending %q; want the plain picker, its keys live", m.modal, m.killAsk)
		}
	})

	t.Run("on nothing with no ask", func(t *testing.T) {
		if m, _ := killAskModel(t, "", killAskSessions); m.modal != modalNone {
			t.Errorf("modal = %v, want none", m.modal)
		}
	})
}

func TestKillAsk_Keys(t *testing.T) {
	t.Run("y kills it through the asked killer and stays in the picker", func(t *testing.T) {
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
		m, next := land(t, m, cmd)

		if killers.asked.killedName != "bravo-d4e5f6" || killers.k.killedName != "" {
			t.Errorf("killed %q through the asked killer, %q through k's; want the asked session through the asked killer alone", killers.asked.killedName, killers.k.killedName)
		}
		if m.modal != modalNone || m.killFromInside || quits(next) || m.Selected() != "" {
			t.Errorf("modal %v, from inside %v, quits %v, selected %q; want the plain picker, open", m.modal, m.killFromInside, quits(next), m.Selected())
		}
	})

	t.Run("d keeps it and stays in the picker, the cursor on it", func(t *testing.T) {
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'd', Text: "d"})

		if killers.asked.killedName != "" || cmd != nil {
			t.Errorf("killed %q, cmd %v; want nothing done", killers.asked.killedName, cmd != nil)
		}
		if m.modal != modalNone || m.killFromInside || m.Selected() != "" {
			t.Errorf("modal %v, from inside %v, selected %q; want the plain picker", m.modal, m.killFromInside, m.Selected())
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the kept session's row", got)
		}
	})

	t.Run("Esc keeps it and goes back into it", func(t *testing.T) {
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if killers.asked.killedName != "" {
			t.Errorf("killed %q, want nothing killed", killers.asked.killedName)
		}
		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want the session picked", m.Selected(), quits(cmd))
		}
	})

	t.Run("Esc on a session not answering flashes why, as picking it would", func(t *testing.T) {
		sessions := []Session{{Name: "alpha-a1b2c3"}, {Name: "bravo-d4e5f6", NotAnswering: true}}
		m, _ := killAskModel(t, "bravo-d4e5f6", sessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		assertFlashedAndOpen(t, m, cmd, "session bravo-d4e5f6 is not answering; nothing was opened", []string{"alpha-a1b2c3", "bravo-d4e5f6"})
	})

	t.Run("Enter does nothing", func(t *testing.T) {
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		if m.modal != modalKillConfirm || !m.killFromInside || m.Selected() != "" || cmd != nil || killers.asked.killedName != "" {
			t.Errorf("modal %v, from inside %v, selected %q, cmd %v, killed %q; want Enter ignored", m.modal, m.killFromInside, m.Selected(), cmd != nil, killers.asked.killedName)
		}
	})
}

// k's own prompt, opened once d has kept the asked session, is the plain one.
func TestKillAsk_KsOwnPromptAfterAnAsk(t *testing.T) {
	kPrompt := func(t *testing.T) (Model, askKillers) {
		t.Helper()
		m, killers := killAskModel(t, "bravo-d4e5f6", killAskSessions)
		m, _ = pressKeys(m, tea.KeyPressMsg{Code: 'd', Text: "d"}, tea.KeyPressMsg{Code: 'k', Text: "k"})
		if m.modal != modalKillConfirm || m.killFromInside || m.pendingKill.Name != "bravo-d4e5f6" {
			t.Fatalf("modal %v, from inside %v, pending %q; want k's prompt on the kept session", m.modal, m.killFromInside, m.pendingKill.Name)
		}
		return m, killers
	}

	t.Run("reads y kill, esc cancel, and offers no d", func(t *testing.T) {
		m, _ := kPrompt(t)

		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "y kill   esc cancel") || strings.Contains(view, "detach") {
			t.Errorf("k's prompt reads, want y kill then esc cancel and no d:\n%s", view)
		}
	})

	t.Run("Esc keeps it and stays in the picker", func(t *testing.T) {
		m, _ := kPrompt(t)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if m.modal != modalNone || m.Selected() != "" || cmd != nil {
			t.Errorf("modal %v, selected %q, cmd %v; want the plain picker", m.modal, m.Selected(), cmd != nil)
		}
	})

	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'd', Text: "d"}} {
		t.Run(key.String()+" does nothing", func(t *testing.T) {
			m, _ := kPrompt(t)

			m, cmd := pressKeys(m, key)

			if m.modal != modalKillConfirm || m.Selected() != "" || cmd != nil {
				t.Errorf("modal %v, selected %q, cmd %v; want the key ignored", m.modal, m.Selected(), cmd != nil)
			}
		})
	}

	t.Run("y kills through k's killer", func(t *testing.T) {
		m, killers := kPrompt(t)

		_, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
		_ = cmd()

		if killers.k.killedName != "bravo-d4e5f6" || killers.asked.killedName != "" {
			t.Errorf("killed %q through k's killer, %q through the asked; want the session through k's alone", killers.k.killedName, killers.asked.killedName)
		}
	})
}

// Init reads the rows and the projects at once, so either can land first.
func TestKillAsk_KeysTypedAhead(t *testing.T) {
	t.Run("before the rows land, a key is dropped, Ctrl+C aside", func(t *testing.T) {
		m, _ := killAskPicker(t, "bravo-d4e5f6", killAskSessions)

		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'q', Text: "q"}, {Code: tea.KeyEnter}, {Code: tea.KeyDown}} {
			var cmd tea.Cmd
			if m, cmd = pressKeys(m, key); cmd != nil || m.Selected() != "" {
				t.Errorf("%v: cmd %v, selected %q; want the key dropped", key, cmd != nil, m.Selected())
			}
		}
		if _, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); !quits(cmd) {
			t.Error("Ctrl+C did not quit")
		}

		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})
		if m.modal != modalKillConfirm || selectedSessionName(t, m) != "bravo-d4e5f6" {
			t.Errorf("modal %v, cursor on %q; want the prompt on the asked session, no key typed ahead acting", m.modal, selectedSessionName(t, m))
		}
	})

	t.Run("the rows landing before the projects open the prompt, so Esc attaches the asked session", func(t *testing.T) {
		m, _ := killAskPicker(t, "bravo-d4e5f6", killAskSessions)
		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want the asked session attached, not the first row", m.Selected(), quits(cmd))
		}
	})

	t.Run("the projects landing first change nothing: the prompt opens with the rows", func(t *testing.T) {
		m, _ := killAskPicker(t, "bravo-d4e5f6", killAskSessions)
		m = deliver(t, m, ProjectsLoadedMsg{})
		if _, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
			t.Error("Esc between the projects and the rows acted")
		}

		if m = deliver(t, m, SessionsMsg{Sessions: killAskSessions}); m.modal != modalKillConfirm {
			t.Errorf("modal = %v, want the kill prompt", m.modal)
		}
	})

	t.Run("a session not listed lets keys act again", func(t *testing.T) {
		m, _ := killAskPicker(t, "gone-x9y8z7", killAskSessions)
		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})

		if _, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'q', Text: "q"}); !quits(cmd) {
			t.Error("q did not quit the plain picker")
		}
	})
}
