package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// This file is the §7.3 over-filtered no-matches gate (task 2-9b). It pins the MV
// treatment of the Sessions body when an active filter query matches zero
// sessions: a centred dim null-set glyph (text.faint) over
// `No sessions match "<query>"` (text.primary) over the
// `⌫ to widen the search · esc to clear the filter` hint (text.detail), on the
// owned canvas, while the footer stays in the input-active form WITHOUT the
// browse-results entry (there are no results to browse — §7.3 / reference).
//
// The state renders ONLY when an active non-empty query matches zero — never when
// results exist, and it is DISTINCT from the empty-sessions state (§11.1, Phase 4:
// no sessions exist at all, no active query). Colour roles are pinned in exact
// mode-resolved SGR so a token swap is caught, not merely the glyph's presence. No
// t.Parallel() (the package-level mock convention + shared canvas helpers make
// parallelism unsafe across this package's tests).

// enterNoMatches drives the model into the over-filtered no-matches state: the
// fab* fixture loaded, `/` pressed, then a query typed that matches nothing. The
// list is in Filtering (input-active) with zero visible items.
func enterNoMatches(t *testing.T, mode theme.Mode, query string) Model {
	t.Helper()
	m := filteringTestModel(t, mode)
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

// TestNoMatches_RendersGlyphMessageHint asserts §7.3: with an active query
// matching zero sessions the body shows the centred null-set glyph, the
// `No sessions match "<query>"` message, and the `⌫ to widen the search …` hint.
func TestNoMatches_RendersGlyphMessageHint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := enterNoMatches(t, mode, "zzqx")
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			noMatchesGlyph,
			`No sessions match "zzqx"`,
			"to widen the search",
			"to clear the filter",
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] no-matches body missing %q:\n%s", mode, want, vis)
			}
		}
		// The widen glyph is the ⌫ backspace glyph (reference), not the literal word.
		if strings.Contains(vis, "backspace to widen") {
			t.Errorf("[%v] widen hint must use the ⌫ glyph, not the word 'backspace':\n%s", mode, vis)
		}
	}
}

// TestNoMatches_InterpolatesQueryVerbatimWithLiteralQuotes asserts §7.3: the
// message interpolates the current query verbatim with byte-exact literal straight
// double-quotes — mirroring formatSessionGoneFlash, NOT %q — so spaces / dashes /
// unicode / embedded quotes render verbatim.
func TestNoMatches_InterpolatesQueryVerbatimWithLiteralQuotes(t *testing.T) {
	// A query with a space and a dash to prove the wrapping is exactly two straight
	// quotes around the verbatim query. None of the fab* fixture names contains
	// "qz - " so it matches zero.
	m := enterNoMatches(t, theme.Dark, "qz - x")
	vis := ansi.Strip(m.View().Content)

	want := `No sessions match "qz - x"`
	if !strings.Contains(vis, want) {
		t.Errorf("no-matches message did not interpolate the query verbatim with literal quotes; want %q:\n%s", want, vis)
	}

	// Discriminating case: a query with an EMBEDDED double-quote. The literal-quote
	// pattern emits the quote verbatim (`say "hi"`); %q would backslash-escape it
	// (`say \"hi\"`). The `qz - x` case above is byte-identical under %q and the
	// literal-quote pattern, so only an embedded quote actually distinguishes them
	// (the task's embedded-quote edge case). Asserting the verbatim form present + the
	// backslash-escaped form absent pins the literal-quote pattern, not %q.
	embed := formatNoMatchesMessage(`say "hi"`)
	if w := `No sessions match "say "hi""`; embed != w {
		t.Errorf("embedded-quote query not interpolated verbatim; got %q, want %q (literal quotes, not %%q)", embed, w)
	}
	if strings.Contains(embed, `\"`) {
		t.Errorf("embedded-quote message appears to use %%q escaping (found \\\"); want literal straight quotes: %q", embed)
	}
}

// TestNoMatches_FooterStaysInputActiveForm asserts §7.3: the footer stays in the
// input-active form, reduced for this state — `type to filter · esc clear`,
// WITHOUT the `browse results` entry (no results to browse) and NOT the list-active
// footer.
func TestNoMatches_FooterStaysInputActiveForm(t *testing.T) {
	m := enterNoMatches(t, theme.Dark, "zzqx")
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"type to filter", "esc clear"} {
		if !strings.Contains(vis, want) {
			t.Errorf("no-matches footer missing input-active entry %q:\n%s", want, vis)
		}
	}
	// No browse-results entry (no results to browse) and not the list-active footer.
	if strings.Contains(vis, "browse results") {
		t.Errorf("no-matches footer must DROP the browse-results entry (no results to browse):\n%s", vis)
	}
	if strings.Contains(vis, "navigate") || strings.Contains(vis, "clear filter") {
		t.Errorf("no-matches footer must NOT be the list-active footer:\n%s", vis)
	}
	// The standard condensed footer must not appear while filtering.
	if strings.Contains(vis, "switch view") {
		t.Errorf("no-matches footer must replace the standard footer (found 'switch view'):\n%s", vis)
	}
}

// TestNoMatches_DoesNotRenderWhenResultsExist asserts §7.3: the no-matches state
// renders ONLY when the query matches zero — typing a query that DOES match shows
// the normal list body, not the empty state.
func TestNoMatches_DoesNotRenderWhenResultsExist(t *testing.T) {
	m := filteringTestModel(t, theme.Dark)
	m = pressSlash(t, m)
	m = typeKeys(t, m, "fab")
	if got := len(m.sessionList.VisibleItems()); got == 0 {
		t.Fatalf("precondition: query %q matched zero sessions, want >0", "fab")
	}
	vis := ansi.Strip(m.View().Content)

	if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
		t.Errorf("no-matches state rendered while results exist:\n%s", vis)
	}
	// The matching session rows are present.
	if !strings.Contains(vis, "fab") {
		t.Errorf("expected matching session rows in the body:\n%s", vis)
	}
}

// TestNoMatches_NotRenderedWithoutActiveQuery asserts §7.3 / §11.1: the no-matches
// state is DISTINCT from the empty-sessions state — with NO active filter (no
// query) the empty state never renders, even with zero sessions.
func TestNoMatches_NotRenderedWithoutActiveQuery(t *testing.T) {
	// Zero sessions, NO filter active (Unfiltered) — the §11.1 empty-sessions
	// condition, NOT the §7.3 no-matches condition.
	m := filteringTestModel(t, theme.Dark)
	m.applySessions(nil)
	if m.sessionList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: filter state = %v, want Unfiltered", m.sessionList.FilterState())
	}
	vis := ansi.Strip(m.View().Content)

	if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
		t.Errorf("no-matches state must NOT render without an active query (empty-sessions is a distinct state):\n%s", vis)
	}
}

// TestNoMatches_OnlyRendersWithActiveNonEmptyQueryAndZeroItems asserts the precise
// detection predicate: an active non-empty query AND zero visible items.
func TestNoMatches_OnlyRendersWithActiveNonEmptyQueryAndZeroItems(t *testing.T) {
	// Active non-empty query, zero matches -> renders.
	m := enterNoMatches(t, theme.Dark, "zzqx")
	if !m.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=true for active non-empty query with zero items")
	}

	// Active non-empty query WITH matches -> does not render.
	withResults := filteringTestModel(t, theme.Dark)
	withResults = pressSlash(t, withResults)
	withResults = typeKeys(t, withResults, "fab")
	if withResults.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=false when the query matches results")
	}

	// No active query, zero sessions -> does not render (empty-sessions state).
	empty := filteringTestModel(t, theme.Dark)
	empty.applySessions(nil)
	if empty.sessionListNoMatches() {
		t.Errorf("expected sessionListNoMatches()=false without an active query (empty-sessions, not no-matches)")
	}
}

// TestNoMatches_ColourRoles asserts §2.9: the glyph reads text.faint, the message
// text.primary, and the hint text.detail — pinned in exact mode-resolved SGR.
func TestNoMatches_ColourRoles(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		body := renderNoMatchesBody("zzqx", filteringReskinWidth, 20, mode, false)

		if seq := tokenFgSeq(t, theme.MV.TextFaint, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches glyph missing text.faint SGR %q:\n%s", mode, seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches message missing text.primary SGR %q:\n%s", mode, seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] no-matches hint missing text.detail SGR %q:\n%s", mode, seq, escSeq(body))
		}
	}
}

// TestNoMatches_QueryWhittledToEmptyExitsState asserts §7.3 parity: a query
// whittled down to empty exits the no-matches state and returns to the normal view
// (the filter engine's backspace behaviour is unchanged — the hint just documents
// it).
func TestNoMatches_QueryWhittledToEmptyExitsState(t *testing.T) {
	m := enterNoMatches(t, theme.Dark, "z")
	if !m.sessionListNoMatches() {
		t.Fatalf("precondition: expected no-matches state for query %q", "z")
	}
	// Backspace deletes the single query char -> empty query -> exits the state.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	out := drainFilterCmd(updated, cmd).(Model)
	if out.sessionListNoMatches() {
		t.Errorf("query whittled to empty must exit the no-matches state; filter value = %q", out.sessionList.FilterValue())
	}
}
