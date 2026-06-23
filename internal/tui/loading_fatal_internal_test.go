package tui

// Task spectrum-tui-design-5-6 — fatal cold-boot error frame render (§10.5).
//
// Internal (package tui) tests for the loading-page error overlay: the failed
// step's row carries the ✗ glyph in state.red, the steps before it stay done (✓),
// the steps after stay pending (·), and the one-line message renders in state.red.
// They drive renderLoadingScreen directly with a failed view so the colour
// sequences (which need the package-internal tokenFgSeq) are assertable.

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// failedMidProgress builds the accumulator at "steps 1–2 done, fatal pending at
// step 3 (Registered hooks)" and marks that label failed for the error frame.
func failedAtRegisteredHooks() LoadingProgressView {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	p = p.Apply(BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"})
	return p.FailedView(3, "Portal failed to set @portal-restoring marker: permission denied")
}

// TestErrorFrame_FailedRowIsRedCross asserts the failed step row carries the ✗
// glyph painted state.red, and the one-line message is painted state.red.
func TestErrorFrame_FailedRowIsRedCross(t *testing.T) {
	view := failedAtRegisteredHooks()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, false)
	visible := ansi.Strip(out)

	if !strings.Contains(visible, loadingGlyphFailed) {
		t.Errorf("error frame missing the ✗ failed glyph:\n%s", visible)
	}
	// The failed row's ✗ glyph + label + the message line all carry state.red.
	redSeq := tokenFgSeq(t, theme.MV.StateRed, theme.Dark)
	if !strings.Contains(out, redSeq) {
		t.Errorf("error frame did not paint anything state.red:\n%q", out)
	}
	// The failed label is "Registered hooks" (step 3 → that group).
	if !strings.Contains(visible, LabelRegisteredHooks) {
		t.Errorf("error frame missing the failed label:\n%s", visible)
	}
	// The message renders verbatim.
	if !strings.Contains(visible, "Portal failed to set @portal-restoring marker") {
		t.Errorf("error frame missing the one-line message:\n%s", visible)
	}
}

// TestErrorFrame_StepStatesAroundFailure asserts steps before the failure stay
// done (✓) and steps after stay pending (·) — they never ran.
func TestErrorFrame_StepStatesAroundFailure(t *testing.T) {
	view := failedAtRegisteredHooks()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, false)
	visible := ansi.Strip(out)

	// "Started tmux server" (step 1) completed before the failure → ✓ done.
	startedRow := rowContaining(t, visible, LabelStartedTmuxServer)
	if !strings.Contains(startedRow, loadingGlyphDone) {
		t.Errorf("pre-failure label %q not done (✓): %q", LabelStartedTmuxServer, startedRow)
	}
	// "Replaying scrollback" is after the failure → · pending (never ran).
	pendingRow := rowContaining(t, visible, LabelReplayingScrollback)
	if !strings.Contains(pendingRow, loadingGlyphPending) {
		t.Errorf("post-failure label %q not pending (·): %q", LabelReplayingScrollback, pendingRow)
	}
}

// TestErrorFrame_NeverOverflowsHeight asserts the §10.5 error frame degrades
// (§2.7) and never overflows the height budget — the spacer/hint/message footer is
// shed in priority order on a short terminal so the frame is always ≤ h (the red ✗
// on the failed step row still carries the failure even when the footer fully
// degrades). This pins the height-budgeted footer regression: folding a 3-row
// footer into the list floor must not push the irreducible bar+list floor past h.
func TestErrorFrame_NeverOverflowsHeight(t *testing.T) {
	view := failedAtRegisteredHooks()
	for _, dims := range [][2]int{{120, 40}, {80, 24}, {40, 12}, {30, 8}, {24, 7}, {20, 6}} {
		out := renderLoadingScreen(view, dims[0], dims[1], theme.Dark, false)
		if h := lipgloss.Height(out); h > dims[1] {
			t.Errorf("%dx%d: error frame height %d exceeds %d (overflow)", dims[0], dims[1], h, dims[1])
		}
	}
}

// rowContaining returns the single rendered line containing the needle.
func rowContaining(t *testing.T, block, needle string) string {
	t.Helper()
	for _, line := range strings.Split(block, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", needle, block)
	return ""
}
