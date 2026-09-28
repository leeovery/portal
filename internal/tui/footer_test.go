package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

const referenceFooterWidth = 120

func footerVisible(s string) string {
	return ansi.Strip(s)
}

func TestSessionsFooter_SingleRowCoreKeysWithRightAlignedHelp(t *testing.T) {
	footer := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false)
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("condensed footer must be 2 rows (rule + key row), got %d:\n%s", len(lines), footer)
	}
	keyRow := footerVisible(lines[1])

	for _, want := range []string{
		"⏎ attach", "/ filter", "␣ preview",
		"s switch view", "x projects", "t theme", "m multi", "? help",
	} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("key row missing Core entry %q:\n%s", want, keyRow)
		}
	}

	helpIdx := strings.Index(keyRow, "? help")
	projectsIdx := strings.Index(keyRow, "x projects")
	if helpIdx < 0 || projectsIdx < 0 {
		t.Fatalf("expected both 'x projects' and '? help' on the key row:\n%s", keyRow)
	}
	if helpIdx <= projectsIdx {
		t.Errorf("? help (idx %d) must be right of x projects (idx %d):\n%s", helpIdx, projectsIdx, keyRow)
	}

	if got := lipgloss.Width(lines[1]); got != referenceFooterWidth {
		t.Errorf("key row width = %d, want exactly %d (right-aligned to width)", got, referenceFooterWidth)
	}
	if !strings.HasSuffix(strings.TrimRight(keyRow, " "), "help") {
		t.Errorf("? help must be the trailing (right-most) entry on the row:\n%s", keyRow)
	}
}

func TestSessionsFooter_TokenColours(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		footer := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, th, false)

		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(footer, seq) {
			t.Errorf("footer missing accent.key key-glyph role sequence %q", seq)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(footer, seq) {
			t.Errorf("footer missing text.muted label role sequence %q", seq)
		}
		if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(footer, seq) {
			t.Errorf("footer missing accent.primary ? role sequence %q", seq)
		}
		if seq := tokenFgSeq(t, th.Border); !strings.Contains(footer, seq) {
			t.Errorf("footer missing the border rule role sequence %q", seq)
		}
	})
}

func TestSessionsFooter_HelpGlyphIsViolet(t *testing.T) {
	footer := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false)
	violet := tokenFgSeq(t, testDarkTheme(t).AccentPrimary)

	qIdx := strings.IndexByte(footer, '?')
	if qIdx < 0 {
		t.Fatalf("footer does not contain the ? glyph:\n%s", footer)
	}
	prefix := footer[:qIdx]
	lastEsc := strings.LastIndex(prefix, "\x1b[")
	if lastEsc < 0 {
		t.Fatalf("no SGR run precedes the ? glyph:\n%s", footer)
	}
	run := footer[lastEsc:qIdx]
	if !strings.Contains(run, violet) {
		t.Errorf("the ? glyph's SGR run %q is not accent.primary (%q)", run, violet)
	}
}

func TestSessionsFooter_OmitsHelpOnlyKeys(t *testing.T) {
	footer := footerVisible(renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false))
	for _, banned := range []string{"new in cwd", "rename", "kill", "quit", "page", "Ctrl"} {
		if strings.Contains(footer, banned) {
			t.Errorf("footer must NOT show the help-only entry %q (it lives in ? help):\n%s", banned, footer)
		}
	}
}

func TestSessionsFooter_SourcedFromKeymapDescriptor(t *testing.T) {
	footer := footerVisible(renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false))

	for _, e := range sessionsKeymap() {
		shown := strings.Contains(footer, e.Action)
		if e.Core && !shown {
			t.Errorf("Core descriptor entry %q (%s) missing from the footer:\n%s", e.Action, e.Key, footer)
		}
		if !e.Core && shown {
			t.Errorf("non-Core descriptor entry %q leaked into the footer (help-only):\n%s", e.Action, footer)
		}
	}

	core, right := splitFooterEntries(sessionsKeymap())
	wantCore := []string{"attach", "filter", "preview", "switch view", "projects", "theme", "multi"}
	if len(core) != len(wantCore) {
		t.Fatalf("left-cluster Core entries = %d, want %d", len(core), len(wantCore))
	}
	for i, w := range wantCore {
		if core[i].Action != w {
			t.Errorf("left-cluster entry %d = %q, want %q (descriptor order)", i, core[i].Action, w)
		}
	}
	if right == nil || right.Action != "help" || !right.RightAligned {
		t.Errorf("right anchor = %+v, want the right-aligned ? help entry", right)
	}
}

func TestSessionsFooter_ColourlessDropsHueAndCanvas(t *testing.T) {
	footer := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), true)

	vis := footerVisible(footer)
	for _, want := range []string{"⏎ attach", "s switch view", "x projects", "t theme", "m multi", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("colourless footer missing %q:\n%s", want, vis)
		}
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(footer, seq) {
		t.Errorf("colourless footer still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{
		testDarkTheme(t).AccentKey, testDarkTheme(t).TextMuted, testDarkTheme(t).AccentPrimary, testDarkTheme(t).Border,
	} {
		if seq := tokenFgSeq(t, tok); strings.Contains(footer, seq) {
			t.Errorf("colourless footer still emits a foreground role sequence %q", seq)
		}
	}
}

func TestSessionsFooter_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, th, false)
		if seq := canvasSeq(t, th); !strings.Contains(footer, seq) {
			t.Errorf("footer does not paint the canvas background sequence %q:\n%s", seq, footer)
		}
	}
}

func TestSessionsFooter_NarrowTruncationNoWrap(t *testing.T) {
	for _, w := range []int{90, 76, 60, 40, 24, 12} {
		footer := renderSessionsFooter(sessionsKeymap(), w, testDarkTheme(t), false)
		lines := strings.Split(footer, "\n")
		if len(lines) != 2 {
			t.Errorf("at width %d the footer has %d rows, want 2 (rule + single key row, no wrap):\n%s", w, len(lines), footer)
			continue
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("at width %d, footer line %d width = %d (overflow)", w, i, lw)
			}
		}
		if w >= 24 {
			if !strings.Contains(footerVisible(lines[1]), "? help") {
				t.Errorf("at width %d the ? help right anchor was dropped too early:\n%s", w, footerVisible(lines[1]))
			}
		}
	}
}

func TestSessionsFooter_NarrowTruncationKeepsHighestPriority(t *testing.T) {
	footer := footerVisible(renderSessionsFooter(sessionsKeymap(), 60, testDarkTheme(t), false))
	if !strings.Contains(footer, "⏎ attach") {
		t.Errorf("highest-priority entry 'attach' must survive the degrade:\n%s", footer)
	}
	if !strings.Contains(footer, "…") {
		t.Errorf("a degraded footer must carry the ellipsis drop marker:\n%s", footer)
	}
	if !strings.Contains(footer, "? help") {
		t.Errorf("the ? help right anchor must survive at width 60:\n%s", footer)
	}
	for _, dropped := range []string{"m multi", "t theme", "x projects"} {
		if strings.Contains(footer, dropped) {
			t.Errorf("lowest-priority entry %q should have dropped first at width 60:\n%s", dropped, footer)
		}
	}
}

func TestSessionsFooterHeight_SubtractedFromListBudget(t *testing.T) {
	const w, h = 120, 24
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)

	footerH := m.sessionFooterHeight(m.contentWidth())
	if footerH != 2 {
		t.Errorf("condensed footer height = %d, want 2 (1px rule + single key row)", footerH)
	}

	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("composed Sessions view height = %d, want <= %d (footer folded into budget)", got, h)
	}
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("filled frame height = %d, want exactly %d", got, h)
	}
}

func TestSessionsFooterHeight_CountedAtEverySizeApplySite(t *testing.T) {
	const w, h = 120, 20
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = updated.(Model)
	m.applySessions(sessions, nil)

	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("after resize+rebuild composed view height = %d, want <= %d", got, h)
	}
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("after resize+rebuild filled frame height = %d, want exactly %d", got, h)
	}
}

func TestSessionsFooter_OmittedKeysStillDispatchable(t *testing.T) {
	t.Run("k still opens the kill confirm modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionKiller = keymapParityKiller{}
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: 'k', Text: "k"})
		if updated.(Model).modal != modalKillConfirm {
			t.Errorf("k must still open the kill modal after the footer swap")
		}
	})
	t.Run("r still opens the rename modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionRenamer = keymapParityRenamer{}
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if updated.(Model).modal != modalRename {
			t.Errorf("r must still open the rename modal after the footer swap")
		}
	})
	t.Run("q still quits", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatalf("q must still produce a quit cmd after the footer swap")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("q must still quit after the footer swap")
		}
	})
}
