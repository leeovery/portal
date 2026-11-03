package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
)

const sectionHeaderWidth = 90

func TestSectionHeader_LabelCyanCountGreen(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		header := renderSectionHeader(prefs.ModeFlat, "", 7, sectionHeaderWidth, th, false)

		if !strings.Contains(header, "Sessions") {
			t.Errorf("section header missing the %q label:\n%s", "Sessions", header)
		}
		if !strings.Contains(header, "7") {
			t.Errorf("section header missing the count %q:\n%s", "7", header)
		}
		if seq := tokenFgSeq(t, th.AccentMode); !strings.Contains(header, seq) {
			t.Errorf("section header missing the accent.mode label role sequence %q", seq)
		}
		if seq := tokenFgSeq(t, th.StatePositive); !strings.Contains(header, seq) {
			t.Errorf("section header missing the state.positive count role sequence %q", seq)
		}
	})
}

func TestSectionHeader_ModeSuffixFromTitleFn(t *testing.T) {
	for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
		title := sessionListTitleForMode(mode, "")
		suffix := strings.TrimPrefix(title, "Sessions")
		if suffix == "" {
			t.Fatalf("expected a non-empty mode suffix for %v, got title %q", mode, title)
		}
		header := renderSectionHeader(mode, "", 3, sectionHeaderWidth, testDarkTheme(t), false)
		if !strings.Contains(header, strings.TrimSpace(suffix)) {
			t.Errorf("section header for %v missing the suffix %q from sessionListTitleForMode:\n%s", mode, suffix, header)
		}
		if seq := tokenFgSeq(t, testDarkTheme(t).TextMuted); !strings.Contains(header, seq) {
			t.Errorf("section header for %v missing the text.muted suffix role sequence %q", mode, seq)
		}
	}
}

func TestSectionHeader_RightAlignedFilterHint(t *testing.T) {
	for _, mode := range []prefs.SessionListMode{prefs.ModeFlat, prefs.ModeByProject, prefs.ModeByTag} {
		header := renderSectionHeader(mode, "", 4, sectionHeaderWidth, testDarkTheme(t), false)
		if !strings.Contains(header, sectionFilterHint) {
			t.Errorf("section header for %v missing the %q hint:\n%s", mode, sectionFilterHint, header)
		}
		labelIdx := strings.Index(header, "Sessions")
		hintIdx := strings.LastIndex(header, sectionFilterHint)
		if hintIdx < labelIdx {
			t.Errorf("section header for %v: hint (idx %d) appears before the label (idx %d); must be right-aligned", mode, hintIdx, labelIdx)
		}
		if got := lipgloss.Width(header); got != sectionHeaderWidth {
			t.Errorf("section header for %v width = %d, want exactly %d (flex spacer to content width)", mode, got, sectionHeaderWidth)
		}
	}
}

func TestSectionHeader_NoSwitchViewHint(t *testing.T) {
	header := renderSectionHeader(prefs.ModeByTag, "", 4, sectionHeaderWidth, testDarkTheme(t), false)
	if strings.Contains(header, "switch view") {
		t.Errorf("section header must NOT duplicate the footer %q hint:\n%s", "s switch view", header)
	}
}

func TestSectionHeader_PreservesCurrentSessionDecoration(t *testing.T) {
	const current = "my-project-x7k2m9"
	header := renderSectionHeader(prefs.ModeFlat, current, 2, sectionHeaderWidth, testDarkTheme(t), false)
	want := "(current: " + current + ")"
	if !strings.Contains(header, want) {
		t.Errorf("section header dropped the inside-tmux decoration %q:\n%s", want, header)
	}
}

func TestSectionHeader_NarrowDegradeDropsHint(t *testing.T) {
	wide := renderSectionHeader(prefs.ModeFlat, "", 5, sectionHeaderWidth, testDarkTheme(t), false)
	if !strings.Contains(wide, sectionFilterHint) {
		t.Fatalf("wide section header missing the hint:\n%s", wide)
	}

	const narrow = 14
	narrowHeader := renderSectionHeader(prefs.ModeFlat, "", 5, narrow, testDarkTheme(t), false)
	if strings.Contains(narrowHeader, sectionFilterHint) {
		t.Errorf("narrow section header at width %d still shows the %q hint (degrade failed):\n%s", narrow, sectionFilterHint, narrowHeader)
	}
	for i, line := range strings.Split(narrowHeader, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow section header line %d width = %d (overflow, want <= %d)", i, lw, narrow)
		}
	}
}

func TestSectionHeader_CountValueAndSuffixByteIdentical(t *testing.T) {
	for _, tc := range []struct {
		mode  prefs.SessionListMode
		count int
	}{
		{prefs.ModeFlat, 12},
		{prefs.ModeByProject, 8},
		{prefs.ModeByTag, 15},
	} {
		header := renderSectionHeader(tc.mode, "", tc.count, sectionHeaderWidth, testDarkTheme(t), false)
		title := sessionListTitleForMode(tc.mode, "")
		if suffix := strings.TrimSpace(strings.TrimPrefix(title, "Sessions")); suffix != "" && !strings.Contains(header, suffix) {
			t.Errorf("section header for %v missing the parity suffix %q from title %q:\n%s", tc.mode, suffix, title, header)
		}
		countRun := headerStyle(testDarkTheme(t).StatePositive, testDarkTheme(t), false).Render(itoa(tc.count))
		if !strings.Contains(header, countRun) {
			t.Errorf("section header for %v missing the exact count %d in a state.positive run:\n%s", tc.mode, tc.count, header)
		}
	}
}

func TestSectionHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderSectionHeader(prefs.ModeByTag, "", 6, sectionHeaderWidth, testDarkTheme(t), true)

	if !strings.Contains(header, "Sessions") || !strings.Contains(header, "6") || !strings.Contains(header, sectionFilterHint) {
		t.Errorf("colourless section header dropped structure:\n%s", header)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(header, seq) {
		t.Errorf("colourless section header still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).AccentMode, testDarkTheme(t).StatePositive, testDarkTheme(t).TextMuted} {
		if seq := tokenFgSeq(t, tok); strings.Contains(header, seq) {
			t.Errorf("colourless section header still emits a foreground role sequence %q", seq)
		}
	}
}

func TestSectionHeader_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		header := renderSectionHeader(prefs.ModeFlat, "", 3, sectionHeaderWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(header, seq) {
			t.Errorf("section header does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	}
}

func TestViewSessionList_ReplacesTitleWithSectionHeader(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.MemberDark)
	view := m.viewSessionList()

	countRun := headerStyle(testDarkTheme(t).StatePositive, testDarkTheme(t), false).Render("3")
	if !strings.Contains(view, countRun) {
		t.Errorf("composed Sessions view missing the state.positive count run for 3 visible sessions:\n%s", view)
	}
	if !strings.Contains(view, sectionFilterHint) {
		t.Errorf("composed Sessions view missing the %q hint:\n%s", sectionFilterHint, view)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentMode); !strings.Contains(view, seq) {
		t.Errorf("composed Sessions view missing the accent.mode label role sequence %q", seq)
	}
	if got := m.SessionListTitle(); got != "Sessions" {
		t.Errorf("SessionListTitle() = %q, want %q (title field untouched by the reskin)", got, "Sessions")
	}
}

func TestViewSessionList_SectionHeaderCountMatchesVisible(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Attached: true},
		{Name: "bravo"},
		{Name: "charlie"},
	}
	m := NewModelWithSessions(sessions).WithCurrentSession("alpha")
	view := m.viewSessionList()

	twoRun := headerStyle(testDarkTheme(t).StatePositive, testDarkTheme(t), false).Render("2")
	if !strings.Contains(view, twoRun) {
		t.Errorf("composed view section-header count should be 2 (alpha excluded as the current session):\n%s", view)
	}
	if !strings.Contains(view, "current: alpha") {
		t.Errorf("composed view dropped the inside-tmux decoration:\n%s", view)
	}
}

func TestViewSessionList_FilterInputNotReplaced(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.MemberDark)
	m.sessionList.SetFilterState(list.Filtering)
	view := m.viewSessionList()

	if strings.Contains(view, sectionFilterHint) {
		t.Errorf("section header hint leaked into the active filter-input frame:\n%s", view)
	}
}

func leadingPrintableCol(line string) int {
	stripped := ansi.Strip(line)
	return len(stripped) - len(strings.TrimLeft(stripped, " "))
}

func TestSectionHeader_AlignsWithHeaderWordmark(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		const w = sectionHeaderWidth

		headerFirstLine, _, _ := strings.Cut(renderHeaderBlock(w, th, false), "\n")
		wordmarkCol := leadingPrintableCol(headerFirstLine)
		if wordmarkCol != 0 {
			t.Fatalf("PORTAL wordmark leading column = %d, want 0 (flush at the content edge)", wordmarkCol)
		}

		section := renderSectionHeader(prefs.ModeFlat, "", 3, w, th, false)
		sectionCol := leadingPrintableCol(section)
		if sectionCol != 0 {
			t.Errorf("section header `Sessions` leading column = %d, want 0 (no extra indent; must align with the PORTAL wordmark at the content edge)", sectionCol)
		}
		if sectionCol != wordmarkCol {
			t.Errorf("section header leading column %d != PORTAL wordmark leading column %d; they must share the content's left edge", sectionCol, wordmarkCol)
		}
	})
}

func TestViewSessionList_HeaderSectionCursorShareLeftEdge(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.MemberDark)
	view := m.viewSessionList()

	var wordmarkCol, sectionCol, cursorCol = -1, -1, -1
	for line := range strings.SplitSeq(view, "\n") {
		stripped := strings.TrimLeft(ansi.Strip(line), " ")
		switch {
		case strings.HasPrefix(stripped, "P O R T A L"):
			wordmarkCol = leadingPrintableCol(line)
		case strings.HasPrefix(stripped, "Sessions"):
			sectionCol = leadingPrintableCol(line)
		case strings.HasPrefix(stripped, "▌") && cursorCol < 0:
			cursorCol = leadingPrintableCol(line)
		}
	}

	if wordmarkCol < 0 || sectionCol < 0 || cursorCol < 0 {
		t.Fatalf("composed view missing a measured row: wordmarkCol=%d sectionCol=%d cursorCol=%d\n%s", wordmarkCol, sectionCol, cursorCol, view)
	}
	if wordmarkCol != sectionCol || sectionCol != cursorCol {
		t.Errorf("left edges differ: PORTAL=%d Sessions=%d cursor=%d; all three must share the content's left edge", wordmarkCol, sectionCol, cursorCol)
	}
}

func isBlankRow(line string) bool {
	return strings.TrimSpace(ansi.Strip(line)) == ""
}

func TestViewSessionList_HeaderZoneVerticalRhythm(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.MemberDark)
	lines := strings.Split(m.viewSessionList(), "\n")

	idxOf := func(prefix string) int {
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimLeft(ansi.Strip(line), " "), prefix) {
				return i
			}
		}
		return -1
	}

	portalIdx := idxOf("P O R T A L")
	ruleIdx := idxOf(headerRuleGlyph)
	sessionsIdx := idxOf("Sessions")
	firstRowIdx := idxOf("▌")

	if portalIdx < 0 || ruleIdx < 0 || sessionsIdx < 0 || firstRowIdx < 0 {
		t.Fatalf("composed view missing a landmark: PORTAL=%d rule=%d Sessions=%d firstRow=%d\n%s",
			portalIdx, ruleIdx, sessionsIdx, firstRowIdx, m.viewSessionList())
	}

	if ruleIdx != portalIdx+1 {
		t.Errorf("rule row at %d, want %d (flush under PORTAL, no blank)", ruleIdx, portalIdx+1)
	}
	if sessionsIdx != ruleIdx+2 {
		t.Errorf("Sessions row at %d, want %d (rule + 1 blank + Sessions)", sessionsIdx, ruleIdx+2)
	}
	if firstRowIdx != sessionsIdx+2 {
		t.Errorf("first session row at %d, want %d (Sessions + 1 blank + first row)", firstRowIdx, sessionsIdx+2)
	}

	for _, idx := range []int{ruleIdx + 1, sessionsIdx + 1} {
		if !isBlankRow(lines[idx]) {
			t.Errorf("header-zone gap row %d is not blank: %q", idx, ansi.Strip(lines[idx]))
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
