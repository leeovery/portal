package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// This file pins the shared right-anchored footer row assembler
// (assembleRightAnchoredRow) extracted from footerKeyRow and filterFooterRow
// (task 8-7). Both the standard condensed footer and the contextual filter
// footers route their final right-anchor layout (the fit test, the
// headerPadRight narrow-degrade, and the canvas flex-spacer join) through this
// single assembler, so a change to the right-anchor degrade rule is made once.
//
// The byte-identical guarantee is the load-bearing assertion: both footers must
// render identically to current output at WIDE widths AND at/below the
// narrow-degrade boundary (leftWidth+1+rightWidth > w).

// rightAnchoredCanvasWidth is a content width wide enough that both footers
// render their full left cluster + flex spacer + the ? help right anchor with no
// §2.7 truncation.
const rightAnchoredCanvasWidth = 120

// TestAssembleRightAnchoredRow_WideEmitsClusterSpacerAnchor asserts the WIDE
// path: when the right anchor fits beside the left cluster, the assembler emits
// the cluster, a canvas-painted flex spacer, then the right anchor — the row is
// exactly w cells wide and the anchor ends flush at the right edge.
func TestAssembleRightAnchoredRow_WideEmitsClusterSpacerAnchor(t *testing.T) {
	const w = rightAnchoredCanvasWidth
	left := renderFooterDetail("left", theme.Dark, false)
	leftWidth := lipgloss.Width(left)
	rightSeg := renderFooterDetail("? help", theme.Dark, false)
	rightWidth := lipgloss.Width(rightSeg)

	row := assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, theme.Dark, false)

	if got := lipgloss.Width(row); got != w {
		t.Errorf("wide row width = %d, want exactly %d", got, w)
	}
	vis := footerVisible(row)
	if !strings.HasPrefix(vis, "left") {
		t.Errorf("wide row must lead with the left cluster:\n%q", vis)
	}
	if !strings.HasSuffix(vis, "? help") {
		t.Errorf("wide row must end with the right anchor:\n%q", vis)
	}
	// The gap between cluster and anchor is a flex spacer wider than one cell.
	gap := strings.TrimSuffix(strings.TrimPrefix(vis, "left"), "? help")
	if len(gap) <= 1 {
		t.Errorf("wide row flex spacer too narrow (%d cells): %q", len(gap), vis)
	}
}

// TestAssembleRightAnchoredRow_NarrowDegradePadsLeftAndReturns asserts the
// narrow-degrade boundary (leftWidth+1+rightWidth > w): the assembler drops the
// right anchor and pads the left cluster to width via headerPadRight. The result
// must equal headerPadRight(left, leftWidth, w, ...) exactly (byte-identical
// degrade), and must NOT contain the right anchor.
func TestAssembleRightAnchoredRow_NarrowDegradePadsLeftAndReturns(t *testing.T) {
	left := renderFooterDetail("left", theme.Dark, false)
	leftWidth := lipgloss.Width(left)
	rightSeg := renderFooterDetail("? help", theme.Dark, false)
	rightWidth := lipgloss.Width(rightSeg)

	// A width at/below the degrade boundary: leftWidth+1+rightWidth > w.
	w := leftWidth + rightWidth // strictly less than leftWidth+1+rightWidth
	if leftWidth+1+rightWidth <= w {
		t.Fatalf("test setup: width %d is not at/below the degrade boundary", w)
	}

	row := assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, theme.Dark, false)

	want := headerPadRight(left, leftWidth, w, theme.Dark, false)
	if row != want {
		t.Errorf("narrow-degrade row != headerPadRight(left, …):\n got=%q\nwant=%q", row, want)
	}
	if strings.Contains(footerVisible(row), "? help") {
		t.Errorf("narrow-degrade row must drop the ? help anchor:\n%q", footerVisible(row))
	}
}

// TestAssembleRightAnchoredRow_NoRightAnchorPadsLeft asserts the rightSeg=="" arm:
// with no right anchor the assembler pads the left cluster to width regardless of
// the fit test (mirrors the original "no right entry" branch).
func TestAssembleRightAnchoredRow_NoRightAnchorPadsLeft(t *testing.T) {
	const w = rightAnchoredCanvasWidth
	left := renderFooterDetail("left", theme.Dark, false)
	leftWidth := lipgloss.Width(left)

	row := assembleRightAnchoredRow(left, leftWidth, "", 0, w, theme.Dark, false)

	want := headerPadRight(left, leftWidth, w, theme.Dark, false)
	if row != want {
		t.Errorf("no-anchor row != headerPadRight(left, …):\n got=%q\nwant=%q", row, want)
	}
}

// TestFooters_RouteThroughSharedAssembler_NarrowDegradeIdentical asserts the
// load-bearing byte-identical degrade guarantee at the narrow-degrade boundary:
// both the standard condensed footer AND the contextual filter footers route
// their final right-anchor layout through assembleRightAnchoredRow, so at a width
// that forces the degrade (leftWidth+1+rightWidth > w) BOTH drop the ? help anchor
// and pad the left cluster to width through the SHARED assembler.
//
// NOTE on scope: the filter footers have NO left-cluster fitting (fitLeftCluster
// is footer.go-specific and stays so per the task scope guard), so their left
// cluster can itself exceed w at very narrow widths — that pre-existing,
// out-of-scope behaviour is unchanged here. This test pins only the degrade that
// the assembler owns: at the boundary, BOTH footers drop the right anchor.
func TestFooters_RouteThroughSharedAssembler_NarrowDegradeIdentical(t *testing.T) {
	const mode = theme.Dark

	// The right anchor is the shared sessionsKeymap ? help, sourced identically by
	// both row functions; its rendered width derives the degrade boundary.
	core, helpEntry := splitFooterEntries(sessionsKeymap())
	if helpEntry == nil {
		t.Fatal("sessionsKeymap must carry the right-aligned ? help anchor")
	}
	rightWidth := lipgloss.Width(renderFooterEntry(*helpEntry, theme.MV.AccentViolet, mode, false))

	// Each footer is driven through assembleRightAnchoredRow with its OWN rendered
	// left cluster. To prove the SHARED degrade, feed every footer's left cluster
	// through the assembler at a width strictly below the degrade boundary
	// (leftWidth+1+rightWidth > w) and assert the assembler drops the anchor and
	// returns exactly headerPadRight(left, leftWidth, w, …) — byte-identical degrade
	// regardless of which footer's cluster it is.
	rightSeg := renderFooterEntry(*helpEntry, theme.MV.AccentViolet, mode, false)
	clusters := map[string]string{
		"standard":  renderFooterCluster(core, mode, false),
		"filtering": renderFilterCluster(filteringFooterEntries(), mode, false),
		"applied":   renderFilterCluster(filterAppliedFooterEntries(), mode, false),
	}
	for name, left := range clusters {
		leftWidth := lipgloss.Width(left)
		w := leftWidth + rightWidth // leftWidth+1+rightWidth > w (no spacer cell)
		if leftWidth+1+rightWidth <= w {
			t.Fatalf("[%s] setup width %d is not at/below the degrade boundary", name, w)
		}

		got := assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, mode, false)
		want := headerPadRight(left, leftWidth, w, mode, false)
		if got != want {
			t.Errorf("[%s w=%d] assembler degrade != headerPadRight(left, …):\n got=%q\nwant=%q", name, w, got, want)
		}
		if strings.Contains(footerVisible(got), "? help") {
			t.Errorf("[%s w=%d] assembler degrade must drop the ? help anchor:\n%q", name, w, footerVisible(got))
		}
	}

	// And the end-to-end render: at a tiny width every footer drops the ? help
	// anchor on its single key row (the degrade is reached through the render path,
	// not just the bare assembler). The standard footer additionally truncates its
	// fitted left cluster; the filter footers do not fit (out of scope) — but all
	// three drop the anchor.
	const tinyWidth = 6
	renders := map[string]string{
		"standard":  lastLine(renderSessionsFooter(tinyWidth, mode, false)),
		"filtering": lastLine(renderFilteringFooter(tinyWidth, mode, false)),
		"applied":   lastLine(renderFilterAppliedFooter(tinyWidth, mode, false)),
	}
	for name, row := range renders {
		if strings.Contains(footerVisible(row), "? help") {
			t.Errorf("[%s w=%d] tiny-width render must drop the ? help anchor:\n%q", name, tinyWidth, footerVisible(row))
		}
	}
}

// lastLine returns the final \n-separated line of a rendered footer (the key row;
// line 0 is the border.footer top rule).
func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}
