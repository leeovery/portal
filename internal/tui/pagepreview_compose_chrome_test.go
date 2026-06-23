package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// These tests pin the §9.1 header compartment's width cascade at the spec's tier
// thresholds. The argument is the joined panel's content width (the body width);
// the header cascade fits within it. The cascade (re-styled from the prior single
// top-bar cascade — counters/session degrade, the hints moved to the footer):
//   - Tier 1: marker + full session + counters.
//   - Tier 2: marker + truncated session + counters.
//   - Tier 3: drop counters; marker + full session.
//   - Tier 4: drop counters; marker + hard-truncated session.

const testSessionName = "nvim-editor" // 11 display cells

// chl composes the §9.1 stripped header at content width w for the single-window
// single-pane counter fixture → counters "Window 1/1 · Pane 1/1".
func chl(w int, name string) string {
	return stripANSI(composePreviewHeaderRow(w, 0, 1, 0, 1, name, theme.Dark, false))
}

// TestComposePreviewHeaderRow_NoEmbeddedNewlines guards the single-row invariant
// across every cascade tier — the header is exactly one compartment row.
func TestComposePreviewHeaderRow_NoEmbeddedNewlines(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 25, 15, 10, 4, 1, 0} {
		got := composePreviewHeaderRow(w, 0, 1, 0, 1, testSessionName, theme.Dark, false)
		if n := strings.Count(got, "\n"); n != 0 {
			t.Errorf("composePreviewHeaderRow(width=%d) returned %d embedded newline(s); want 0; got=%q", w, n, got)
		}
	}
}

// TestComposePreviewHeaderRow_FitsWithinContentWidth pins that the cascaded
// header never exceeds the supplied content width at any tier — so the joined
// panel stays exactly the terminal width (the body, not the header, sets the
// width).
func TestComposePreviewHeaderRow_FitsWithinContentWidth(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 25, 18, 13, 12, 11, 8, 4, 2, 1} {
		got := composePreviewHeaderRow(w, 0, 1, 0, 1, testSessionName, theme.Dark, false)
		if width := lipgloss.Width(got); width > w {
			t.Errorf("content width %d: header width = %d, want <= %d; got=%q", w, width, w, stripANSI(got))
		}
	}
}

// Tier 1 — wide width: marker + full session + counters.
func TestComposePreviewHeaderRow_Tier1FullAtWideWidth(t *testing.T) {
	got := chl(200, testSessionName)
	for _, want := range []string{previewMarker, testSessionName, "Window 1/1 · Pane 1/1"} {
		if !strings.Contains(got, want) {
			t.Errorf("tier 1 wide: expected substring %q; got=%q", want, got)
		}
	}
	if strings.Contains(got, "…") {
		t.Errorf("tier 1 wide: expected no ellipsis on full-name tier; got=%q", got)
	}
}

// Tier 1 boundary — the smallest width at which everything fits whole.
func TestComposePreviewHeaderRow_Tier1BoundaryFullFit(t *testing.T) {
	counters := "Window 1/1 · Pane 1/1"
	fullW := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName) + 1 + lipgloss.Width(counters)
	got := chl(fullW, testSessionName)
	if !strings.Contains(got, testSessionName) {
		t.Errorf("tier 1 boundary w %d: expected full session %q; got=%q", fullW, testSessionName, got)
	}
	if strings.Contains(got, "…") {
		t.Errorf("tier 1 boundary w %d: expected no ellipsis; got=%q", fullW, got)
	}
	if !strings.Contains(got, counters) {
		t.Errorf("tier 1 boundary w %d: expected counters; got=%q", fullW, got)
	}
}

// Tier 2 — session truncated, counters retained.
func TestComposePreviewHeaderRow_Tier2TruncatesSessionKeepsCounters(t *testing.T) {
	counters := "Window 1/1 · Pane 1/1"
	fullW := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName) + 1 + lipgloss.Width(counters)
	w := fullW - 1 // one cell short of full session fit
	got := chl(w, testSessionName)
	if !strings.Contains(got, "…") {
		t.Errorf("tier 2 w %d: expected truncated session with ellipsis; got=%q", w, got)
	}
	if !strings.Contains(got, counters) {
		t.Errorf("tier 2 w %d: expected counters present; got=%q", w, got)
	}
}

// Tier 3 — counters dropped, marker + full session.
func TestComposePreviewHeaderRow_Tier3DropsCountersKeepsFullSession(t *testing.T) {
	// Width that cannot fit counters at tier 2 (session budget < min) but fits
	// marker + full session.
	w := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName)
	got := chl(w, testSessionName)
	if strings.Contains(got, "Window") {
		t.Errorf("tier 3 w %d: expected NO counters segment; got=%q", w, got)
	}
	if !strings.Contains(got, testSessionName) {
		t.Errorf("tier 3 w %d: expected session %q present; got=%q", w, testSessionName, got)
	}
	if strings.Contains(got, "…") {
		t.Errorf("tier 3 w %d: expected no ellipsis on full session; got=%q", w, got)
	}
}

// Tier 4 — counters dropped, session hard-truncated to fit.
func TestComposePreviewHeaderRow_Tier4TruncatesSessionNoCounters(t *testing.T) {
	// One cell below the tier-3 full-session fit forces a truncation, still no
	// counters.
	w := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName) - 1
	got := chl(w, testSessionName)
	if strings.Contains(got, "Window") {
		t.Errorf("tier 4 w %d: expected NO counters; got=%q", w, got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("tier 4 w %d: expected truncated session with ellipsis; got=%q", w, got)
	}
	if !strings.Contains(got, previewMarker) {
		t.Errorf("tier 4 w %d: expected marker present; got=%q", w, got)
	}
}

// TestComposePreviewHeaderRow_AlwaysCarriesMarker pins that the `◉ preview`
// marker is present at every non-degenerate width (the marker never drops — only
// counters and the session truncate).
func TestComposePreviewHeaderRow_AlwaysCarriesMarker(t *testing.T) {
	for _, w := range []int{200, 60, 30, 18, 13, 11} {
		got := chl(w, testSessionName)
		if !strings.Contains(got, previewMarker) {
			t.Errorf("width %d: header dropped the marker; got=%q", w, got)
		}
	}
}
