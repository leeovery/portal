package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §5 multi-select banner: a filter-line analogue that REPLACES the standard
// `Sessions ··· N` section header while multi-select mode is active — `N selected`
// (accent.violet) on the left, a right-aligned `esc cancel` hint (text.detail) on
// the same row, the gap filled with a canvas-painted flex spacer to the content
// width. NO `▌` left-bar (it is a section-header analogue, not a notice band).
// These tests pin the colour roles, the right-alignment, the N=0 render, the
// single-row height, the §2.7 narrow degrade, and the NO_COLOR carve-out.
//
// No t.Parallel() — the package-level mock convention and shared canvas helpers
// make parallelism unsafe across this package's tests.

// TestMultiSelectHeader_CountVioletCancelDetail asserts the banner renders the
// `N selected` cluster in accent.violet and the `esc cancel` hint in text.detail
// (the SAME dim chrome token the standard `/ to filter` hint uses), on both the
// dark and light canvas.
func TestMultiSelectHeader_CountVioletCancelDetail(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := renderMultiSelectHeader(3, sectionHeaderWidth, tc.mode, false)

			if !strings.Contains(header, "3 selected") {
				t.Errorf("banner missing the %q cluster:\n%s", "3 selected", header)
			}
			if !strings.Contains(header, multiSelectCancelHint) {
				t.Errorf("banner missing the %q hint:\n%s", multiSelectCancelHint, header)
			}
			// The `N selected` cluster is accent.violet, the `esc cancel` hint is
			// text.detail — assert the exact styled runs appear verbatim.
			violetRun := headerStyle(theme.MV.AccentViolet, tc.mode, false).Render("3 selected")
			if !strings.Contains(header, violetRun) {
				t.Errorf("banner missing the accent.violet %q run:\n%s", "3 selected", header)
			}
			detailRun := headerStyle(theme.MV.TextDetail, tc.mode, false).Render(multiSelectCancelHint)
			if !strings.Contains(header, detailRun) {
				t.Errorf("banner missing the text.detail %q run:\n%s", multiSelectCancelHint, header)
			}
		})
	}
}

// TestMultiSelectHeader_RightAlignedCancelHint asserts the `esc cancel` hint is
// right-aligned (the left cluster and the hint are separated by a flex spacer to
// the content width) and the single rendered row is exactly the content width.
func TestMultiSelectHeader_RightAlignedCancelHint(t *testing.T) {
	header := renderMultiSelectHeader(2, sectionHeaderWidth, theme.Dark, false)

	countIdx := strings.Index(header, "2 selected")
	hintIdx := strings.LastIndex(header, multiSelectCancelHint)
	if countIdx < 0 || hintIdx < 0 {
		t.Fatalf("banner missing a cluster: countIdx=%d hintIdx=%d\n%s", countIdx, hintIdx, header)
	}
	if hintIdx < countIdx {
		t.Errorf("hint (idx %d) appears before the count cluster (idx %d); must be right-aligned", hintIdx, countIdx)
	}
	if got := lipgloss.Width(header); got != sectionHeaderWidth {
		t.Errorf("banner width = %d, want exactly %d (flex spacer to content width)", got, sectionHeaderWidth)
	}
}

// TestMultiSelectHeader_ExactlyOneRow asserts the banner is exactly one rendered
// row — it REPLACES the section-header row and must not perturb the one-row-per-
// delegate pagination budget (§3.5).
func TestMultiSelectHeader_ExactlyOneRow(t *testing.T) {
	for _, count := range []int{0, 1, 42} {
		header := renderMultiSelectHeader(count, sectionHeaderWidth, theme.Dark, false)
		if got := lipgloss.Height(header); got != 1 {
			t.Errorf("banner for count %d height = %d, want exactly 1 row:\n%s", count, got, header)
		}
	}
}

// TestMultiSelectHeader_ZeroSelected asserts the N=0 render: the banner shows
// `0 selected` (the banner renders even with an empty set — it is a mode
// affordance, not a count-gated element).
func TestMultiSelectHeader_ZeroSelected(t *testing.T) {
	header := renderMultiSelectHeader(0, sectionHeaderWidth, theme.Dark, false)
	if !strings.Contains(ansi.Strip(header), "0 selected") {
		t.Errorf("banner for N=0 must read %q:\n%s", "0 selected", ansi.Strip(header))
	}
	// Still accent.violet, still right-anchored with the hint.
	violetRun := headerStyle(theme.MV.AccentViolet, theme.Dark, false).Render("0 selected")
	if !strings.Contains(header, violetRun) {
		t.Errorf("N=0 banner missing the accent.violet %q run:\n%s", "0 selected", header)
	}
	if !strings.Contains(header, multiSelectCancelHint) {
		t.Errorf("N=0 banner missing the %q hint:\n%s", multiSelectCancelHint, header)
	}
}

// TestMultiSelectHeader_NarrowDegradeDropsHint asserts the §2.7 narrow degrade:
// below the width at which the left cluster + a spacer + the hint fit, the right
// `esc cancel` hint drops and the row never overflows — matching the standard
// section header's degrade exactly (both route through the shared right-anchor
// core).
func TestMultiSelectHeader_NarrowDegradeDropsHint(t *testing.T) {
	// Wide: hint present.
	wide := renderMultiSelectHeader(3, sectionHeaderWidth, theme.Dark, false)
	if !strings.Contains(wide, multiSelectCancelHint) {
		t.Fatalf("wide banner missing the hint:\n%s", wide)
	}

	// Narrow: a width that cannot hold `N selected` + a spacer + `esc cancel`.
	const narrow = 14
	narrowHeader := renderMultiSelectHeader(3, narrow, theme.Dark, false)
	if strings.Contains(narrowHeader, multiSelectCancelHint) {
		t.Errorf("narrow banner at width %d still shows the %q hint (degrade failed):\n%s", narrow, multiSelectCancelHint, narrowHeader)
	}
	if !strings.Contains(ansi.Strip(narrowHeader), "3 selected") {
		t.Errorf("narrow banner dropped the count cluster:\n%s", ansi.Strip(narrowHeader))
	}
	for i, line := range strings.Split(narrowHeader, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow banner line %d width = %d (overflow, want <= %d)", i, lw, narrow)
		}
	}
}

// TestMultiSelectHeader_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless banner carries no canvas background SGR and no foreground
// hue — the `N selected` / `esc cancel` text survives on the terminal's native
// fg/bg.
func TestMultiSelectHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderMultiSelectHeader(3, sectionHeaderWidth, theme.Dark, true)

	if !strings.Contains(header, "3 selected") || !strings.Contains(header, multiSelectCancelHint) {
		t.Errorf("colourless banner dropped structure:\n%s", header)
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless banner still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.AccentViolet, theme.MV.TextDetail} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless banner still emits a foreground role sequence %q", seq)
		}
	}
}

// TestMultiSelectHeader_PaintsCanvasNoEdgeBleed asserts the banner cells carry the
// owned canvas background (leaf .Background(canvas)) so the right-aligned spacer
// gap is canvas-painted, not a terminal-bg island.
func TestMultiSelectHeader_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		header := renderMultiSelectHeader(3, sectionHeaderWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(header, seq) {
			t.Errorf("banner does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	}
}

// multiSelectBannerModel builds a Sessions-page model seeded with the given
// session names at 80x24, entered into multi-select mode with the named sessions
// marked, so applySectionHeader renders the banner deterministically.
func multiSelectBannerModel(marked []string, names ...string) Model {
	sessions := make([]tmux.Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, tmux.Session{Name: n, Windows: 1, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	m.multiSelectMode = true
	m.selectedSessions = markedSet(marked...)
	return m
}

// bannerFirstLine returns the first (section-header) line of applySectionHeader's
// output — the line the banner / section header / filter-query header swap into.
func bannerFirstLine(m Model) string {
	out := m.applySectionHeader(m.sessionList.View())
	return strings.SplitN(out, "\n", 2)[0]
}

// TestApplySectionHeader_MultiSelectShowsBanner asserts that in multi-select mode
// the section-header row swaps to the `N selected` banner (accent.violet), and the
// standard `Sessions` section header is NOT shown on that row.
func TestApplySectionHeader_MultiSelectShowsBanner(t *testing.T) {
	m := multiSelectBannerModel([]string{"alpha", "bravo", "charlie"}, "alpha", "bravo", "charlie")

	first := bannerFirstLine(m)
	if !strings.Contains(ansi.Strip(first), "3 selected") {
		t.Errorf("multi-select section-header row must read %q:\n%s", "3 selected", ansi.Strip(first))
	}
	if strings.Contains(first, "Sessions") {
		t.Errorf("multi-select section-header row must NOT show the standard %q header:\n%s", "Sessions", ansi.Strip(first))
	}
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); !strings.Contains(first, seq) {
		t.Errorf("banner count cluster missing the accent.violet fg %q:\n%s", seq, first)
	}
}

// TestApplySectionHeader_MultiSelectZero asserts the N=0 case: a zero-selected set
// (reachable via double-`m` or Esc-then-reenter) still swaps in the `0 selected`
// banner (the banner shows even with an empty selection).
func TestApplySectionHeader_MultiSelectZero(t *testing.T) {
	m := multiSelectBannerModel(nil, "alpha", "bravo")

	first := bannerFirstLine(m)
	if !strings.Contains(ansi.Strip(first), "0 selected") {
		t.Errorf("N=0 multi-select section-header row must read %q:\n%s", "0 selected", ansi.Strip(first))
	}
	if strings.Contains(first, "Sessions") {
		t.Errorf("N=0 multi-select row must NOT show the standard %q header:\n%s", "Sessions", ansi.Strip(first))
	}
}

// TestApplySectionHeader_FilteringOwnsRowInMultiSelect asserts the precedence: while
// the filter input is focused (FilterState == Filtering) the banner steps aside —
// applySectionHeader returns the list view untouched so the live filter input owns
// the row (the banner never overwrites what the user is typing), even in the mode.
func TestApplySectionHeader_FilteringOwnsRowInMultiSelect(t *testing.T) {
	m := multiSelectBannerModel([]string{"alpha"}, "alpha", "bravo")
	m.sessionList.SetFilterState(list.Filtering)

	listView := m.sessionList.View()
	got := m.applySectionHeader(listView)
	if got != listView {
		t.Errorf("Filtering must leave the list view untouched (filter input owns the row); banner leaked in:\n%s", got)
	}
	if strings.Contains(ansi.Strip(bannerFirstLine(m)), "1 selected") {
		t.Errorf("banner must NOT render while the filter input is focused:\n%s", ansi.Strip(bannerFirstLine(m)))
	}
}

// TestApplySectionHeader_FilterAppliedInMultiSelectShowsBanner asserts the branch
// order: a committed/applied filter WHILE in multi-select shows the BANNER, not the
// locked query header — the multi-select branch precedes the FilterApplied branch.
func TestApplySectionHeader_FilterAppliedInMultiSelectShowsBanner(t *testing.T) {
	m := multiSelectBannerModel([]string{"alpha", "bravo"}, "alpha", "bravo")
	m.SetSessionListFilter("al")
	if m.sessionList.FilterState() != list.FilterApplied {
		t.Fatalf("precondition: filter must be applied, got %v", m.sessionList.FilterState())
	}

	first := bannerFirstLine(m)
	if !strings.Contains(ansi.Strip(first), "2 selected") {
		t.Errorf("FilterApplied + multi-select must show the %q banner, not the query header:\n%s", "2 selected", ansi.Strip(first))
	}
	// The locked `/ ` query prompt must NOT own the row in the mode.
	if strings.Contains(ansi.Strip(first), filterPromptPrefix+"al") {
		t.Errorf("FilterApplied + multi-select must NOT render the locked query header:\n%s", ansi.Strip(first))
	}
}

// TestApplySectionHeader_CountUpdatesLive asserts the count updates live: each `m`
// toggle changes the rendered banner number by exactly 1.
func TestApplySectionHeader_CountUpdatesLive(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})
	m.termWidth = 80
	m.termHeight = 24

	// Enter the mode on a session row: mark-on-entry marks the highlighted alpha → 1 selected.
	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Fatalf("after entering on a session row the banner must read %q:\n%s", "1 selected", got)
	}

	// Toggle the highlighted alpha OFF (double-`m`): 0 selected.
	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "0 selected") {
		t.Errorf("after unmarking the banner must read %q:\n%s", "0 selected", got)
	}

	// Toggle alpha back ON: 1 selected.
	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Errorf("after re-marking the banner must read %q:\n%s", "1 selected", got)
	}

	// Move to bravo and toggle it ON: 2 selected.
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "2 selected") {
		t.Errorf("after a second toggle the banner must read %q:\n%s", "2 selected", got)
	}
}

// TestApplySectionHeader_ByTagMultiMembershipCountsOnce asserts the distinct-
// session count contract at the banner level: a By-Tag session that spans several
// rows, marked via one of them, contributes exactly 1 to the banner count
// (len(m.selectedSessions), keyed on Session.Name).
func TestApplySectionHeader_ByTagMultiMembershipCountsOnce(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()

	rows := sessionRowIndices(m.sessionList.Items())
	if len(rows) != 2 {
		t.Fatalf("precondition: multi-tag session must span 2 rows; got %d", len(rows))
	}

	// Enter with the cursor on the first of the two rows: mark-on-entry marks the
	// multi-tag session once (keyed on Session.Name).
	m.sessionList.Select(rows[0])
	m = pressSession(t, m, pressM)

	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Errorf("a multi-tag session marked once must count as 1 in the banner:\n%s", got)
	}
}

// TestActiveNoticeBand_SuppressesSignpostInMultiSelect asserts that while in
// multi-select mode the By-Tag "No tags yet" signpost is suppressed (does not own
// the slot) — the banner replaces the section header, and the signpost must not
// also render.
func TestActiveNoticeBand_SuppressesSignpostInMultiSelect(t *testing.T) {
	m := signpostModel(t)
	if _, _, ok := m.activeNoticeBand(); !ok {
		t.Fatalf("precondition: the signpost must own the slot outside multi-select mode")
	}

	m.multiSelectMode = true
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Errorf("multi-select mode must suppress the By-Tag signpost notice band")
	}
}

// TestActiveNoticeBand_FlashOutranksBannerInMultiSelect asserts a transient flash
// still owns the notice slot while in the mode — the flash arm stays FIRST, so a
// flash outranks both the banner and the (suppressed) signpost.
func TestActiveNoticeBand_FlashOutranksBannerInMultiSelect(t *testing.T) {
	m := signpostModel(t)
	m.multiSelectMode = true
	const flash = "session \"alpha\" no longer exists"
	m.setFlash(flash)

	role, message, ok := m.activeNoticeBand()
	if !ok {
		t.Fatalf("a transient flash must own the notice slot even in multi-select mode")
	}
	if message != flash {
		t.Errorf("flash message = %q, want %q", message, flash)
	}
	if role != bandWarning {
		t.Errorf("default flash role = %v, want bandWarning", role)
	}
}
