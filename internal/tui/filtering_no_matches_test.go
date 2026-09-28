package tui

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func enterNoMatches(t *testing.T, th theme.Theme, query string) Model {
	t.Helper()
	m := filteringTestModel(t, th)
	m = pressSlash(t, m)
	m = typeKeys(t, m, query)
	if m.sessionList.FilterState() != list.Filtering {
		t.Fatalf("precondition: filter state = %v, want Filtering (input-active)", m.sessionList.FilterState())
	}
	if got := len(m.sessionList.VisibleItems()); got != 0 {
		t.Fatalf("precondition: %d visible items for query %q, want 0 (no matches)", got, query)
	}
	return m
}

func TestNoMatches_RendersGlyphMessageHint(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := enterNoMatches(t, th, "zzqx")
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			noMatchesGlyph,
			`No sessions match "zzqx"`,
			"to widen the search",
			"to clear the filter",
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] no-matches body missing %q:\n%s", themeLabel(th), want, vis)
			}
		}
		if strings.Contains(vis, "backspace to widen") {
			t.Errorf("[%v] widen hint must use the ⌫ glyph, not the word 'backspace':\n%s", themeLabel(th), vis)
		}
	}
}

func TestNoMatches_InterpolatesQueryVerbatimWithLiteralQuotes(t *testing.T) {
	m := enterNoMatches(t, testDarkTheme(t), "qz - x")
	vis := ansi.Strip(m.View().Content)

	want := `No sessions match "qz - x"`
	if !strings.Contains(vis, want) {
		t.Errorf("no-matches message did not interpolate the query verbatim with literal quotes; want %q:\n%s", want, vis)
	}

	embed := formatNoMatchesMessage(`say "hi"`)
	if w := `No sessions match "say "hi""`; embed != w {
		t.Errorf("embedded-quote query not interpolated verbatim; got %q, want %q (literal quotes, not %%q)", embed, w)
	}
	if strings.Contains(embed, `\"`) {
		t.Errorf("embedded-quote message appears to use %%q escaping (found \\\"); want literal straight quotes: %q", embed)
	}
}

func TestNoMatches_FooterStaysInputActiveForm(t *testing.T) {
	m := enterNoMatches(t, testDarkTheme(t), "zzqx")
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"type to filter", "esc clear"} {
		if !strings.Contains(vis, want) {
			t.Errorf("no-matches footer missing input-active entry %q:\n%s", want, vis)
		}
	}
	if strings.Contains(vis, "browse results") {
		t.Errorf("no-matches footer must DROP the browse-results entry (no results to browse):\n%s", vis)
	}
	if strings.Contains(vis, "navigate") || strings.Contains(vis, "clear filter") {
		t.Errorf("no-matches footer must NOT be the list-active footer:\n%s", vis)
	}
	if strings.Contains(vis, "switch view") {
		t.Errorf("no-matches footer must replace the standard footer (found 'switch view'):\n%s", vis)
	}
}

func TestNoMatches_DoesNotRenderWhenResultsExist(t *testing.T) {
	m := filteringTestModel(t, testDarkTheme(t))
	m = pressSlash(t, m)
	m = typeKeys(t, m, "fab")
	if got := len(m.sessionList.VisibleItems()); got == 0 {
		t.Fatalf("precondition: query %q matched zero sessions, want >0", "fab")
	}
	vis := ansi.Strip(m.View().Content)

	if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
		t.Errorf("no-matches state rendered while results exist:\n%s", vis)
	}
	if !strings.Contains(vis, "fab") {
		t.Errorf("expected matching session rows in the body:\n%s", vis)
	}
}

func TestNoMatches_NotRenderedWithoutActiveQuery(t *testing.T) {
	m := filteringTestModel(t, testDarkTheme(t))
	m.applySessions(nil, nil)
	if m.sessionList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: filter state = %v, want Unfiltered", m.sessionList.FilterState())
	}
	vis := ansi.Strip(m.View().Content)

	if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
		t.Errorf("no-matches state must NOT render without an active query (empty-sessions is a distinct state):\n%s", vis)
	}
}

func TestNoMatches_OnlyRendersWithActiveNonEmptyQueryAndZeroItems(t *testing.T) {
	m := enterNoMatches(t, testDarkTheme(t), "zzqx")
	if !m.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=true for active non-empty query with zero items")
	}

	withResults := filteringTestModel(t, testDarkTheme(t))
	withResults = pressSlash(t, withResults)
	withResults = typeKeys(t, withResults, "fab")
	if withResults.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=false when the query matches results")
	}

	empty := filteringTestModel(t, testDarkTheme(t))
	empty.applySessions(nil, nil)
	if empty.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=false without an active query (empty-sessions, not no-matches)")
	}
}

func TestNoMatchesFooterEntries_ExcludesBrowseResultsStructurally(t *testing.T) {
	got := noMatchesFooterEntries(testDarkTheme(t))

	for _, e := range got {
		if e.BrowseResults {
			t.Errorf("no-matches footer must exclude the browse-results entry (structurally tagged), found: %+v", e)
		}
	}

	src := filteringFooterEntries(testDarkTheme(t))
	want := make([]filterFooterEntry, 0, len(src))
	for _, e := range src {
		if e.BrowseResults {
			continue
		}
		want = append(want, e)
	}
	if len(got) != len(want) {
		t.Fatalf("no-matches footer entry count = %d, want %d (input-active minus browse-results)", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("no-matches footer entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestNoMatchesFooterEntries_DecoupledFromBrowseResultsCopy(t *testing.T) {
	src := filteringFooterEntries(testDarkTheme(t))

	tagged := 0
	for _, e := range src {
		if e.BrowseResults {
			tagged++
		}
	}
	if tagged != 1 {
		t.Fatalf("expected exactly one browse-results-tagged entry, got %d", tagged)
	}

	reworded := append([]filterFooterEntry(nil), src...)
	for i := range reworded {
		if reworded[i].BrowseResults {
			reworded[i].Label = "view the matches"
		}
	}
	dropped := dropBrowseResults(reworded)
	for _, e := range dropped {
		if e.BrowseResults {
			t.Errorf("reworded browse-results entry survived the structural filter: %+v", e)
		}
		if e.Label == "view the matches" {
			t.Errorf("reworded browse-results entry leaked into no-matches footer by label: %+v", e)
		}
	}
	if got, want := len(dropped), len(reworded)-1; got != want {
		t.Errorf("dropBrowseResults returned %d entries, want %d (one dropped)", got, want)
	}
}

func TestNoMatches_ColourRoles(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		body := renderNoMatchesBody("zzqx", filteringReskinWidth, 20, th, false)

		if seq := tokenFgSeq(t, th.TextFaint); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches glyph missing text.faint SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches message missing text.primary SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches hint missing text.muted SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
	}
}

func TestNoMatches_QueryWhittledToEmptyExitsState(t *testing.T) {
	m := enterNoMatches(t, testDarkTheme(t), "z")
	if !m.sessionListNoMatches() {
		t.Fatalf("precondition: expected no-matches state for query %q", "z")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	out := drainFilterCmd(updated, cmd).(Model)
	if out.sessionListNoMatches() {
		t.Errorf("query whittled to empty must exit the no-matches state; filter value = %q", out.sessionList.FilterValue())
	}
}
