package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

// The listing the loading page took in predates the restore, so only the
// post-restore listing may open the prompt: left pending, the prompt drops
// every key but Ctrl+C.
func TestKillAsk_PostRestoreListingOpensThePromptPastTheLoadingPage(t *testing.T) {
	m := Build(Deps{
		Lister:           &rowActionLister{sessions: killAskSessions},
		Killer:           &killerStub{},
		AskedKiller:      &killerStub{},
		Theme:            theme.ConstantNomination(testDarkTheme(t)),
		KillAsk:          "bravo-d4e5f6",
		ProgressReceiver: func() tea.Msg { return BootstrapProgressMsg{Index: 1} },
	})
	m = deliver(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.activePage != PageLoading {
		t.Fatalf("page = %v, want the loading page", m.activePage)
	}

	m = deliver(t, m, SessionsMsg{Sessions: killAskSessions})
	m = deliver(t, m, LoadingMinElapsedMsg{})
	m = deliver(t, m, BootstrapCompleteMsg{})
	if m.activePage == PageLoading {
		t.Fatalf("the loading page held past both gates")
	}
	if m.modal == modalKillConfirm {
		t.Fatal("the pre-restore listing opened the prompt")
	}
	m = deliver(t, m, SessionsMsg{Sessions: killAskSessions, afterRestore: true})

	if m.killAsk != "" || m.modal != modalKillConfirm || m.pendingKill.Name != "bravo-d4e5f6" {
		t.Fatalf("ask pending %q, modal %v, pending kill %q; want the prompt open on the asked session", m.killAsk, m.modal, m.pendingKill.Name)
	}
	if m, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'd', Text: "d"}); m.modal != modalNone || cmd != nil {
		t.Errorf("d left modal %v, cmd %v; want the prompt to take the key", m.modal, cmd != nil)
	}
}
