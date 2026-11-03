package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
)

func TestMultiSelectHeader_CountVioletCancelDetail(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		header := renderMultiSelectHeader(3, sectionHeaderWidth, th, false)

		if !strings.Contains(header, "3 selected") {
			t.Errorf("banner missing the %q cluster:\n%s", "3 selected", header)
		}
		if !strings.Contains(header, multiSelectCancelHint) {
			t.Errorf("banner missing the %q hint:\n%s", multiSelectCancelHint, header)
		}
		violetRun := headerStyle(th.AccentPrimary, th, false).Render("3 selected")
		if !strings.Contains(header, violetRun) {
			t.Errorf("banner missing the accent.primary %q run:\n%s", "3 selected", header)
		}
		detailRun := headerStyle(th.TextMuted, th, false).Render(multiSelectCancelHint)
		if !strings.Contains(header, detailRun) {
			t.Errorf("banner missing the text.muted %q run:\n%s", multiSelectCancelHint, header)
		}
	})
}

func TestMultiSelectHeader_RightAlignedCancelHint(t *testing.T) {
	header := renderMultiSelectHeader(2, sectionHeaderWidth, testDarkTheme(t), false)

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

func TestMultiSelectHeader_ExactlyOneRow(t *testing.T) {
	for _, count := range []int{0, 1, 42} {
		header := renderMultiSelectHeader(count, sectionHeaderWidth, testDarkTheme(t), false)
		if got := lipgloss.Height(header); got != 1 {
			t.Errorf("banner for count %d height = %d, want exactly 1 row:\n%s", count, got, header)
		}
	}
}

func TestMultiSelectHeader_ZeroSelected(t *testing.T) {
	header := renderMultiSelectHeader(0, sectionHeaderWidth, testDarkTheme(t), false)
	if !strings.Contains(ansi.Strip(header), "0 selected") {
		t.Errorf("banner for N=0 must read %q:\n%s", "0 selected", ansi.Strip(header))
	}
	violetRun := headerStyle(testDarkTheme(t).AccentPrimary, testDarkTheme(t), false).Render("0 selected")
	if !strings.Contains(header, violetRun) {
		t.Errorf("N=0 banner missing the accent.primary %q run:\n%s", "0 selected", header)
	}
	if !strings.Contains(header, multiSelectCancelHint) {
		t.Errorf("N=0 banner missing the %q hint:\n%s", multiSelectCancelHint, header)
	}
}

func TestMultiSelectHeader_NarrowDegradeDropsHint(t *testing.T) {
	wide := renderMultiSelectHeader(3, sectionHeaderWidth, testDarkTheme(t), false)
	if !strings.Contains(wide, multiSelectCancelHint) {
		t.Fatalf("wide banner missing the hint:\n%s", wide)
	}

	const narrow = 14
	narrowHeader := renderMultiSelectHeader(3, narrow, testDarkTheme(t), false)
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

func TestMultiSelectHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderMultiSelectHeader(3, sectionHeaderWidth, testDarkTheme(t), true)

	if !strings.Contains(header, "3 selected") || !strings.Contains(header, multiSelectCancelHint) {
		t.Errorf("colourless banner dropped structure:\n%s", header)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(header, seq) {
		t.Errorf("colourless banner still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).AccentPrimary, testDarkTheme(t).TextMuted} {
		if seq := tokenFgSeq(t, tok); strings.Contains(header, seq) {
			t.Errorf("colourless banner still emits a foreground role sequence %q", seq)
		}
	}
}

func TestMultiSelectHeader_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		header := renderMultiSelectHeader(3, sectionHeaderWidth, th, false)
		if seq := canvasSeq(t, th); !strings.Contains(header, seq) {
			t.Errorf("banner does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	}
}

func multiSelectBannerModel(marked []string, names ...string) Model {
	sessions := make([]Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, Session{Name: n, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	m.multiSelectMode = true
	m.selectedSessions = markedSet(marked...)
	return m
}

func bannerFirstLine(m Model) string {
	out := m.applySectionHeader(m.sessionList.View())
	return strings.SplitN(out, "\n", 2)[0]
}

func TestApplySectionHeader_MultiSelectShowsBanner(t *testing.T) {
	m := multiSelectBannerModel([]string{"alpha", "bravo", "charlie"}, "alpha", "bravo", "charlie")

	first := bannerFirstLine(m)
	if !strings.Contains(ansi.Strip(first), "3 selected") {
		t.Errorf("multi-select section-header row must read %q:\n%s", "3 selected", ansi.Strip(first))
	}
	if strings.Contains(first, "Sessions") {
		t.Errorf("multi-select section-header row must NOT show the standard %q header:\n%s", "Sessions", ansi.Strip(first))
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); !strings.Contains(first, seq) {
		t.Errorf("banner count cluster missing the accent.primary fg %q:\n%s", seq, first)
	}
}

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
	if strings.Contains(ansi.Strip(first), filterPromptPrefix+"al") {
		t.Errorf("FilterApplied + multi-select must NOT render the locked query header:\n%s", ansi.Strip(first))
	}
}

func TestApplySectionHeader_CountUpdatesLive(t *testing.T) {
	m := NewModelWithSessions([]Session{
		{Name: "alpha"},
		{Name: "bravo"},
	})
	m.termWidth = 80
	m.termHeight = 24

	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Fatalf("after entering on a session row the banner must read %q:\n%s", "1 selected", got)
	}

	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "0 selected") {
		t.Errorf("after unmarking the banner must read %q:\n%s", "0 selected", got)
	}

	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Errorf("after re-marking the banner must read %q:\n%s", "1 selected", got)
	}

	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "2 selected") {
		t.Errorf("after a second toggle the banner must read %q:\n%s", "2 selected", got)
	}
}

func TestApplySectionHeader_ByTagMultiMembershipCountsOnce(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
	sessions := []Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()

	rows := sessionRowIndices(m.sessionList.Items())
	if len(rows) != 2 {
		t.Fatalf("precondition: multi-tag session must span 2 rows; got %d", len(rows))
	}

	m.sessionList.Select(rows[0])
	m = pressSession(t, m, pressM)

	if got := ansi.Strip(bannerFirstLine(m)); !strings.Contains(got, "1 selected") {
		t.Errorf("a multi-tag session marked once must count as 1 in the banner:\n%s", got)
	}
}

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
