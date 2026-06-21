package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §6 / §3.2 / §13.6 Projects section header: `Projects` (state.green) + a
// text.detail count at the SAME cap-height as the label (dim by colour, not a
// smaller glyph), with a right-aligned `/ to filter` hint (text.detail). These
// tests pin the colour roles in exact mode-resolved SGR, the same-cap-height count
// (a plain run, no superscript), the persistent hint, and the §2.7 narrow degrade —
// matching testdata/vhs/reference/projects-mv.png.

const projectsHeaderWidth = 90

// TestProjectsHeader_LabelGreenCountDetail asserts the label renders in
// state.green and the count in text.detail — at the same font size (no
// smaller/superscript glyph), distinguished only by colour (§13.6). Both are plain
// runs so the count digits sit on the same baseline/cap-height as the label.
func TestProjectsHeader_LabelGreenCountDetail(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := renderProjectsSectionHeader(14, projectsHeaderWidth, tc.mode, false)

			if !strings.Contains(ansi.Strip(header), "Projects") {
				t.Errorf("Projects header missing the %q label:\n%s", "Projects", header)
			}
			if !strings.Contains(ansi.Strip(header), "14") {
				t.Errorf("Projects header missing the count %q:\n%s", "14", header)
			}
			// Label is state.green.
			if seq := tokenFgSeq(t, theme.MV.StateGreen, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("Projects header missing the state.green label role sequence %q", seq)
			}
			// The count VALUE renders verbatim inside its own text.detail run (so it is
			// byte-identical and dim, at the same cap-height as the label).
			countRun := headerStyle(theme.MV.TextDetail, tc.mode, false).Render("14")
			if !strings.Contains(header, countRun) {
				t.Errorf("Projects header missing the exact count 14 in a text.detail run:\n%s", header)
			}
		})
	}
}

// TestProjectsHeader_RightAlignedFilterHint asserts a `/ to filter` hint renders in
// text.detail, right-aligned (left cluster + flex spacer + hint to the content
// width), and the row is exactly the content width.
func TestProjectsHeader_RightAlignedFilterHint(t *testing.T) {
	header := renderProjectsSectionHeader(8, projectsHeaderWidth, theme.Dark, false)
	if !strings.Contains(header, sectionFilterHint) {
		t.Errorf("Projects header missing the %q hint:\n%s", sectionFilterHint, header)
	}
	labelIdx := strings.Index(ansi.Strip(header), "Projects")
	hintIdx := strings.LastIndex(ansi.Strip(header), sectionFilterHint)
	if hintIdx < labelIdx {
		t.Errorf("Projects header: hint (idx %d) appears before the label (idx %d); must be right-aligned", hintIdx, labelIdx)
	}
	if got := lipgloss.Width(header); got != projectsHeaderWidth {
		t.Errorf("Projects header width = %d, want exactly %d (flex spacer to content width)", got, projectsHeaderWidth)
	}
}

// TestProjectsHeader_AlignsWithWordmark is the cross-element alignment guard: the
// Projects header's `Projects` label must start at the SAME column as the PORTAL
// wordmark — the content's left edge (col 0 of the inset region), with no leading
// indent.
func TestProjectsHeader_AlignsWithWordmark(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		const w = projectsHeaderWidth
		wordmarkLine := strings.SplitN(renderHeaderBlock(w, mode, false), "\n", 2)[0]
		wordmarkCol := leadingPrintableCol(wordmarkLine)
		if wordmarkCol != 0 {
			t.Fatalf("[%v] PORTAL wordmark leading column = %d, want 0", mode, wordmarkCol)
		}
		header := renderProjectsSectionHeader(3, w, mode, false)
		if got := leadingPrintableCol(header); got != 0 {
			t.Errorf("[%v] Projects header leading column = %d, want 0 (must align with the PORTAL wordmark at the content edge)", mode, got)
		}
	}
}

// TestProjectsHeader_NarrowDegradeDropsHint asserts the §2.7 narrow degrade: below
// the threshold the right `/ to filter` hint drops and the row never overflows.
func TestProjectsHeader_NarrowDegradeDropsHint(t *testing.T) {
	wide := renderProjectsSectionHeader(5, projectsHeaderWidth, theme.Dark, false)
	if !strings.Contains(wide, sectionFilterHint) {
		t.Fatalf("wide Projects header missing the hint:\n%s", wide)
	}
	const narrow = 14
	narrowHeader := renderProjectsSectionHeader(5, narrow, theme.Dark, false)
	if strings.Contains(narrowHeader, sectionFilterHint) {
		t.Errorf("narrow Projects header at width %d still shows the %q hint (degrade failed):\n%s", narrow, sectionFilterHint, narrowHeader)
	}
	for i, line := range strings.Split(narrowHeader, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow Projects header line %d width = %d (overflow, want <= %d)", i, lw, narrow)
		}
	}
}

// TestProjectsHeader_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless Projects header carries no canvas background SGR and no
// foreground hue — structure (label, count, hint) intact.
func TestProjectsHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderProjectsSectionHeader(6, projectsHeaderWidth, theme.Dark, true)
	if !strings.Contains(ansi.Strip(header), "Projects") || !strings.Contains(ansi.Strip(header), "6") || !strings.Contains(header, sectionFilterHint) {
		t.Errorf("colourless Projects header dropped structure:\n%s", header)
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless Projects header still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.StateGreen, theme.MV.TextDetail} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless Projects header still emits a foreground role sequence %q", seq)
		}
	}
}
