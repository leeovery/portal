package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func TestWhereFrom(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"over SSH, the client's address", map[string]string{"SSH_CONNECTION": "192.168.1.20 52144 192.168.1.5 22"}, "SSH from 192.168.1.20"},
		{"SSH first, whatever the terminal app", map[string]string{"SSH_CONNECTION": "10.0.0.7 52144 10.0.0.2 22", "TERM_PROGRAM": "ghostty"}, "SSH from 10.0.0.7"},
		{"an SSH_CONNECTION naming no address is no SSH", map[string]string{"SSH_CONNECTION": "  ", "TERM_PROGRAM": "ghostty"}, "Ghostty on this Mac"},
		{"Ghostty by its own name", map[string]string{"TERM_PROGRAM": "ghostty"}, "Ghostty on this Mac"},
		{"any other app as it names itself", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "iTerm.app on this Mac"},
		{"an empty TERM_PROGRAM names no app", map[string]string{"TERM_PROGRAM": " "}, "another terminal on this Mac"},
		{"neither, another terminal", map[string]string{}, "another terminal on this Mac"},
		{"what a terminal would act on is dropped", map[string]string{"TERM_PROGRAM": "Evil\x1b]0;x\x07App"}, "Evil]0;xApp on this Mac"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				v, ok := c.env[key]
				return v, ok
			}
			if got := WhereFrom(lookup); got != c.want {
				t.Errorf("WhereFrom = %q, want %q", got, c.want)
			}
		})
	}
}

var takenSessions = []Session{
	{Name: "alpha-a1b2c3"},
	{Name: "bravo-d4e5f6", Label: "release checklist"},
	{Name: "charlie-g7h8i9"},
}

// takenPicker is the picker opened on taken, sized, its rows not yet listed,
// previewing through history.
func takenPicker(t *testing.T, taken Taken, sessions []Session) Model {
	t.Helper()
	m := Build(Deps{
		Lister:  &rowActionLister{sessions: sessions},
		Theme:   theme.ConstantNomination(testDarkTheme(t)),
		History: historyFor(t, "the session's screen", sessions),
		Taken:   taken,
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

// takenModel is takenPicker landed on sessions and the projects.
func takenModel(t *testing.T, taken Taken, sessions []Session) Model {
	t.Helper()
	m := deliver(t, takenPicker(t, taken, sessions), SessionsMsg{Sessions: sessions})
	return deliver(t, m, ProjectsLoadedMsg{})
}

var takenOverSSHAsk = Taken{Session: "bravo-d4e5f6", By: "SSH from 192.168.1.20"}

func TestTaken_Opens(t *testing.T) {
	t.Run("on the modal for the session, its cursor on the session's row", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		if m.modal != modalTaken || m.takenSession != takenSessions[1] {
			t.Fatalf("modal = %v on %+v; want the taken modal on the listed %+v", m.modal, m.takenSession, takenSessions[1])
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the taken session's row", got)
		}
	})

	t.Run("saying where the session went, and offering its three keys", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		view := ansi.Strip(m.View().Content)
		for _, want := range []string{"release checklist is open on another device", "(SSH from 192.168.1.20)", "⏎ take it back   space watch   esc picker"} {
			if !strings.Contains(view, want) {
				t.Errorf("the frame shows no %q:\n%s", want, view)
			}
		}
		if strings.Contains(view, "alpha-a1b2c3") {
			t.Errorf("the frame shows the session list behind the modal:\n%s", view)
		}
	})

	t.Run("saying only that it is open on another device when where is unknown", func(t *testing.T) {
		m := takenModel(t, Taken{Session: "alpha-a1b2c3"}, takenSessions)

		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "alpha-a1b2c3 is open on another device") || strings.Contains(view, "(") {
			t.Errorf("the frame reads, want the first line alone:\n%s", view)
		}
	})

	t.Run("in no danger colour", func(t *testing.T) {
		th := testDarkTheme(t)
		danger := headerStyle(th.StateDestructive, th, false).Render("x")
		dangerSGR := danger[:strings.Index(danger, "x")]

		if frame := renderTakenModalOnClearedCanvas("bravo-d4e5f6", "SSH from 192.168.1.20", 120, 40, th, false); strings.Contains(frame, dangerSGR) {
			t.Error("the modal paints in state.destructive")
		}
	})

	t.Run("on nothing for a session no longer listed", func(t *testing.T) {
		m := takenModel(t, Taken{Session: "gone-x9y8z7", By: "SSH from 192.168.1.20"}, takenSessions)

		if m.modal != modalNone || m.activePage != PageSessions {
			t.Errorf("modal = %v on page %v; want the plain picker", m.modal, m.activePage)
		}
	})

	t.Run("on nothing with no session taken", func(t *testing.T) {
		if m := takenModel(t, Taken{}, takenSessions); m.modal != modalNone {
			t.Errorf("modal = %v, want none", m.modal)
		}
	})
}

func TestTaken_Keys(t *testing.T) {
	t.Run("Enter takes the session back", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want the taken session picked", m.Selected(), quits(cmd))
		}
	})

	t.Run("Enter on a session not answering flashes why, as picking it would", func(t *testing.T) {
		sessions := []Session{{Name: "alpha-a1b2c3"}, {Name: "bravo-d4e5f6", NotAnswering: true}}
		m := takenModel(t, takenOverSSHAsk, sessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})

		assertFlashedAndOpen(t, m, cmd, "session bravo-d4e5f6 is not answering; nothing was opened", []string{"alpha-a1b2c3", "bravo-d4e5f6"})
	})

	t.Run("Esc goes to the picker, the cursor on the session's row", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if m.modal != modalNone || m.Selected() != "" || cmd != nil {
			t.Errorf("modal %v, selected %q, cmd %v; want the plain picker", m.modal, m.Selected(), cmd != nil)
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the taken session's row", got)
		}
	})

	for _, key := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: 'k', Text: "k"}, {Code: tea.KeyDown}} {
		t.Run(key.String()+" does nothing", func(t *testing.T) {
			m := takenModel(t, takenOverSSHAsk, takenSessions)

			m, cmd := pressKeys(m, key)

			if m.modal != modalTaken || m.Selected() != "" || cmd != nil {
				t.Errorf("modal %v, selected %q, cmd %v; want the key ignored", m.modal, m.Selected(), cmd != nil)
			}
		})
	}
}

func TestTaken_Watch(t *testing.T) {
	watching := func(t *testing.T) Model {
		t.Helper()
		m := takenModel(t, takenOverSSHAsk, takenSessions)
		m, read := pressKeys(m, keySpaceMsg())
		if m.activePage != pagePreview || m.modal != modalNone || m.preview.session != "bravo-d4e5f6" {
			t.Fatalf("page %v, modal %v, previewing %q; want the taken session's preview", m.activePage, m.modal, m.preview.session)
		}
		return landRead(t, m, read)
	}

	t.Run("Space shows the session read-only, as the preview does", func(t *testing.T) {
		m := watching(t)

		if body := lastBodyRow(m.preview); body != "the session's screen" {
			t.Errorf("body = %q, want the session's history", body)
		}
		if m.Selected() != "" {
			t.Errorf("selected %q, want nothing attached", m.Selected())
		}
	})

	t.Run("Esc from the watch goes back to the modal, the cursor on the session's row", func(t *testing.T) {
		m := dismissPreview(t, watching(t))

		if m.activePage != PageSessions || m.modal != modalTaken || m.takenSession.Name != "bravo-d4e5f6" || m.takenBy != takenOverSSHAsk.By {
			t.Fatalf("page %v, modal %v on %q by %q; want the taken modal again", m.activePage, m.modal, m.takenSession.Name, m.takenBy)
		}
		if got := selectedSessionName(t, m); got != "bravo-d4e5f6" {
			t.Errorf("cursor on %q, want the taken session's row", got)
		}

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) {
			t.Errorf("selected %q, quits %v; want Enter taking it back", m.Selected(), quits(cmd))
		}
	})

	t.Run("Esc from a watch stepped to another session goes back to the modal on the taken one", func(t *testing.T) {
		m, read := pressStep(t, watching(t))
		m = landRead(t, m, read)
		if m.preview.session == "bravo-d4e5f6" {
			t.Fatalf("stepped to %q, want another session", m.preview.session)
		}

		m = dismissPreview(t, m)

		if m.modal != modalTaken || selectedSessionName(t, m) != "bravo-d4e5f6" {
			t.Errorf("modal %v, cursor on %q; want the taken modal on the taken session", m.modal, selectedSessionName(t, m))
		}
	})

	t.Run("Enter from the watch takes the session back, done with the modal", func(t *testing.T) {
		m, open := pressKeys(watching(t), tea.KeyPressMsg{Code: tea.KeyEnter})
		m, cmd := land(t, m, open)

		if m.Selected() != "bravo-d4e5f6" || !quits(cmd) || m.watchingTaken || m.takenSession.Name != "" {
			t.Errorf("selected %q, quits %v, watching %v, taken %q; want the session picked and the modal done", m.Selected(), quits(cmd), m.watchingTaken, m.takenSession.Name)
		}
	})

	t.Run("Esc from the picker's own preview after the modal closed stays on the list", func(t *testing.T) {
		m, _ := pressKeys(takenModel(t, takenOverSSHAsk, takenSessions), tea.KeyPressMsg{Code: tea.KeyEscape})
		m = openPreviewOn(t, m)

		if m = dismissPreview(t, m); m.modal != modalNone {
			t.Errorf("modal = %v, want the plain picker", m.modal)
		}
	})
}

// The first listing and the projects are read at once, so either can land
// first.
func TestTaken_KeysTypedAhead(t *testing.T) {
	t.Run("before the rows land, a key is dropped, Ctrl+C aside", func(t *testing.T) {
		m := takenPicker(t, takenOverSSHAsk, takenSessions)

		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'q', Text: "q"}, {Code: tea.KeyEnter}, keySpaceMsg()} {
			var cmd tea.Cmd
			if m, cmd = pressKeys(m, key); cmd != nil || m.Selected() != "" {
				t.Errorf("%v: cmd %v, selected %q; want the key dropped", key, cmd != nil, m.Selected())
			}
		}
		if _, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); !quits(cmd) {
			t.Error("Ctrl+C did not quit")
		}

		m = deliver(t, m, SessionsMsg{Sessions: takenSessions})
		if m.modal != modalTaken || selectedSessionName(t, m) != "bravo-d4e5f6" {
			t.Errorf("modal %v, cursor on %q; want the modal on the taken session, no key typed ahead acting", m.modal, selectedSessionName(t, m))
		}
	})

	t.Run("a session not listed lets keys act again", func(t *testing.T) {
		m := deliver(t, takenPicker(t, Taken{Session: "gone-x9y8z7"}, takenSessions), SessionsMsg{Sessions: takenSessions})

		if _, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'q', Text: "q"}); !quits(cmd) {
			t.Error("q did not quit the plain picker")
		}
	})
}
