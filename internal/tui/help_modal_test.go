package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// helpModelSessions builds a Sessions-page Model on a sized canvas for exercising
// the ? help modal open/close dispatch and the render shell.
func helpModelSessions(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	m := New(fakeLister{}, WithCanvasMode(mode))
	m.termWidth = 90
	m.termHeight = 30
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
	})
	return m
}

// helpModelProjects builds a Projects-page Model on a sized canvas for exercising
// the ? help modal open/close dispatch and the render shell.
func helpModelProjects(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	projects := []project.Project{
		{Path: "/p/one", Name: "one"},
	}
	m := New(fakeLister{}, WithCanvasMode(mode))
	m.termWidth = 90
	m.termHeight = 30
	m.activePage = PageProjects
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	return m
}

// TestHelpModalOpen asserts ? opens the help modal on Sessions and Projects,
// replacing the prior swallow. The key is still consumed (no page change, no
// fall-through to the list's own help toggle).
func TestHelpModalOpen(t *testing.T) {
	t.Run("it opens the help modal on ? for Sessions", func(t *testing.T) {
		m := helpModelSessions(t, theme.Dark)
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
		m := helpModelProjects(t, theme.Dark)
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
		// Pre-swallow, ? returned nil and the list never saw it. Now ? opens OUR
		// modal — still consuming the key so the list's help is never toggled. The
		// observable proof is that the list's built-in ShowHelp state is unchanged
		// (still hidden) and OUR modal opened instead.
		m := helpModelSessions(t, theme.Dark)
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

// TestHelpModalClose asserts the help modal is key-exclusive (§8.1): ? toggles it
// closed, Esc dismisses it, and Esc does NOT fall through to the page's
// clear-filter / quit while the modal is open.
func TestHelpModalClose(t *testing.T) {
	t.Run("it closes on ? toggle", func(t *testing.T) {
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("? must toggle the open help modal closed; modal = %v, want modalNone", m.modal)
		}
	})

	t.Run("it closes on Esc", func(t *testing.T) {
		m := helpModelSessions(t, theme.Dark)
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
		// Edge case (§8.1): help open + a filter applied. Esc dismisses the help
		// modal ONLY — the filter stays applied.
		m := helpModelSessions(t, theme.Dark)
		// Apply a filter: focus the input, type a query, commit to list-active.
		m2, _ := m.updateSessionList(tea.KeyPressMsg{Code: '/', Text: "/"})
		m = m2.(Model)
		m2, _ = m.updateSessionList(tea.KeyPressMsg{Code: 'a', Text: "a"})
		m = m2.(Model)
		m2, _ = m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = m2.(Model)
		filterBefore := m.sessionList.FilterState()
		// Open help over the filtered list, then Esc.
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
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd != nil {
			t.Errorf("Esc on help with no filter must NOT quit; got a non-nil cmd")
		}
	})

	t.Run("it consumes all other keys while open (key-exclusive)", func(t *testing.T) {
		// A non-dismiss key (q, which would otherwise quit) must be swallowed.
		m := helpModelSessions(t, theme.Dark)
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
		m := helpModelProjects(t, theme.Dark)
		m.modal = modalHelp
		updated, _ := m.updateProjectsPage(tea.KeyPressMsg{Code: '?', Text: "?"})
		m = updated.(Model)
		if m.modal != modalNone {
			t.Errorf("? must toggle the open help modal closed on Projects; modal = %v", m.modal)
		}
	})

	t.Run("it closes on Esc from Projects without falling through", func(t *testing.T) {
		m := helpModelProjects(t, theme.Dark)
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

// TestHelpModalContent asserts the help modal is GENERATED from the page keymap
// descriptor (Sessions / Projects) and lists the page's COMPLETE keymap,
// including footer-core keys (the full reference, not just the footer overflow).
func TestHelpModalContent(t *testing.T) {
	t.Run("it generates the Sessions help content from the descriptor including footer-core keys", func(t *testing.T) {
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		view := m.viewSessionList()

		// Every Sessions descriptor entry's help action must appear — footer-core
		// AND help-only. The ? help self-entry is excluded (a help modal does not
		// list its own open key).
		for _, action := range []string{
			"Move selection",        // ↑/↓ — footer-core
			"Open / attach session", // enter — footer-core
			"Filter sessions",       // / — footer-core
			"Preview scrollback",    // space — footer-core
			"Switch view",           // s — footer-core
			"Switch to Projects",    // x — footer-core
			"New session in cwd",    // n — help-only
			"Rename session",        // r — help-only
			"Kill session",          // k — help-only
			"Quit",                  // q — help-only
		} {
			if !strings.Contains(view, action) {
				t.Errorf("Sessions help must list %q (complete keymap from descriptor), missing in:\n%s", action, view)
			}
		}
	})

	t.Run("it includes a footer-core key in the Sessions help (complete keymap)", func(t *testing.T) {
		// Edge case: the complete keymap must include keys ALSO in the footer.
		// space/preview is a footer-core key; it must still appear in help.
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		view := m.viewSessionList()
		if !strings.Contains(view, "Preview scrollback") {
			t.Errorf("Sessions help must include the footer-core space/preview key; missing in:\n%s", view)
		}
		// The help body shows the ␣ glyph for the preview key (HelpKey), matching the
		// footer Key (now also ␣ per §3.4 — task 8-2).
		if !strings.Contains(view, "␣") {
			t.Errorf("Sessions help must show the ␣ glyph for the footer-core preview key; missing in:\n%s", view)
		}
	})

	t.Run("it generates the Projects help content from the descriptor including help-only keys", func(t *testing.T) {
		m := helpModelProjects(t, theme.Dark)
		m.modal = modalHelp
		view := m.viewProjectList()
		// Help-only Projects keys (d/n/nav/q) must appear, plus the footer-core set.
		for _, action := range []string{
			"New session",        // ⏎ — footer-core
			"Switch to Sessions", // x — footer-core
			"Edit project",       // e — footer-core
			"Filter projects",    // / — footer-core
			"Delete project",     // d — help-only
			"New session in cwd", // n — help-only
			"Move selection",     // ↑/↓ — help-only
			"Quit",               // q — help-only
		} {
			if !strings.Contains(view, action) {
				t.Errorf("Projects help must list %q (complete keymap from descriptor), missing in:\n%s", action, view)
			}
		}
	})

	t.Run("it does not list the ? help self-entry in the modal body", func(t *testing.T) {
		// The reference omits the ? open key from the body (the panel's own header
		// carries the dismiss hint instead).
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		body := helpModalBody(sessionsKeymap(), m.canvasMode, m.colourless)
		if strings.Contains(body, "help") {
			t.Errorf("help modal body must not list the ? help self-entry; got:\n%s", body)
		}
	})
}

// TestHelpModalGlyphs asserts the "all symbols, caret for ctrl" final glyph set
// in the help body: page → `^↑/↓`, space → `␣`, enter → `⏎`, nav → `↑/↓` (the
// slashed help-only forms via HelpKey). The footer reads the §3.4 glyph Key forms
// `␣` / `⏎` / `↑↓` (no slash) and never shows the page `^↑/↓` (help-only).
func TestHelpModalGlyphs(t *testing.T) {
	body := helpModalBody(sessionsKeymap(), theme.Dark, false)
	for _, glyph := range []string{"^↑/↓", "␣", "⏎", "↑/↓"} {
		if !strings.Contains(body, glyph) {
			t.Errorf("help body must show glyph %q; missing in:\n%s", glyph, body)
		}
	}
	// The help body must NOT regress to the literal words the footer used to use
	// for the overridden keys.
	if strings.Contains(body, "ctrl+") {
		t.Errorf("help body must use the caret form (^↑/↓), not ctrl+; got:\n%s", body)
	}

	// Footer reads Key: per §3.4 the Core forms are the glyphs ␣ / ⏎ / ↑↓ (no
	// slash), and the help-only page key (^↑/↓) is never in the footer.
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

// TestHelpModalHeader asserts the header reads `? Keybindings` left with a
// right-aligned `esc close`, and that there is NO contextual footer (the §8.1
// help-modal exception — dismiss hint in the header).
func TestHelpModalHeader(t *testing.T) {
	t.Run("it renders the header ? Keybindings left and esc close right", func(t *testing.T) {
		m := helpModelSessions(t, theme.Dark)
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
		// The §8.1 exception: the dismiss hint lives in the header, NOT a contextual
		// footer. The other modals render `esc cancel`/`esc discard` in a footer; the
		// help modal must not — its only dismiss copy is the header `esc close`.
		m := helpModelSessions(t, theme.Dark)
		m.modal = modalHelp
		view := m.viewSessionList()
		for _, footerVerb := range []string{"esc cancel", "esc discard"} {
			if strings.Contains(view, footerVerb) {
				t.Errorf("help modal must not carry a contextual footer dismiss hint %q (§8.1 exception); got:\n%s", footerVerb, view)
			}
		}
	})
}

// TestHelpModalColourRoles asserts the two-column colour wiring (§2.9): key-hint
// glyphs in accent.blue, action labels in text.strong, the header `? Keybindings`
// in text.primary, and `esc close` in text.detail. Verified by the presence of
// each token's mode-resolved SGR in the rendered body.
func TestHelpModalColourRoles(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := helpModalBody(sessionsKeymap(), tc.mode, false)

			wantTokens := map[string]theme.Token{
				"accent.blue (key glyphs)":    theme.MV.AccentBlue,
				"text.strong (action labels)": theme.MV.TextStrong,
			}
			for label, tok := range wantTokens {
				seq := tokenFgSeq(t, tok, tc.mode)
				if !strings.Contains(body, seq) {
					t.Errorf("help body must render %s via token SGR core %q; missing in:\n%s", label, seq, body)
				}
			}

			header := helpModalHeader(60, tc.mode, false)
			if seq := tokenFgSeq(t, theme.MV.TextPrimary, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("help header must render '? Keybindings' in text.primary SGR core %q; missing in:\n%s", seq, header)
			}
			if seq := tokenFgSeq(t, theme.MV.TextDetail, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("help header must render 'esc close' in text.detail SGR core %q; missing in:\n%s", seq, header)
			}
		})
	}
}

// TestHelpModalDestructiveKill asserts the kill (k) key glyph renders in the
// state.red destructive role per the reference (the k glyph is the one destructive
// glyph in the Sessions help). Glyph + colour, not colour alone (§2.2).
func TestHelpModalDestructiveKill(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		body := helpModalBody(sessionsKeymap(), mode, false)
		if seq := tokenFgSeq(t, theme.MV.StateRed, mode); !strings.Contains(body, seq) {
			t.Errorf("mode %v: the kill (k) glyph must render in state.red; missing SGR core %q in:\n%s", mode, seq, body)
		}
	}
}

// TestHelpModalBlankCanvas asserts the help modal renders on the cleared owned
// canvas (inherits the 3-1 blank-screen modal layer): the session rows behind it
// are gone and the owned-canvas backdrop SGR is present (dark + light).
func TestHelpModalBlankCanvas(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := helpModelSessions(t, tc.mode)
			// Sanity: the live view shows the list rows.
			if !strings.Contains(m.viewSessionList(), "alpha") {
				t.Fatalf("pre-modal sanity: live view should contain a session row")
			}
			m.modal = modalHelp
			frame := m.View().Content
			if strings.Contains(frame, "bravo") {
				t.Errorf("help modal must clear the list rows behind it; 'bravo' leaked:\n%s", frame)
			}
			if seq := canvasSeq(t, tc.mode); !strings.Contains(frame, seq) {
				t.Errorf("help modal backdrop must be the owned canvas SGR %q; missing", seq)
			}
		})
	}
}
