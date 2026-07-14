package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Tests for task 9-4: the §2.7 narrow-degrade fitter is unified across the standard
// keymap footer (fitLeftCluster) and the per-glyph filter footer (fitFilterCluster)
// into the shared fitClusterToWidth helper. Both fitters must produce byte-identical
// output to the pre-refactor algorithm across every width regime (wide/full,
// narrow-degrade prefix+`· …`, ellipsis-only, and sub-ellipsis empty).
//
// No t.Parallel() — the package's shared canvas/mock helpers make parallelism unsafe.

// referenceFitCluster is an independent copy of the pre-refactor narrow-degrade
// algorithm (try full → greedy leading prefix with `<cluster> sep …` → bare ellipsis
// → empty). The through-fitter tests below assert each production fitter matches this
// reference byte-for-byte, pinning the caller wiring (budget + renderer + sep/ellipsis)
// as well as the shared loop.
func referenceFitCluster(count, budget int, render func(n int) (string, int), sep, ellipsis string) (string, int) {
	if full, fullWidth := render(count); fullWidth <= budget {
		return full, fullWidth
	}
	sepWidth := lipgloss.Width(sep)
	ellipsisWidth := lipgloss.Width(ellipsis)
	best := ""
	bestWidth := 0
	for n := 1; n <= count; n++ {
		cluster, clusterWidth := render(n)
		candidateWidth := clusterWidth + sepWidth + ellipsisWidth
		if candidateWidth > budget {
			break
		}
		best = lipgloss.JoinHorizontal(lipgloss.Top, cluster, sep, ellipsis)
		bestWidth = candidateWidth
	}
	if best != "" {
		return best, bestWidth
	}
	if ellipsisWidth <= budget {
		return ellipsis, ellipsisWidth
	}
	return "", 0
}

// TestFitClusterToWidth_AlgorithmAcrossWidthRegimes drives the shared helper directly
// with deterministic plain-string clusters (no ANSI) so the four width regimes and the
// exact fitted output are asserted crisply. Each pseudo-entry is 5 cells wide; sep is
// 3 cells (` · `) and the ellipsis 1 cell (`…`).
func TestFitClusterToWidth_AlgorithmAcrossWidthRegimes(t *testing.T) {
	const sep = " · "    // width 3
	const ellipsis = "…" // width 1
	const count = 4      // full cluster = 4 * 5 = 20 cells
	render := func(n int) (string, int) {
		s := strings.Repeat("X", n*5)
		return s, lipgloss.Width(s)
	}

	for _, tc := range []struct {
		name    string
		budget  int
		wantStr string
		wantWid int
	}{
		{
			name:    "wide budget returns the full cluster",
			budget:  100,
			wantStr: strings.Repeat("X", 20),
			wantWid: 20,
		},
		{
			// full(20) does not fit; n=2 gives 10+3+1=14 ≤ 15, n=3 gives 15+3+1=19 > 15.
			name:    "narrow budget returns leading prefix plus separator and ellipsis",
			budget:  15,
			wantStr: strings.Repeat("X", 10) + sep + ellipsis,
			wantWid: 14,
		},
		{
			name:    "ellipsis-only budget returns the bare ellipsis",
			budget:  1,
			wantStr: ellipsis,
			wantWid: 1,
		},
		{
			name:    "sub-ellipsis budget returns the empty cluster",
			budget:  0,
			wantStr: "",
			wantWid: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotStr, gotWid := fitClusterToWidth(count, tc.budget, render, sep, ellipsis)
			if gotStr != tc.wantStr {
				t.Errorf("fitClusterToWidth string = %q, want %q", gotStr, tc.wantStr)
			}
			if gotWid != tc.wantWid {
				t.Errorf("fitClusterToWidth width = %d, want %d", gotWid, tc.wantWid)
			}
			if gotWid > tc.budget {
				t.Errorf("fitClusterToWidth width = %d exceeds budget %d", gotWid, tc.budget)
			}
		})
	}
}

// TestFitClusterToWidth_EmptyClusterCount asserts the count==0 fast path: the empty
// cluster (width 0) fits any non-negative budget and is returned verbatim.
func TestFitClusterToWidth_EmptyClusterCount(t *testing.T) {
	render := func(n int) (string, int) { return "", 0 }
	got, gotWid := fitClusterToWidth(0, 10, render, " · ", "…")
	if got != "" || gotWid != 0 {
		t.Errorf("fitClusterToWidth(0, ...) = (%q, %d), want (%q, 0)", got, gotWid, "")
	}
}

// TestFitFilterCluster_MatchesSharedHelperAcrossWidths asserts the multi-select
// (per-glyph) fitter delegates to the shared narrow-degrade helper with a full-width
// budget and the renderFilterCluster renderer — byte-identical to the reference
// algorithm across every width regime, with the returned width never exceeding the
// budget (the full width).
func TestFitFilterCluster_MatchesSharedHelperAcrossWidths(t *testing.T) {
	const mode = theme.Dark
	const colourless = false
	entries := multiSelectFooterEntries()

	sep := renderFooterDetail(footerEntrySeparator, mode, colourless)
	ellipsis := renderFooterDetail(footerEllipsis, mode, colourless)
	render := func(n int) (string, int) {
		cluster := renderFilterCluster(entries[:n], mode, colourless)
		return cluster, lipgloss.Width(cluster)
	}
	fullWidth := lipgloss.Width(renderFilterCluster(entries, mode, colourless))

	for _, tc := range []struct {
		name string
		w    int
	}{
		{"wide full cluster", fullWidth + 20},
		{"narrow degrade prefix plus ellipsis", 30},
		{"ellipsis only", 1},
		{"sub-ellipsis empty", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// fitFilterCluster's budget is the full width (no right anchor reserved).
			wantStr, wantWid := referenceFitCluster(len(entries), tc.w, render, sep, ellipsis)
			gotStr, gotWid := fitFilterCluster(entries, tc.w, mode, colourless)
			if gotStr != wantStr {
				t.Errorf("fitFilterCluster string diverged from pre-refactor output:\ngot  %q\nwant %q", gotStr, wantStr)
			}
			if gotWid != wantWid {
				t.Errorf("fitFilterCluster width = %d, want %d (pre-refactor)", gotWid, wantWid)
			}
			if gotWid > tc.w {
				t.Errorf("fitFilterCluster width = %d exceeds budget %d", gotWid, tc.w)
			}
		})
	}
}

// TestFitLeftCluster_MatchesSharedHelperAcrossWidths asserts the standard keymap
// fitter delegates to the shared narrow-degrade helper with its right-anchor-reserved
// budget (full width minus the right anchor plus one spacer) and the renderFooterCluster
// renderer — byte-identical to the reference algorithm across every width regime, with
// the returned width never exceeding the reserved budget.
func TestFitLeftCluster_MatchesSharedHelperAcrossWidths(t *testing.T) {
	const mode = theme.Dark
	const colourless = false
	core, _ := splitFooterEntries(sessionsKeymap())

	sep := renderFooterDetail(footerEntrySeparator, mode, colourless)
	ellipsis := renderFooterDetail(footerEllipsis, mode, colourless)
	render := func(n int) (string, int) {
		cluster := renderFooterCluster(core[:n], mode, colourless)
		return cluster, lipgloss.Width(cluster)
	}
	fullWidth := lipgloss.Width(renderFooterCluster(core, mode, colourless))

	for _, tc := range []struct {
		name       string
		w          int
		rightWidth int
	}{
		{"wide full cluster no right anchor", fullWidth + 20, 0},
		{"narrow degrade with reserved right anchor", 60, 6},
		{"ellipsis only", 1, 0},
		{"sub-ellipsis empty", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := tc.w
			if tc.rightWidth > 0 {
				budget = tc.w - tc.rightWidth - 1
			}
			if budget < 0 {
				budget = 0
			}
			wantStr, wantWid := referenceFitCluster(len(core), budget, render, sep, ellipsis)
			gotStr, gotWid := fitLeftCluster(core, tc.w, tc.rightWidth, mode, colourless)
			if gotStr != wantStr {
				t.Errorf("fitLeftCluster string diverged from pre-refactor output:\ngot  %q\nwant %q", gotStr, wantStr)
			}
			if gotWid != wantWid {
				t.Errorf("fitLeftCluster width = %d, want %d (pre-refactor)", gotWid, wantWid)
			}
			if gotWid > budget {
				t.Errorf("fitLeftCluster width = %d exceeds budget %d", gotWid, budget)
			}
		})
	}
}
