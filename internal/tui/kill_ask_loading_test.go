package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

// killAskLoadingPicker is the picker the kill key's ask opens on asked while a
// bootstrap runs behind its loading page, through receiver when it has one.
func killAskLoadingPicker(t *testing.T, asked string, receiver tea.Cmd) Model {
	t.Helper()
	m := Build(Deps{
		Lister:           &rowActionLister{sessions: killAskSessions},
		Killer:           &killerStub{},
		AskedKiller:      &killerStub{},
		Theme:            theme.ConstantNomination(testDarkTheme(t)),
		KillAsk:          asked,
		Bootstrapping:    true,
		ProgressReceiver: receiver,
	})
	m = deliver(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.activePage != PageLoading {
		t.Fatalf("page = %v, want the loading page", m.activePage)
	}
	return m
}

// passLoadingGates delivers both of the loading page's gates.
func passLoadingGates(t *testing.T, m Model) Model {
	t.Helper()
	m = deliver(t, m, LoadingMinElapsedMsg{})
	m = deliver(t, m, BootstrapCompleteMsg{})
	if m.activePage == PageLoading {
		t.Fatalf("the loading page held past both gates")
	}
	return m
}

func assertKillPromptOpen(t *testing.T, m Model) {
	t.Helper()
	if m.killAsk != "" || m.modal != modalKillConfirm || m.pendingKill.Name != "bravo-d4e5f6" {
		t.Fatalf("ask pending %q, modal %v, pending kill %q; want the prompt open on the asked session", m.killAsk, m.modal, m.pendingKill.Name)
	}
	if m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape}); m.modal != modalNone || cmd != nil {
		t.Errorf("Esc left modal %v, cmd %v; want the prompt to take the key", m.modal, cmd != nil)
	}
}

func TestKillAsk_LandsPastTheLoadingPage(t *testing.T) {
	t.Run("with no receiver, the listing the loading page took in opens it", func(t *testing.T) {
		m := killAskLoadingPicker(t, "bravo-d4e5f6", nil)

		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})
		m = passLoadingGates(t, m)

		assertKillPromptOpen(t, m)
	})

	t.Run("with no receiver, a listing landing past the loading page opens it", func(t *testing.T) {
		m := killAskLoadingPicker(t, "bravo-d4e5f6", nil)

		m = passLoadingGates(t, m)
		if m.killAsk == "" {
			t.Fatal("the ask was spent before any listing landed")
		}
		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})

		assertKillPromptOpen(t, m)
	})

	t.Run("with a receiver, the post-restore listing opens it", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return BootstrapProgressMsg{Index: 1} })
		m := killAskLoadingPicker(t, "bravo-d4e5f6", receiver)

		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})
		m = passLoadingGates(t, m)
		m = deliver(t, m, SessionsMsg{Sessions: killAskSessions, afterRestore: true})

		assertKillPromptOpen(t, m)
	})
}
