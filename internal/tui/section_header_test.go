package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §3.2/§4.2 Sessions section header: `Sessions` (accent.cyan) + count
// (state.green, same font size, dim-by-colour §13.6) + optional mode suffix
// (text.detail), with a right-aligned `/ to filter` hint (text.detail) on the
// same row. These tests pin the colour roles, the parity with
// sessionListTitleForMode, the persistent hint, the inside-tmux decoration, and
// the §2.7 narrow degrade.

const sectionHeaderWidth = 90

// TestSectionHeader_LabelCyanCountGreen asserts the label renders in accent.cyan
// and the count in state.green — at the same font size (no smaller/superscript
// glyph), distinguished only by colour (§13.6). Both are plain runs, so the
// count digits sit on the same baseline as the label letters.
func TestSectionHeader_LabelCyanCountGreen(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := renderSectionHeader(prefs.ModeFlat, false, "", 7, sectionHeaderWidth, tc.mode, false)

			if !strings.Contains(header, "Sessions") {
				t.Errorf("section header missing the %q label:\n%s", "Sessions", header)
			}
			if !strings.Contains(header, "7") {
				t.Errorf("section header missing the count %q:\n%s", "7", header)
			}
			// Label is accent.cyan, count is state.green — each via its token.
			if seq := tokenFgSeq(t, theme.MV.AccentCyan, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("section header missing the accent.cyan label role sequence %q", seq)
			}
			if seq := tokenFgSeq(t, theme.MV.StateGreen, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("section header missing the state.green count role sequence %q", seq)
			}
		})
	}
}

// TestSectionHeader_ModeSuffixFromTitleFn asserts the mode suffix renders in
// text.detail and is byte-identical (as a substring) to the suffix
// sessionListTitleForMode produces — the single source of truth for parity.
func TestSectionHeader_ModeSuffixFromTitleFn(t *testing.T) {
	for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
		title := sessionListTitleForMode(mode, false, "")
		suffix := strings.TrimPrefix(title, "Sessions")
		if suffix == "" {
			t.Fatalf("expected a non-empty mode suffix for %v, got title %q", mode, title)
		}
		header := renderSectionHeader(mode, false, "", 3, sectionHeaderWidth, theme.Dark, false)
		if !strings.Contains(header, strings.TrimSpace(suffix)) {
			t.Errorf("section header for %v missing the suffix %q from sessionListTitleForMode:\n%s", mode, suffix, header)
		}
		// The suffix is rendered in text.detail.
		if seq := tokenFgSeq(t, theme.MV.TextDetail, theme.Dark); !strings.Contains(header, seq) {
			t.Errorf("section header for %v missing the text.detail suffix role sequence %q", mode, seq)
		}
	}
}

// TestSectionHeader_RightAlignedFilterHint asserts a `/ to filter` hint renders
// in text.detail, right-aligned (the left cluster and the hint are separated by
// a flex spacer to the content width), on flat, by-project, and by-tag.
func TestSectionHeader_RightAlignedFilterHint(t *testing.T) {
	for _, mode := range []prefs.SessionListMode{prefs.ModeFlat, prefs.ModeByProject, prefs.ModeByTag} {
		header := renderSectionHeader(mode, false, "", 4, sectionHeaderWidth, theme.Dark, false)
		if !strings.Contains(header, sectionFilterHint) {
			t.Errorf("section header for %v missing the %q hint:\n%s", mode, sectionFilterHint, header)
		}
		// The hint is right-aligned: it appears AFTER the label in the row.
		labelIdx := strings.Index(header, "Sessions")
		hintIdx := strings.LastIndex(header, sectionFilterHint)
		if hintIdx < labelIdx {
			t.Errorf("section header for %v: hint (idx %d) appears before the label (idx %d); must be right-aligned", mode, hintIdx, labelIdx)
		}
		// The single rendered row is exactly the content width (the spacer fills it).
		if got := lipgloss.Width(header); got != sectionHeaderWidth {
			t.Errorf("section header for %v width = %d, want exactly %d (flex spacer to content width)", mode, got, sectionHeaderWidth)
		}
	}
}

// TestSectionHeader_NoSwitchViewHint asserts the `s switch view` hint is NOT
// duplicated in the section header — it lives in the footer only (§3.2).
func TestSectionHeader_NoSwitchViewHint(t *testing.T) {
	header := renderSectionHeader(prefs.ModeByTag, false, "", 4, sectionHeaderWidth, theme.Dark, false)
	if strings.Contains(header, "switch view") {
		t.Errorf("section header must NOT duplicate the footer %q hint:\n%s", "s switch view", header)
	}
}

// TestSectionHeader_PreservesInsideTmuxDecoration asserts the inside-tmux
// `(current: %s)` decoration survives the restyle — it is sourced from
// sessionListTitleForMode so its wording is identical to the pre-reskin title.
func TestSectionHeader_PreservesInsideTmuxDecoration(t *testing.T) {
	const current = "my-project-x7k2m9"
	header := renderSectionHeader(prefs.ModeFlat, true, current, 2, sectionHeaderWidth, theme.Dark, false)
	want := "(current: " + current + ")"
	if !strings.Contains(header, want) {
		t.Errorf("section header dropped the inside-tmux decoration %q:\n%s", want, header)
	}
}

// TestSectionHeader_NarrowDegradeDropsHint asserts the §2.7 narrow degrade: below
// the threshold (where the left cluster + spacer + hint no longer fit) the right
// `/ to filter` hint drops, and the row never overflows the width.
func TestSectionHeader_NarrowDegradeDropsHint(t *testing.T) {
	// Wide: hint present.
	wide := renderSectionHeader(prefs.ModeFlat, false, "", 5, sectionHeaderWidth, theme.Dark, false)
	if !strings.Contains(wide, sectionFilterHint) {
		t.Fatalf("wide section header missing the hint:\n%s", wide)
	}

	// Narrow: a width that cannot hold the left cluster + a spacer + the hint.
	const narrow = 14
	narrowHeader := renderSectionHeader(prefs.ModeFlat, false, "", 5, narrow, theme.Dark, false)
	if strings.Contains(narrowHeader, sectionFilterHint) {
		t.Errorf("narrow section header at width %d still shows the %q hint (degrade failed):\n%s", narrow, sectionFilterHint, narrowHeader)
	}
	// Never overflows.
	for i, line := range strings.Split(narrowHeader, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow section header line %d width = %d (overflow, want <= %d)", i, lw, narrow)
		}
	}
}

// TestSectionHeader_CountValueAndSuffixByteIdentical asserts the count VALUE and
// the mode-suffix TEXT are byte-identical to the pre-reskin title output: the
// label+suffix string equals sessionListTitleForMode, and the count is the exact
// integer passed (no transformation).
func TestSectionHeader_CountValueAndSuffixByteIdentical(t *testing.T) {
	for _, tc := range []struct {
		mode  prefs.SessionListMode
		count int
	}{
		{prefs.ModeFlat, 12},
		{prefs.ModeByProject, 8},
		{prefs.ModeByTag, 15},
	} {
		header := renderSectionHeader(tc.mode, false, "", tc.count, sectionHeaderWidth, theme.Dark, false)
		// The mode-suffix text is the sessionListTitleForMode remainder (parity).
		title := sessionListTitleForMode(tc.mode, false, "")
		if suffix := strings.TrimSpace(strings.TrimPrefix(title, "Sessions")); suffix != "" && !strings.Contains(header, suffix) {
			t.Errorf("section header for %v missing the parity suffix %q from title %q:\n%s", tc.mode, suffix, title, header)
		}
		// The exact count value is rendered inside its own state.green run — build
		// the same run the implementation emits (state.green fg over the canvas) and
		// assert it appears verbatim, so the count value is byte-identical and green.
		countRun := headerStyle(theme.MV.StateGreen, theme.Dark, false).Render(itoa(tc.count))
		if !strings.Contains(header, countRun) {
			t.Errorf("section header for %v missing the exact count %d in a state.green run:\n%s", tc.mode, tc.count, header)
		}
	}
}

// TestSectionHeader_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless section header carries no canvas background SGR and no
// foreground hue — structure (label, count, suffix, hint) intact.
func TestSectionHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderSectionHeader(prefs.ModeByTag, false, "", 6, sectionHeaderWidth, theme.Dark, true)

	if !strings.Contains(header, "Sessions") || !strings.Contains(header, "6") || !strings.Contains(header, sectionFilterHint) {
		t.Errorf("colourless section header dropped structure:\n%s", header)
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless section header still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.AccentCyan, theme.MV.StateGreen, theme.MV.TextDetail} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless section header still emits a foreground role sequence %q", seq)
		}
	}
}

// TestSectionHeader_PaintsCanvasNoEdgeBleed asserts the section header cells carry
// the owned canvas background (leaf .Background(canvas)) so the right-aligned
// spacer gap is canvas-painted, not a terminal-bg island.
func TestSectionHeader_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		header := renderSectionHeader(prefs.ModeFlat, false, "", 3, sectionHeaderWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(header, seq) {
			t.Errorf("section header does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	}
}

// TestViewSessionList_ReplacesTitleWithSectionHeader asserts the composed
// Sessions view renders the §3.2 section header in place of the plain bubbles/list
// title: the `Sessions` label (accent.cyan), the live count (state.green) matching
// the visible session count, and the right-aligned `/ to filter` hint — while the
// title FIELD (m.sessionList.Title) keeps its parity value.
func TestViewSessionList_ReplacesTitleWithSectionHeader(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.Dark) // 3 sessions (alpha/bravo/charlie)
	view := m.viewSessionList()

	// The section header's count (3) renders in a state.green run.
	countRun := headerStyle(theme.MV.StateGreen, theme.Dark, false).Render("3")
	if !strings.Contains(view, countRun) {
		t.Errorf("composed Sessions view missing the state.green count run for 3 visible sessions:\n%s", view)
	}
	// The persistent right hint is present.
	if !strings.Contains(view, sectionFilterHint) {
		t.Errorf("composed Sessions view missing the %q hint:\n%s", sectionFilterHint, view)
	}
	// The `Sessions` label is painted in accent.cyan (the replaced title line).
	if seq := tokenFgSeq(t, theme.MV.AccentCyan, theme.Dark); !strings.Contains(view, seq) {
		t.Errorf("composed Sessions view missing the accent.cyan label role sequence %q", seq)
	}
	// Parity: the title FIELD is unchanged.
	if got := m.SessionListTitle(); got != "Sessions" {
		t.Errorf("SessionListTitle() = %q, want %q (title field untouched by the reskin)", got, "Sessions")
	}
}

// TestViewSessionList_SectionHeaderCountMatchesVisible asserts the section header
// count tracks the VISIBLE session count: with the current session excluded inside
// tmux the count drops by one (parity with filteredSessions).
func TestViewSessionList_SectionHeaderCountMatchesVisible(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: true},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	m := NewModelWithSessions(sessions).WithInsideTmux("alpha")
	view := m.viewSessionList()

	// Two sessions are visible (alpha is the current session, excluded).
	twoRun := headerStyle(theme.MV.StateGreen, theme.Dark, false).Render("2")
	if !strings.Contains(view, twoRun) {
		t.Errorf("composed view section-header count should be 2 (alpha excluded inside tmux):\n%s", view)
	}
	// The inside-tmux decoration survives in the rendered view.
	if !strings.Contains(view, "current: alpha") {
		t.Errorf("composed view dropped the inside-tmux decoration:\n%s", view)
	}
}

// TestViewSessionList_FilterInputNotReplaced asserts the section header does NOT
// overwrite the filter input row while the filter is being typed (FilterState ==
// Filtering): the title row is the live filter input then, not the section header.
func TestViewSessionList_FilterInputNotReplaced(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.Dark)
	m.sessionList.SetFilterState(list.Filtering)
	view := m.viewSessionList()

	// The section header's `/ to filter` hint must NOT appear while typing a filter
	// (the row is the live input, the section header is suppressed for that frame).
	if strings.Contains(view, sectionFilterHint) {
		t.Errorf("section header hint leaked into the active filter-input frame:\n%s", view)
	}
}

// leadingPrintableCol returns the column of the first printable (non-space)
// character of a rendered line, after stripping ANSI/SGR sequences — i.e. the
// number of leading spaces. It is the cross-element left-edge measurement the
// alignment guards compare.
func leadingPrintableCol(line string) int {
	stripped := ansi.Strip(line)
	return len(stripped) - len(strings.TrimLeft(stripped, " "))
}

// TestSectionHeader_AlignsWithHeaderWordmark is the cross-element alignment guard
// (the check that was missing when the col-2 indent shipped): the section header's
// `Sessions` label must start at the SAME column as the header.go PORTAL wordmark —
// the content's left edge (col 0 of the inset region). header.go's headerBand
// renders the wordmark flush at col 0 with NO leading indent, so the section header
// must too; the old groupHeaderIndent prefix (a legacy artifact of the bubbles/list
// TitleBar PaddingLeft=2 that this header REPLACES) pushed `Sessions` 2 cells right
// of `PORTAL`. Both leading printable columns must be 0 and equal.
func TestSectionHeader_AlignsWithHeaderWordmark(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const w = sectionHeaderWidth

			headerFirstLine := strings.SplitN(renderHeaderBlock(w, tc.mode, false), "\n", 2)[0]
			wordmarkCol := leadingPrintableCol(headerFirstLine)
			if wordmarkCol != 0 {
				t.Fatalf("PORTAL wordmark leading column = %d, want 0 (flush at the content edge)", wordmarkCol)
			}

			section := renderSectionHeader(prefs.ModeFlat, false, "", 3, w, tc.mode, false)
			sectionCol := leadingPrintableCol(section)
			if sectionCol != 0 {
				t.Errorf("section header `Sessions` leading column = %d, want 0 (no extra indent; must align with the PORTAL wordmark at the content edge)", sectionCol)
			}
			if sectionCol != wordmarkCol {
				t.Errorf("section header leading column %d != PORTAL wordmark leading column %d; they must share the content's left edge", sectionCol, wordmarkCol)
			}
		})
	}
}

// TestViewSessionList_HeaderSectionCursorShareLeftEdge is the composed-view
// cross-element alignment guard: in the fully composed Sessions view the FIRST
// printable column of the PORTAL wordmark row, the `Sessions` section-header row,
// and the row cursor/selector (the ▌ bar) must all be the SAME column — the
// content's left edge. The row NAMES stay the 2-cell bar-column width further in
// (they sit after the ▌ bar column); that is correct and is not what this guard
// measures.
func TestViewSessionList_HeaderSectionCursorShareLeftEdge(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.Dark)
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

// isBlankRow reports whether a rendered line carries no visible printable glyph
// (canvas-painted spaces only) after stripping ANSI/SGR sequences.
func isBlankRow(line string) bool {
	return strings.TrimSpace(ansi.Strip(line)) == ""
}

// TestViewSessionList_HeaderZoneVerticalRhythm is the composed end-to-end guard
// for the header-zone vertical breathing room. The separator rule is FLUSH beneath
// the PORTAL band (the rule is the wordmark's underline), then ONE blank row before
// "Sessions" and ONE blank row before the first session row:
//
//	PORTAL band → (flush) → separator rule → 1 blank → "Sessions" section header
//	→ 1 blank → first session row
//
// The rule→Sessions gap is folded into renderHeaderBlock and the Sessions→first-row
// gap is the TitleBar PaddingBottom, so this asserts the whole zone as the user
// sees it. It exists because a flush-chrome spacing miss slipped through the
// element-scoped checks; locking the composed sequence stops a silent regress.
func TestViewSessionList_HeaderZoneVerticalRhythm(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.Dark) // 3 sessions: alpha/bravo/charlie
	lines := strings.Split(m.viewSessionList(), "\n")

	// Locate each landmark row by its visible content.
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
	// The first session row is the cursor row (the first ▌ selector-bar line).
	firstRowIdx := idxOf("▌")

	if portalIdx < 0 || ruleIdx < 0 || sessionsIdx < 0 || firstRowIdx < 0 {
		t.Fatalf("composed view missing a landmark: PORTAL=%d rule=%d Sessions=%d firstRow=%d\n%s",
			portalIdx, ruleIdx, sessionsIdx, firstRowIdx, m.viewSessionList())
	}

	// Exact row offsets: rule FLUSH under PORTAL (no blank between), 1 blank
	// (rule→Sessions), 1 blank (Sessions→first row).
	if ruleIdx != portalIdx+1 {
		t.Errorf("rule row at %d, want %d (flush under PORTAL, no blank)", ruleIdx, portalIdx+1)
	}
	if sessionsIdx != ruleIdx+2 {
		t.Errorf("Sessions row at %d, want %d (rule + 1 blank + Sessions)", sessionsIdx, ruleIdx+2)
	}
	if firstRowIdx != sessionsIdx+2 {
		t.Errorf("first session row at %d, want %d (Sessions + 1 blank + first row)", firstRowIdx, sessionsIdx+2)
	}

	// The two breathing-room rows are actually blank; PORTAL→rule has none (flush).
	for _, idx := range []int{ruleIdx + 1, sessionsIdx + 1} {
		if !isBlankRow(lines[idx]) {
			t.Errorf("header-zone gap row %d is not blank: %q", idx, ansi.Strip(lines[idx]))
		}
	}
}

// itoa is a tiny test-local int→string so the assertions stay dependency-free.
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
