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

func TestHeaderBlock_RendersWordmarkCaretSubtitleRule(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		const w = 80
		header := renderHeaderBlock(w, th, false)

		if !strings.Contains(header, "P O R T A L") {
			t.Errorf("header does not contain the letter-spaced wordmark %q:\n%s", "P O R T A L", header)
		}
		if !strings.Contains(header, "▌") {
			t.Errorf("header does not contain the block caret %q:\n%s", "▌", header)
		}
		if !strings.Contains(header, "session manager") {
			t.Errorf("header does not contain the subtitle %q:\n%s", "session manager", header)
		}

		for _, want := range []struct {
			role string
			tok  theme.Token
		}{
			{"text.primary wordmark", th.TextPrimary},
			{"accent.primary caret", th.AccentPrimary},
			{"text.muted subtitle", th.TextMuted},
		} {
			if seq := tokenFgSeq(t, want.tok); !strings.Contains(header, seq) {
				t.Errorf("header missing the %s foreground role sequence %q", want.role, seq)
			}
		}
		if seq := tokenFgSeq(t, th.Border); !strings.Contains(header, seq) {
			t.Errorf("header missing the border rule role sequence %q", seq)
		}

		lines := strings.Split(header, "\n")
		if got := lipgloss.Height(header); got < 2 {
			t.Errorf("header height = %d, want >= 2 (band + rule)", got)
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("header line %d width = %d, want exactly %d (full-width, no overflow)", i, lw, w)
			}
		}
	})
}

func visibleContent(line string) string {
	return ansi.Strip(line)
}

func TestHeaderBlock_VerticalRhythm(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, testDarkTheme(t), false)
	lines := strings.Split(header, "\n")

	if len(lines) != 3 {
		t.Fatalf("header block has %d lines, want exactly 3 (band, rule, 1 blank):\n%s", len(lines), header)
	}

	if !strings.Contains(visibleContent(lines[0]), "P O R T A L") {
		t.Errorf("line 0 should be the PORTAL band, got %q", visibleContent(lines[0]))
	}
	if !strings.Contains(visibleContent(lines[1]), headerRuleGlyph) {
		t.Errorf("line 1 should be the separator rule flush under the band, got %q", visibleContent(lines[1]))
	}
	if got := strings.TrimSpace(visibleContent(lines[2])); got != "" {
		t.Errorf("line 2 should be blank (rule → section-header gap), got visible %q", got)
	}

	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("header block line %d width = %d, want exactly %d", i, lw, w)
		}
	}
}

func TestHeaderBlock_BlankRowsPaintCanvas(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, testDarkTheme(t), false)
	lines := strings.Split(header, "\n")
	seq := canvasSeq(t, testDarkTheme(t))
	for _, idx := range []int{2} {
		if !strings.Contains(lines[idx], seq) {
			t.Errorf("blank row %d does not paint the canvas background sequence %q: %q", idx, seq, lines[idx])
		}
	}

	colourless := renderHeaderBlock(w, testDarkTheme(t), true)
	clLines := strings.Split(colourless, "\n")
	if len(clLines) != 3 {
		t.Fatalf("colourless header block has %d lines, want 3", len(clLines))
	}
	for _, idx := range []int{2} {
		if strings.Contains(clLines[idx], seq) {
			t.Errorf("colourless blank row %d still paints the canvas background sequence %q", idx, seq)
		}
	}
}

func TestHeaderBlock_SeparatorRule(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, testDarkTheme(t), false)
	rule := headerSeparatorRule(w, testDarkTheme(t), false)
	if got := lipgloss.Height(rule); got != 1 {
		t.Errorf("separator rule height = %d, want 1 (single full-width heavy rule matching the frame)", got)
	}
	if lw := lipgloss.Width(rule); lw != w {
		t.Errorf("rule width = %d, want %d (full-width)", lw, w)
	}
	if !strings.Contains(header, rule) {
		t.Errorf("header block does not contain the separator rule row")
	}
}

func TestHeaderBlock_NarrowDegradeProgressive(t *testing.T) {
	full := renderHeaderBlock(120, testDarkTheme(t), false)
	if !strings.Contains(full, "P O R T A L") {
		t.Errorf("wide header missing full wordmark:\n%s", full)
	}
	if !strings.Contains(full, "session manager") {
		t.Errorf("wide header missing subtitle:\n%s", full)
	}

	noSub := renderHeaderBlock(headerSubtitleMinWidth-1, testDarkTheme(t), false)
	if strings.Contains(noSub, "session manager") {
		t.Errorf("header at width %d still shows the subtitle (step-1 drop failed):\n%s", headerSubtitleMinWidth-1, noSub)
	}
	if !strings.Contains(noSub, "P O R T A L") {
		t.Errorf("header at width %d dropped the full wordmark too early (step-2 before step-1):\n%s", headerSubtitleMinWidth-1, noSub)
	}

	compact := renderHeaderBlock(headerWordmarkMinWidth-1, testDarkTheme(t), false)
	if strings.Contains(compact, "P O R T A L") {
		t.Errorf("header at width %d still shows the full letter-spaced wordmark (compact collapse failed):\n%s", headerWordmarkMinWidth-1, compact)
	}
	if !strings.Contains(compact, headerCompactWordmark) {
		t.Errorf("header at width %d missing the compact wordmark %q:\n%s", headerWordmarkMinWidth-1, headerCompactWordmark, compact)
	}
}

func TestHeaderBlock_NeverOverflowsAtMinWidth(t *testing.T) {
	for _, w := range []int{minTerminalWidth, headerWordmarkMinWidth - 1, headerSubtitleMinWidth - 1, 20, 8} {
		header := renderHeaderBlock(w, testDarkTheme(t), false)
		for i, line := range strings.Split(header, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("at width %d, header line %d width = %d (overflow)", w, i, lw)
			}
		}
	}
}

func TestHeaderBlock_PaintsOnCanvasNoEdgeBleed(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		header := renderHeaderBlock(80, th, false)
		seq := canvasSeq(t, th)
		if !strings.Contains(header, seq) {
			t.Errorf("header does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	})
}

func TestHeaderBlock_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderHeaderBlock(80, testDarkTheme(t), true)

	if !strings.Contains(header, "P O R T A L") {
		t.Errorf("colourless header missing wordmark:\n%s", header)
	}
	if !strings.Contains(header, "session manager") {
		t.Errorf("colourless header missing subtitle:\n%s", header)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(header, seq) {
		t.Errorf("colourless header still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).TextPrimary, testDarkTheme(t).AccentPrimary, testDarkTheme(t).TextMuted, testDarkTheme(t).Border} {
		if seq := tokenFgSeq(t, tok); strings.Contains(header, seq) {
			t.Errorf("colourless header still emits a foreground role sequence %q", seq)
		}
	}
}

func TestHeaderBlock_ZeroWidthFallsBackTo80(t *testing.T) {
	header := renderHeaderBlock(0, testDarkTheme(t), false)
	for i, line := range strings.Split(header, "\n") {
		if lw := lipgloss.Width(line); lw != 80 {
			t.Errorf("zero-width header line %d width = %d, want 80 fallback", i, lw)
		}
	}
}

func TestHeaderHeight_EqualsThreeRows(t *testing.T) {
	const w = 80
	for _, tc := range []struct {
		name       string
		colourless bool
	}{
		{"coloured", false},
		{"colourless", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
			m.colourless = tc.colourless
			if got := m.headerHeight(w); got != 3 {
				t.Errorf("headerHeight(%d) = %d, want 3 (band, rule, 1 blank)", w, got)
			}
		})
	}
}

func TestViewSessionList_ComposesHeaderFirst(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.MemberDark)
	view := m.viewSessionList()

	portalIdx := strings.Index(view, "P O R T A L")
	if portalIdx < 0 {
		t.Fatalf("Sessions view does not contain the header wordmark:\n%s", view)
	}
	titleIdx := strings.Index(view, "Sessions")
	if titleIdx < 0 {
		t.Fatalf("Sessions view does not contain the list title")
	}
	if portalIdx > titleIdx {
		t.Errorf("header wordmark (idx %d) appears after the list title (idx %d); header must be first", portalIdx, titleIdx)
	}
}

func TestHeaderHeight_SubtractedFromListBudget(t *testing.T) {
	const w, h = 90, 24
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)

	headerH := lipgloss.Height(renderHeaderBlock(w, testDarkTheme(t), false))
	if headerH <= 0 {
		t.Fatalf("header height = %d, want > 0", headerH)
	}

	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("composed Sessions view height = %d, want <= %d (header folded into budget)", got, h)
	}
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("filled frame height = %d, want exactly %d", got, h)
	}
}

func TestHeaderHeight_CountedAtEverySizeApplySite(t *testing.T) {
	const w, h = 90, 20
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
