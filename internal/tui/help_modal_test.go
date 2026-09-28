package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func helpModelSessions(t *testing.T, appearance theme.Member) Model {
	t.Helper()
	m := New(fakeLister{}, WithThemeNomination(testBuiltinPair(t)), WithCanvasMode(appearance))
	m.termWidth = 90
	m.termHeight = 30
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
	}, nil)
	return m
}

func helpModelProjects(t *testing.T, appearance theme.Member) Model {
	t.Helper()
	projects := []project.Project{
		{Path: "/p/one", Name: "one"},
	}
	m := New(fakeLister{}, WithThemeNomination(testBuiltinPair(t)), WithCanvasMode(appearance))
	m.termWidth = 90
	m.termHeight = 30
	m.activePage = PageProjects
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	return m
}

func TestHelpModalOpen(t *testing.T) {
	t.Run("it opens the help modal on ? for Sessions", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalHelp {
			t.Errorf("? must open the help modal; modal = %v, want modalHelp", m.modal)
		}
		if m.activePage != PageSessions {
			t.Errorf("? must not change the active page; got %d", m.activePage)
		}
	})

	t.Run("it opens the help modal on ? for Projects", func(t *testing.T) {
		m := helpModelProjects(t, theme.MemberDark)
		updated, _ := m.updateProjectsPage(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalHelp {
			t.Errorf("? must open the help modal; modal = %v, want modalHelp", m.modal)
		}
		if m.activePage != PageProjects {
			t.Errorf("? must not change the active page; got %d", m.activePage)
		}
	})

	t.Run("it no longer lets bubbles/list self-toggle its own help on ?", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		before := m.sessionList.ShowHelp()
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.sessionList.ShowHelp() != before {
			t.Errorf("? must not toggle the list's own help; ShowHelp %v → %v", before, m.sessionList.ShowHelp())
		}
		if m.modal != modalHelp {
			t.Errorf("? must open our help modal instead; modal = %v", m.modal)
		}
	})
}

func TestHelpModalClose(t *testing.T) {
	t.Run("it closes on ? toggle", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("? must toggle the open help modal closed; modal = %v, want modalNone", m.modal)
		}
	})

	t.Run("it closes on Esc", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("Esc must dismiss the help modal; modal = %v, want modalNone", m.modal)
		}
		if cmd != nil {
			t.Errorf("Esc on the help modal must not fall through to quit; got a non-nil cmd")
		}
	})

	t.Run("it does not fall through to clear-filter on Esc (key-exclusive)", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m2, _ := m.updateSessionList(tea.KeyPressMsg{Code: '/', Text: "/"})
		m = m2.(Model)
		m2, _ = m.updateSessionList(tea.KeyPressMsg{Code: 'a', Text: "a"})
		m = m2.(Model)
		m2, _ = m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = m2.(Model)
		filterBefore := m.sessionList.FilterState()
		m.modal = modalHelp
		m2, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = m2.(Model)
		if m.modal != modalNone {
			t.Errorf("Esc must dismiss the help modal; modal = %v", m.modal)
		}
		if cmd != nil {
			t.Errorf("Esc on help must not produce a cmd (no quit/clear fall-through); got non-nil")
		}
		if m.sessionList.FilterState() != filterBefore {
			t.Errorf("Esc on help must NOT clear the filter (key-exclusive); filter %v → %v", filterBefore, m.sessionList.FilterState())
		}
	})

	t.Run("it does not fall through to quit on Esc (key-exclusive, no filter)", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd != nil {
			t.Errorf("Esc on help with no filter must NOT quit; got a non-nil cmd")
		}
	})

	t.Run("it consumes all other keys while open (key-exclusive)", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'q', Text: "q"})
		m = updated.(Model)
		if m.modal != modalHelp {
			t.Errorf("a non-dismiss key must keep the help modal open; modal = %v", m.modal)
		}
		if cmd != nil {
			t.Errorf("q while help is open must be consumed (no quit); got a non-nil cmd")
		}
	})

	t.Run("it closes on ? toggle from Projects too", func(t *testing.T) {
		m := helpModelProjects(t, theme.MemberDark)
		m.modal = modalHelp
		updated, _ := m.updateProjectsPage(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("? must toggle the open help modal closed on Projects; modal = %v", m.modal)
		}
	})

	t.Run("it closes on Esc from Projects without falling through", func(t *testing.T) {
		m := helpModelProjects(t, theme.MemberDark)
		m.modal = modalHelp
		updated, cmd := m.updateProjectsPage(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("Esc must dismiss the help modal on Projects; modal = %v", m.modal)
		}
		if cmd != nil {
			t.Errorf("Esc on Projects help must not fall through to quit; got a non-nil cmd")
		}
	})
}

func TestHelpModalContent(t *testing.T) {
	t.Run("it generates the Sessions help content from the descriptor including footer-core keys", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewSessionList()

		for _, action := range []string{
			"Move selection",
			"Open / attach session",
			"Filter sessions",
			"Preview scrollback",
			"Switch view",
			"Switch to Projects",
			"New session in cwd",
			"Rename session",
			"Kill session",
			"Multi-select mode",
			"Theme picker",
			"Quit",
		} {
			if !strings.Contains(view, action) {
				t.Errorf("Sessions help must list %q (complete keymap from descriptor), missing in:\n%s", action, view)
			}
		}
	})

	t.Run("it includes a footer-core key in the Sessions help (complete keymap)", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewSessionList()
		if !strings.Contains(view, "Preview scrollback") {
			t.Errorf("Sessions help must include the footer-core space/preview key; missing in:\n%s", view)
		}
		if !strings.Contains(view, "␣") {
			t.Errorf("Sessions help must show the ␣ glyph for the footer-core preview key; missing in:\n%s", view)
		}
	})

	t.Run("it generates the Projects help content from the descriptor including help-only keys", func(t *testing.T) {
		m := helpModelProjects(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewProjectList()
		for _, action := range []string{
			"New session",
			"Switch to Sessions",
			"Edit project",
			"Filter projects",
			"Delete project",
			"New session in cwd",
			"Move selection",
			"Theme picker",
			"Quit",
		} {
			if !strings.Contains(view, action) {
				t.Errorf("Projects help must list %q (complete keymap from descriptor), missing in:\n%s", action, view)
			}
		}
	})

	t.Run("it does not list the ? help self-entry in the modal body", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		body := helpModalBody(sessionsKeymap(), m.themeState.active, m.colourless)
		if strings.Contains(body, "help") {
			t.Errorf("help modal body must not list the ? help self-entry; got:\n%s", body)
		}
	})
}

func TestHelpModalGlyphs(t *testing.T) {
	body := helpModalBody(sessionsKeymap(), testDarkTheme(t), false)
	for _, glyph := range []string{"^↑/↓", "␣", "⏎", "↑/↓"} {
		if !strings.Contains(body, glyph) {
			t.Errorf("help body must show glyph %q; missing in:\n%s", glyph, body)
		}
	}
	if strings.Contains(body, "ctrl+") {
		t.Errorf("help body must use the caret form (^↑/↓), not ctrl+; got:\n%s", body)
	}

	entries := sessionsKeymap()
	keyByGlyph := map[string]bool{}
	for _, e := range entries {
		keyByGlyph[e.Key] = true
	}
	for _, footerKey := range []string{"␣", "⏎", "↑↓"} {
		if !keyByGlyph[footerKey] {
			t.Errorf("footer Key form %q must remain in the descriptor", footerKey)
		}
	}
}

func TestHelpModalHeader(t *testing.T) {
	t.Run("it renders the header ? Keybindings left and esc close right", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewSessionList()
		if !strings.Contains(view, "Keybindings") {
			t.Errorf("help modal header must read 'Keybindings'; missing in:\n%s", view)
		}
		if !strings.Contains(view, "esc close") {
			t.Errorf("help modal header must carry the right-aligned 'esc close' dismiss hint; missing in:\n%s", view)
		}
	})

	t.Run("it has no contextual footer dismiss hint", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewSessionList()
		for _, footerVerb := range []string{"esc cancel", "esc discard"} {
			if strings.Contains(view, footerVerb) {
				t.Errorf("help modal must not carry a contextual footer dismiss hint %q; got:\n%s", footerVerb, view)
			}
		}
	})
}

func TestHelpModalColourRoles(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		body := helpModalBody(sessionsKeymap(), th, false)

		wantTokens := map[string]theme.Token{
			"accent.key (key glyphs)":        th.AccentKey,
			"text.secondary (action labels)": th.TextSecondary,
		}
		for label, tok := range wantTokens {
			seq := tokenFgSeq(t, tok)
			if !strings.Contains(body, seq) {
				t.Errorf("help body must render %s via token SGR core %q; missing in:\n%s", label, seq, body)
			}
		}

		header := helpModalHeader(60, th, false)
		if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(header, seq) {
			t.Errorf("help header must render '? Keybindings' in text.primary SGR core %q; missing in:\n%s", seq, header)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(header, seq) {
			t.Errorf("help header must render 'esc close' in text.muted SGR core %q; missing in:\n%s", seq, header)
		}
	})
}

func TestHelpModalDestructiveKill(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		body := helpModalBody(sessionsKeymap(), th, false)
		if seq := tokenFgSeq(t, th.StateDestructive); !strings.Contains(body, seq) {
			t.Errorf("theme %v: the kill (k) glyph must render in state.destructive; missing SGR core %q in:\n%s", themeLabel(th), seq, body)
		}
	}
}

func TestHelpModalBlankCanvas(t *testing.T) {
	forEachCanvasMode(t, func(t *testing.T, mode theme.Member) {
		m := helpModelSessions(t, mode)
		if !strings.Contains(m.viewSessionList(), "alpha") {
			t.Fatalf("pre-modal sanity: live view should contain a session row")
		}
		m.modal = modalHelp
		frame := m.View().Content
		if strings.Contains(frame, "bravo") {
			t.Errorf("help modal must clear the list rows behind it; 'bravo' leaked:\n%s", frame)
		}
		if seq := canvasSeq(t, themeForAppearance(t, mode)); !strings.Contains(frame, seq) {
			t.Errorf("help modal backdrop must be the owned canvas SGR %q; missing", seq)
		}
	})
}
