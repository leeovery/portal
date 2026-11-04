package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func failedRaisingRestoring() LoadingProgressView {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1})
	return p.FailedView(2, "Portal failed to raise the restoring flag: permission denied")
}

func TestErrorFrame_FailedRowIsRedCross(t *testing.T) {
	view := failedRaisingRestoring()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), false)
	visible := ansi.Strip(out)

	if !strings.Contains(visible, loadingGlyphFailed) {
		t.Errorf("error frame missing the ✗ failed glyph:\n%s", visible)
	}
	redSeq := tokenFgSeq(t, testDarkTheme(t).StateDestructive)
	if !strings.Contains(out, redSeq) {
		t.Errorf("error frame did not paint anything state.destructive:\n%q", out)
	}
	if !strings.Contains(visible, LabelStartingTheSaver) {
		t.Errorf("error frame missing the failed label:\n%s", visible)
	}
	if !strings.Contains(visible, "Portal failed to raise the restoring flag") {
		t.Errorf("error frame missing the one-line message:\n%s", visible)
	}
}

func TestErrorFrame_StepStatesAroundFailure(t *testing.T) {
	view := failedRaisingRestoring()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), false)
	visible := ansi.Strip(out)

	startedRow := rowContaining(t, visible, LabelCheckingZmx)
	if !strings.Contains(startedRow, loadingGlyphDone) {
		t.Errorf("pre-failure label %q not done (✓): %q", LabelCheckingZmx, startedRow)
	}
	pendingRow := rowContaining(t, visible, LabelRestoringSessions)
	if !strings.Contains(pendingRow, loadingGlyphPending) {
		t.Errorf("post-failure label %q not pending (·): %q", LabelRestoringSessions, pendingRow)
	}
}

func TestErrorFrame_NeverOverflowsHeight(t *testing.T) {
	view := failedRaisingRestoring()
	for _, dims := range [][2]int{{120, 40}, {80, 24}, {40, 12}, {30, 8}, {24, 7}, {20, 6}} {
		out := renderLoadingScreen(view, dims[0], dims[1], testDarkTheme(t), false)
		if h := lipgloss.Height(out); h > dims[1] {
			t.Errorf("%dx%d: error frame height %d exceeds %d (overflow)", dims[0], dims[1], h, dims[1])
		}
	}
}

func rowContaining(t *testing.T, block, needle string) string {
	t.Helper()
	for line := range strings.SplitSeq(block, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", needle, block)
	return ""
}
