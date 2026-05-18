package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// These tests pin composeChromeLine's cascade behaviour at the spec's tier
// thresholds per specification.md § Width cascade and § Top edge composition.
// All args are the function's INNER width parameter (terminalWidth − 2); the
// function's returned string has display-cell width arg+2 (the outer width)
// for arg >= 0 and is empty for arg < 0.
//
// Threshold args are derived from the cascade math with the test counter
// fixture (windowIdx=0, windowCount=1, paneIdx=0, paneCount=1) — counters
// segment "Window 1 of 1 · Pane 1 of 1" is 27 display cells. With
// fixedTier1 = 4 + 27 + 8 + 1 + 57 = 97 cells, tier 1 needs outer >= 105 for
// nameBudget == 8 (i.e. inner arg >= 103). Tier 2 needs outer >= 4+27+1+57 = 89
// (arg >= 87). Tier 3 needs outer >= 4+27+1+9 = 41 (arg >= 39). Tier 4 always
// fits at arg >= 0.

const testWindowName = "nvim-editor" // 11 display cells

// Counter fixture used by every cascade test below — keeps fixed-overhead
// arithmetic consistent. windowIdx=0, windowCount=1, paneIdx=0, paneCount=1
// produces "Window 1 of 1 · Pane 1 of 1" (27 cells).
func ccl(arg int, name string) string {
	return composeChromeLine(arg, 0, 1, 0, 1, name)
}

func TestComposeChromeLine_NegativeArgReturnsEmpty(t *testing.T) {
	if got := ccl(-1, testWindowName); got != "" {
		t.Errorf("composeChromeLine(-1, ...) = %q; want empty string", got)
	}
}

func TestComposeChromeLine_OutputWidthEqualsArgPlusTwo(t *testing.T) {
	for _, arg := range []int{0, 1, 2, 5, 13, 25, 40, 60, 87, 95, 102, 103, 200} {
		got := ccl(arg, testWindowName)
		if w := lipgloss.Width(got); w != arg+2 {
			t.Errorf("arg %d: lipgloss.Width(got)=%d, want %d; got=%q", arg, w, arg+2, got)
		}
	}
}

// TestComposeChromeLine_NoEmbeddedNewlinesAcrossThresholds sweeps the
// cascade-boundary width set (every tier-transition arg). The
// spec-mandated invariant test for the resize-math contract is
// TestComposeChromeLine_NoEmbeddedNewlines below, which uses the spec's
// chrome-row invariant width set. Both assertions hold identically; the
// two width sets exist because they answer different questions (cascade
// boundaries vs resize-math invariant widths).
func TestComposeChromeLine_NoEmbeddedNewlinesAcrossThresholds(t *testing.T) {
	for _, arg := range []int{0, 1, 2, 5, 13, 25, 40, 60, 87, 95, 102, 103, 200} {
		got := ccl(arg, testWindowName)
		if n := strings.Count(got, "\n"); n != 0 {
			t.Errorf("arg %d: strings.Count(got, \"\\n\")=%d, want 0; got=%q", arg, n, got)
		}
	}
}

// Chrome-row single-line invariant test per specification.md § Chrome-row
// invariant for resize math and § Tests > Chrome-row invariant test. The
// resize math viewport.SetSize(msg.Width − 2, msg.Height − 2) assumes the
// top edge is exactly one row at every width; this test guards that
// assumption across the spec's chrome-row invariant width set, which spans
// every cascade tier (tier 1 wide, tier 1 truncated, tier 2, tier 3, tier 4
// down to degenerate widths). Negative widths are excluded — they return
// empty string and are not load-bearing for this invariant.
func TestComposeChromeLine_NoEmbeddedNewlines(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0} {
		got := composeChromeLine(w, 0, 1, 0, 1, testWindowName)
		if n := strings.Count(got, "\n"); n != 0 {
			t.Errorf("composeChromeLine(width=%d) returned %d embedded newline(s); want 0; got=%q", w, n, got)
		}
	}
}

func TestComposeChromeLine_CornerGlyphsSourcedFromLipglossRoundedBorder(t *testing.T) {
	border := lipgloss.RoundedBorder()
	got := ccl(200, testWindowName)
	if !strings.HasPrefix(got, border.TopLeft) {
		t.Errorf("composeChromeLine output does not start with border.TopLeft %q; got=%q", border.TopLeft, got)
	}
	if !strings.HasSuffix(got, border.TopRight) {
		t.Errorf("composeChromeLine output does not end with border.TopRight %q; got=%q", border.TopRight, got)
	}
}

// Tier 1 — wide width, full name present.
func TestComposeChromeLine_Arg200Tier1FullNamePresent(t *testing.T) {
	got := ccl(200, testWindowName)
	if !strings.Contains(got, "win: "+testWindowName) {
		t.Errorf("arg 200: expected full window name segment %q; got=%q", "win: "+testWindowName, got)
	}
	if !strings.Contains(got, verboseKeymap) {
		t.Errorf("arg 200: expected verboseKeymap present; got=%q", got)
	}
	if strings.Contains(got, "…") {
		t.Errorf("arg 200: expected no ellipsis on full-name tier 1; got=%q", got)
	}
}

// Tier 1 — name truncated with ellipsis. arg 103 is the smallest arg at which
// nameBudget == 8 (minWindowNameCells); the 11-cell test name does not fit in
// 8 cells so it is truncated and "…" is appended.
func TestComposeChromeLine_NameBudgetExactlyEightStaysInTier1(t *testing.T) {
	const arg = 103 // outer 105, nameBudget = 105-97 = 8
	got := ccl(arg, testWindowName)
	if !strings.Contains(got, "win: ") {
		t.Errorf("arg %d: expected 'win: ' segment present (tier 1); got=%q", arg, got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("arg %d: expected ellipsis on truncated name; got=%q", arg, got)
	}
	if !strings.Contains(got, verboseKeymap) {
		t.Errorf("arg %d: expected verboseKeymap present on tier 1; got=%q", arg, got)
	}
}

// Tier 2 — drops to tier 2 when nameBudget == 7 (one below the minimum).
func TestComposeChromeLine_NameBudgetSevenDropsToTier2(t *testing.T) {
	const arg = 102 // outer 104, nameBudget = 104-97 = 7 → below minWindowNameCells
	got := ccl(arg, testWindowName)
	if strings.Contains(got, "win:") {
		t.Errorf("arg %d: expected NO 'win:' segment (tier 2); got=%q", arg, got)
	}
	if !strings.Contains(got, verboseKeymap) {
		t.Errorf("arg %d: expected verboseKeymap present on tier 2; got=%q", arg, got)
	}
}

// Tier 2 — drops window-name segment; verbose keymap still fits. arg 95 →
// outer 97 ≥ 89 (tier 2 minimum), and nameBudget = 97-97 = 0 < 8.
func TestComposeChromeLine_Arg95Tier2WinSegmentAbsentVerboseKeymapPresent(t *testing.T) {
	const arg = 95
	got := ccl(arg, testWindowName)
	if strings.Contains(got, "win:") {
		t.Errorf("arg %d: expected NO 'win:' segment (tier 2); got=%q", arg, got)
	}
	if !strings.Contains(got, verboseKeymap) {
		t.Errorf("arg %d: expected verboseKeymap present on tier 2; got=%q", arg, got)
	}
}

// Tier 3 — verbose keymap drops to compact. arg 50 → outer 52, tier 2 needs
// outer >= 89 (no), tier 3 needs outer >= 41 (yes).
func TestComposeChromeLine_Arg50Tier3CompactKeymapPresent(t *testing.T) {
	const arg = 50
	got := ccl(arg, testWindowName)
	if strings.Contains(got, "win:") {
		t.Errorf("arg %d: expected NO 'win:' segment (tier 3); got=%q", arg, got)
	}
	if !strings.Contains(got, compactKeymap) {
		t.Errorf("arg %d: expected compactKeymap %q present (tier 3); got=%q", arg, compactKeymap, got)
	}
	if strings.Contains(got, "next pane") {
		t.Errorf("arg %d: expected verbose token 'next pane' absent (tier 3); got=%q", arg, got)
	}
}

// Tier 4 — chrome dropped; corners + filler only. arg 13 → outer 15.
func TestComposeChromeLine_Arg13Tier4FifteenCellTopEdge(t *testing.T) {
	const arg = 13
	got := ccl(arg, testWindowName)
	if strings.Contains(got, "Window ") {
		t.Errorf("arg %d: expected NO 'Window ' segment (tier 4); got=%q", arg, got)
	}
	if strings.Contains(got, "win:") {
		t.Errorf("arg %d: expected NO 'win:' segment (tier 4); got=%q", arg, got)
	}
	if strings.Contains(got, verboseKeymap) || strings.Contains(got, compactKeymap) {
		t.Errorf("arg %d: expected NO keymap (tier 4); got=%q", arg, got)
	}
	border := lipgloss.RoundedBorder()
	want := border.TopLeft + strings.Repeat(border.Top, 13) + border.TopRight
	if got != want {
		t.Errorf("arg %d: got=%q want=%q", arg, got, want)
	}
}

// Tier 4 degenerate width: arg 2 → outer 4 → ╭──╮.
func TestComposeChromeLine_Arg2Tier4FourCellTopEdge(t *testing.T) {
	const arg = 2
	got := ccl(arg, testWindowName)
	border := lipgloss.RoundedBorder()
	want := border.TopLeft + strings.Repeat(border.Top, 2) + border.TopRight
	if got != want {
		t.Errorf("arg %d: got=%q want=%q", arg, got, want)
	}
}

// Tier 4 degenerate width: arg 1 → outer 3 → ╭─╮.
func TestComposeChromeLine_Arg1Tier4ThreeCellTopEdge(t *testing.T) {
	const arg = 1
	got := ccl(arg, testWindowName)
	border := lipgloss.RoundedBorder()
	want := border.TopLeft + strings.Repeat(border.Top, 1) + border.TopRight
	if got != want {
		t.Errorf("arg %d: got=%q want=%q", arg, got, want)
	}
}

// Tier 4 degenerate width: arg 0 → outer 2 → ╭╮.
func TestComposeChromeLine_Arg0Tier4TwoCellTopEdge(t *testing.T) {
	const arg = 0
	got := ccl(arg, testWindowName)
	border := lipgloss.RoundedBorder()
	want := border.TopLeft + border.TopRight
	if got != want {
		t.Errorf("arg %d: got=%q want=%q", arg, got, want)
	}
}

// composeChromeLineParts — property test: left + chrome + right exactly
// equals composeChromeLine across every cascade threshold.
func TestComposeChromeLineParts_ConcatenationEqualsComposeChromeLineAtAllThresholds(t *testing.T) {
	for _, arg := range []int{0, 1, 2, 5, 13, 25, 40, 50, 60, 87, 95, 102, 103, 200} {
		full := ccl(arg, testWindowName)
		left, chrome, right := composeChromeLineParts(arg, 0, 1, 0, 1, testWindowName)
		got := left + chrome + right
		if got != full {
			t.Errorf("arg %d: left+chrome+right=%q; composeChromeLine=%q", arg, got, full)
		}
	}
}

// composeChromeLineParts — at tier 4 args, chrome is empty.
func TestComposeChromeLineParts_Tier4ReturnsEmptyChrome(t *testing.T) {
	for _, arg := range []int{0, 1, 2, 5, 13, 25} {
		_, chrome, _ := composeChromeLineParts(arg, 0, 1, 0, 1, testWindowName)
		if chrome != "" {
			t.Errorf("arg %d (tier 4): expected empty chrome; got=%q", arg, chrome)
		}
	}
}

// composeChromeLineParts — at a non-truncated tier 1, the chrome region's
// display-cell width matches the chrome substring inside composeChromeLine.
func TestComposeChromeLineParts_Tier1ChromeRegionWidthMatchesComposeChromeLineChromeRegion(t *testing.T) {
	const arg = 200
	_, chrome, _ := composeChromeLineParts(arg, 0, 1, 0, 1, testWindowName)
	full := ccl(arg, testWindowName)
	if !strings.Contains(full, chrome) {
		t.Errorf("arg %d: full output does not contain chrome substring; chrome=%q full=%q", arg, chrome, full)
	}
	if chrome == "" {
		t.Errorf("arg %d: expected non-empty chrome on tier 1; got empty", arg)
	}
	// Sanity check: chrome region width matches what tier 1 builds.
	wantChrome := "Window 1 of 1 · Pane 1 of 1" + " · win: " + testWindowName + " " + verboseKeymap
	if chrome != wantChrome {
		t.Errorf("arg %d: chrome=%q want=%q", arg, chrome, wantChrome)
	}
}
