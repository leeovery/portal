package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	t.Run("titled, naming the session, saying where it went, and offering its three keys", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		view := ansi.Strip(m.View().Content)
		want := []string{"⇄ Open on another device", "release checklist", "", "Taken by SSH from 192.168.1.20.", "⏎ take it back   space watch   esc picker"}
		if rows := modalRows(view); !slices.Equal(rows, want) {
			t.Errorf("the modal reads %q, want %q", rows, want)
		}
		if strings.Contains(view, "alpha-a1b2c3") {
			t.Errorf("the frame shows the session list behind the modal:\n%s", view)
		}
	})

	t.Run("naming the session alone when where it went is unknown", func(t *testing.T) {
		m := takenModel(t, Taken{Session: "alpha-a1b2c3"}, takenSessions)

		rows := modalRows(ansi.Strip(m.View().Content))
		want := []string{"⇄ Open on another device", "alpha-a1b2c3", "⏎ take it back   space watch   esc picker"}
		if !slices.Equal(rows, want) {
			t.Errorf("the modal reads %q, want %q", rows, want)
		}
	})

	t.Run("titled as the help modal is, the session in the accent, bold, where it went muted, and nothing in a danger colour", func(t *testing.T) {
		th := testDarkTheme(t)
		danger := headerStyle(th.StateDestructive, th, false).Render("x")

		frame := renderTakenModalOnClearedCanvas("bravo-d4e5f6", "SSH from 192.168.1.20", false, 120, 40, th, false)

		if strings.Contains(frame, danger[:strings.Index(danger, "x")]) {
			t.Error("the modal paints in state.destructive")
		}
		if title := modalTitle(takenTitleGlyph, takenTitle, th, false); !strings.Contains(frame, title) {
			t.Error("the title is not drawn as the help modal draws its own")
		}
		if name := headerStyle(th.AccentPrimary, th, false).Bold(true).Render("bravo-d4e5f6"); !strings.Contains(frame, name) {
			t.Error("the session's name is not bold in accent.primary")
		}
		if by := headerStyle(th.TextMuted, th, false).Render("Taken by SSH from 192.168.1.20."); !strings.Contains(frame, by) {
			t.Error("where the session went is not in text.muted")
		}
	})

	t.Run("under NO_COLOR, the title's glyph and bold still carry it", func(t *testing.T) {
		frame := renderTakenModalOnClearedCanvas("bravo-d4e5f6", "", false, 120, 40, testDarkTheme(t), true)

		bold := lipgloss.NewStyle().Bold(true)
		for _, want := range []string{bold.Render("⇄"), bold.Render("Open on another device"), bold.Render("bravo-d4e5f6")} {
			if !strings.Contains(frame, want) {
				t.Errorf("the colourless frame holds no %q", want)
			}
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

// modalRows is the text of each row inside the modal frame shows, its
// dividers left out.
func modalRows(frame string) []string {
	var rows []string
	for line := range strings.SplitSeq(frame, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "│") {
			continue
		}
		rows = append(rows, strings.TrimSpace(strings.Trim(line, "│")))
	}
	return rows
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

	t.Run("Esc goes to the picker, read again, the cursor on the session's row", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if m.modal != modalNone || m.Selected() != "" || quits(cmd) {
			t.Errorf("modal %v, selected %q, quits %v; want the plain picker", m.modal, m.Selected(), quits(cmd))
		}
		m = landRefresh(t, m, cmd, "bravo-d4e5f6")
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

// takenReader is a SessionReader answering every read alike, counting them.
type takenReader struct {
	session Session
	found   bool
	err     error
	reads   int
}

func (r *takenReader) ReadSession(name string) (Session, bool, error) {
	r.reads++
	if r.found && r.session.Name == "" {
		return Session{Name: name}, true, r.err
	}
	return r.session, r.found, r.err
}

// watchedTakenModel is takenModel listing through lister, its modal reading
// its session through reader. The watch's ticks are delivered by hand, so its
// gap is long enough never to pass.
func watchedTakenModel(t *testing.T, taken Taken, lister *rowActionLister, reader SessionReader) Model {
	t.Helper()
	m := Build(Deps{
		Lister:        lister,
		Theme:         theme.ConstantNomination(testDarkTheme(t)),
		History:       historyFor(t, "the session's screen", lister.sessions),
		Taken:         taken,
		SessionReader: reader,
		TakenRecheck:  time.Hour,
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = deliver(t, updated.(Model), SessionsMsg{Sessions: lister.sessions})
	return deliver(t, m, ProjectsLoadedMsg{})
}

// recheckTaken delivers the watch's tick and lands the read it starts,
// returning what landing it returned.
func recheckTaken(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, read := m.Update(takenWatchTickMsg{gen: m.takenWatch.gen})
	if read == nil {
		t.Fatal("the watch's tick started no read")
	}
	updated, next := updated.(Model).Update(read())
	return updated.(Model), next
}

// landRefresh runs cmd, which must read the list again re-anchoring on
// preserve, and delivers what it reads.
func landRefresh(t *testing.T, m Model, cmd tea.Cmd, preserve string) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("want the list read again, got no command")
	}
	msg, ok := cmd().(sessionsRefreshedMsg)
	if !ok {
		t.Fatalf("the command read %T, want the list read again", msg)
	}
	if msg.PreserveName != preserve {
		t.Errorf("the read re-anchors on %q, want %q", msg.PreserveName, preserve)
	}
	return deliver(t, m, msg)
}

// endedTakenModel is the taken modal turned to say its session has ended,
// the session gone from what lister lists.
func endedTakenModel(t *testing.T, taken Taken) (Model, *rowActionLister) {
	t.Helper()
	lister := &rowActionLister{sessions: slices.Clone(takenSessions)}
	m := watchedTakenModel(t, taken, lister, &takenReader{})
	lister.sessions = []Session{takenSessions[0], takenSessions[2]}
	m, _ = recheckTaken(t, m)
	if m.modal != modalTaken || !m.takenEnded {
		t.Fatalf("modal %v, ended %v; want the taken modal saying the session has ended", m.modal, m.takenEnded)
	}
	return m, lister
}

func TestTaken_Watch_Reads(t *testing.T) {
	t.Run("a session read gone turns the modal to say it has ended, and where it had gone", func(t *testing.T) {
		m, _ := endedTakenModel(t, takenOverSSHAsk)

		want := []string{"✕ Session ended", "release checklist", "", "Ended after it was taken by SSH from 192.168.1.20.", "⏎ picker   q quit"}
		if rows := modalRows(ansi.Strip(m.View().Content)); !slices.Equal(rows, want) {
			t.Errorf("the modal reads %q, want %q", rows, want)
		}
	})

	t.Run("saying only that it has ended when where it went is unknown", func(t *testing.T) {
		m, _ := endedTakenModel(t, Taken{Session: "bravo-d4e5f6"})

		want := []string{"✕ Session ended", "release checklist", "", "It has ended.", "⏎ picker   q quit"}
		if rows := modalRows(ansi.Strip(m.View().Content)); !slices.Equal(rows, want) {
			t.Errorf("the modal reads %q, want %q", rows, want)
		}
	})

	t.Run("an ended session is read no more", func(t *testing.T) {
		reader := &takenReader{}
		m := watchedTakenModel(t, takenOverSSHAsk, &rowActionLister{sessions: takenSessions}, reader)

		m, next := recheckTaken(t, m)

		if next != nil {
			t.Error("the watch goes on reading a session that has ended")
		}
		if _, read := m.Update(takenWatchTickMsg{gen: m.takenWatch.gen}); read != nil {
			t.Error("a tick still reads a session that has ended")
		}
	})

	t.Run("a session read live shows as read, its label included, and is read again", func(t *testing.T) {
		reader := &takenReader{session: Session{Name: "bravo-d4e5f6", Label: "shipped checklist"}, found: true}
		m := watchedTakenModel(t, takenOverSSHAsk, &rowActionLister{sessions: takenSessions}, reader)

		m, next := recheckTaken(t, m)

		if m.takenEnded || next == nil {
			t.Fatalf("ended %v, reads again %v; want the session live and read again", m.takenEnded, next != nil)
		}
		if rows := modalRows(ansi.Strip(m.View().Content)); !slices.Contains(rows, "shipped checklist") {
			t.Errorf("the modal reads %q, want the label as read", rows)
		}
	})

	t.Run("a read that tells neither changes nothing, and is read again", func(t *testing.T) {
		reader := &takenReader{err: errors.New("session bravo-d4e5f6 is not answering")}
		m := watchedTakenModel(t, takenOverSSHAsk, &rowActionLister{sessions: takenSessions}, reader)

		m, next := recheckTaken(t, m)

		if m.takenEnded || m.takenSession != takenSessions[1] || next == nil {
			t.Errorf("ended %v on %+v, reads again %v; want the modal as it was, read again", m.takenEnded, m.takenSession, next != nil)
		}
	})

	t.Run("a tick from an earlier watch, or after the modal closed, reads nothing", func(t *testing.T) {
		reader := &takenReader{found: true}
		m := watchedTakenModel(t, takenOverSSHAsk, &rowActionLister{sessions: takenSessions}, reader)

		if _, read := m.Update(takenWatchTickMsg{gen: m.takenWatch.gen - 1}); read != nil {
			t.Error("an earlier watch's tick read the session")
		}
		m, _ = pressKeys(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if _, read := m.Update(takenWatchTickMsg{gen: m.takenWatch.gen}); read != nil {
			t.Error("a tick after the modal closed read the session")
		}
	})

	t.Run("with no reader, nothing is read", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		if _, read := m.Update(takenWatchTickMsg{gen: m.takenWatch.gen}); read != nil {
			t.Error("a picker with no reader read the session")
		}
	})
}

func TestTaken_Watch_Listings(t *testing.T) {
	t.Run("a listing without the session turns the modal to say it has ended", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)

		m = deliver(t, m, SessionsMsg{Sessions: []Session{takenSessions[0], takenSessions[2]}})

		if m.modal != modalTaken || !m.takenEnded {
			t.Errorf("modal %v, ended %v; want the modal saying the session has ended", m.modal, m.takenEnded)
		}
	})

	t.Run("a listing with the session shows it as listed", func(t *testing.T) {
		m := takenModel(t, takenOverSSHAsk, takenSessions)
		relabelled := Session{Name: "bravo-d4e5f6", Label: "shipped checklist"}

		m = deliver(t, m, SessionsMsg{Sessions: []Session{takenSessions[0], relabelled, takenSessions[2]}})

		if m.takenEnded || m.takenSession != relabelled {
			t.Errorf("ended %v on %+v; want the session as listed", m.takenEnded, m.takenSession)
		}
	})
}

func TestTaken_Watch_WhileWatching(t *testing.T) {
	t.Run("the session ending closes the watch onto the modal saying so", func(t *testing.T) {
		lister := &rowActionLister{sessions: slices.Clone(takenSessions)}
		m := watchedTakenModel(t, takenOverSSHAsk, lister, &takenReader{})
		m, read := pressKeys(m, keySpaceMsg())
		m = landRead(t, m, read)
		lister.sessions = []Session{takenSessions[0], takenSessions[2]}

		m, refresh := recheckTaken(t, m)

		if m.activePage != PageSessions || m.modal != modalTaken || !m.takenEnded || m.watchingTaken {
			t.Fatalf("page %v, modal %v, ended %v, watching %v; want the modal saying the session has ended", m.activePage, m.modal, m.takenEnded, m.watchingTaken)
		}
		m = landRefresh(t, m, refresh, "")
		if m.modal != modalTaken || !m.takenEnded {
			t.Errorf("modal %v, ended %v once the list was read again; want the ended modal still", m.modal, m.takenEnded)
		}
	})
}

func TestTaken_Ended_Keys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: tea.KeyEscape}} {
		t.Run(key.String()+" goes to the picker, read again, the ended session gone", func(t *testing.T) {
			m, _ := endedTakenModel(t, takenOverSSHAsk)

			m, cmd := pressKeys(m, key)

			if m.modal != modalNone || m.Selected() != "" || quits(cmd) {
				t.Fatalf("modal %v, selected %q, quits %v; want the plain picker", m.modal, m.Selected(), quits(cmd))
			}
			m = landRefresh(t, m, cmd, "")
			for _, s := range m.sessions {
				if s.Name == "bravo-d4e5f6" {
					t.Error("the picker still lists the ended session")
				}
			}
			if m.takenEnded || m.takenSession.Name != "" {
				t.Errorf("ended %v on %q; want the modal done with", m.takenEnded, m.takenSession.Name)
			}
		})
	}

	t.Run("q quits, picking nothing", func(t *testing.T) {
		m, _ := endedTakenModel(t, takenOverSSHAsk)

		m, cmd := pressKeys(m, tea.KeyPressMsg{Code: 'q', Text: "q"})

		if !quits(cmd) || m.Selected() != "" {
			t.Errorf("quits %v, selected %q; want a quit picking nothing", quits(cmd), m.Selected())
		}
	})

	for _, key := range []tea.KeyPressMsg{keySpaceMsg(), {Code: 'k', Text: "k"}, {Code: 'y', Text: "y"}, {Code: tea.KeyDown}} {
		t.Run(key.String()+" does nothing", func(t *testing.T) {
			m, _ := endedTakenModel(t, takenOverSSHAsk)

			m, cmd := pressKeys(m, key)

			if m.modal != modalTaken || !m.takenEnded || m.activePage != PageSessions || cmd != nil {
				t.Errorf("modal %v, ended %v, page %v, cmd %v; want the key ignored", m.modal, m.takenEnded, m.activePage, cmd != nil)
			}
		})
	}
}
