package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// This file is the §7 / §7.1 / §7.2 filtering-reskin gate (task 2-8). It pins the
// MV treatment of the bubbles/list filter input: the `/` prompt + the live query
// in accent.orange, the two mutually-exclusive modes (input-active = typing,
// cursor at end, NO selected row; list-active = locked cursor-less query, selected
// row, no input bg tint), the two contextual footers driven by the filter mode,
// and the §5.1 flatten-on-filter behaviour (grouped headings vanish the instant a
// query is typed via HeaderItem.FilterValue()==""). The filter ENGINE is unchanged
// (parity) — these tests assert only the styling + the mode-clarity rendering +
// the engine's commit/clear transitions hold.
//
// Colour roles are pinned with exact mode-resolved SGR (the §2.9 token core), like
// the row / footer / header / grouped-reskin tests — a token swap is caught, not
// merely the glyph's presence. No t.Parallel() (the package-level mock convention
// and the shared canvas helpers make parallelism unsafe across this package's
// tests).

const filteringReskinWidth = 120
const filteringReskinHeight = 40

// filteringTestModel builds a production-shaped Sessions model pinned to the given
// mode, sized for rendering, loaded with the deterministic fab* session set
// through the production applySessions path. The fab* names mirror the committed
// Paper references so the rendered query matches "fab" filtering.
func filteringTestModel(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 2, Attached: true},
		{Name: "fab-aws-migration", Windows: 1, Attached: false},
		{Name: "fabric-lk26UG", Windows: 1, Attached: false},
		{Name: "other-session", Windows: 1, Attached: false},
	}
	appearance := prefs.AppearanceDark
	if mode == theme.Light {
		appearance = prefs.AppearanceLight
	}
	m := Build(Deps{Lister: fakeLister{}, Appearance: appearance})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(sessions)
	return m
}

// drainFilterCmd runs cmd (if non-nil) and, when it produces a list
// FilterMatchesMsg, feeds it back into the model so the asynchronous filter result
// is applied (bubbles/list's filterItems is a tea.Cmd, so VisibleItems only
// updates once its message is processed). Non-filter messages are ignored — the
// tests only need the filtered visible set to settle.
func drainFilterCmd(model tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return model
	}
	msg := cmd()
	if _, ok := msg.(list.FilterMatchesMsg); ok {
		model, _ = model.Update(msg)
	}
	return model
}

// typeKeys feeds each rune of s into the model as an individual KeyPressMsg,
// draining the async filter cmd after each keystroke so the filtered visible set
// settles. It mirrors how vhs drives the live filter input one keystroke at a time.
func typeKeys(t *testing.T, m Model, s string) Model {
	t.Helper()
	var model tea.Model = m
	for _, r := range s {
		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		model = drainFilterCmd(model, cmd)
	}
	return model.(Model)
}

// pressSlash activates the filter input (the `/` key) and returns the model in the
// Filtering (input-active) state.
func pressSlash(t *testing.T, m Model) Model {
	t.Helper()
	var model tea.Model = m
	model, _ = model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	return model.(Model)
}

// enterInputActive drives the model into the input-active state with the query
// "fab" typed (cursor at end). The list is in Filtering.
func enterInputActive(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	m := filteringTestModel(t, mode)
	m = pressSlash(t, m)
	m = typeKeys(t, m, "fab")
	if m.sessionList.FilterState() != list.Filtering {
		t.Fatalf("precondition: filter state = %v, want Filtering (input-active)", m.sessionList.FilterState())
	}
	return m
}

// enterListActive drives the model into the list-active state: query "fab" typed,
// then Enter to commit Filtering→FilterApplied (locked query, selectable rows).
func enterListActive(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	m := enterInputActive(t, mode)
	var model tea.Model = m
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := model.(Model)
	if out.sessionList.FilterState() != list.FilterApplied {
		t.Fatalf("precondition: filter state = %v, want FilterApplied (list-active)", out.sessionList.FilterState())
	}
	return out
}

// TestFiltering_InputActiveQueryOrange asserts §7: while typing (input-active) the
// `/` prompt and the query text render in accent.orange. Pinned in exact
// mode-resolved SGR so a token swap is caught.
func TestFiltering_InputActiveQueryOrange(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := enterInputActive(t, mode)
		view := m.View().Content

		orange := tokenFgSeq(t, theme.MV.AccentOrange, mode)
		if !strings.Contains(view, orange) {
			t.Errorf("[%v] input-active view missing accent.orange filter SGR %q:\n%s", mode, orange, ansi.Strip(view))
		}
		// The visible query + `/` prefix are present in the rendered frame.
		vis := ansi.Strip(view)
		if !strings.Contains(vis, "/") {
			t.Errorf("[%v] input-active view missing the `/` prefix:\n%s", mode, vis)
		}
		if !strings.Contains(vis, "fab") {
			t.Errorf("[%v] input-active view missing the live query %q:\n%s", mode, "fab", vis)
		}
	}
}

// TestFiltering_InputActiveNoRowSelected asserts §7.1: while typing (input-active)
// NO list row is selected — there is no bg.selection band painted on any row. The
// cursor sits at end-of-text in the filter input instead (the input owns the
// cursor; the list owns no selected row).
func TestFiltering_InputActiveNoRowSelected(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := enterInputActive(t, mode)
		view := m.View().Content

		// No bg.selection tint may appear anywhere in the input-active frame — the
		// definitive "no row selected" signal (the §3.3 selected-row band).
		selBg := selectionBgParams(t, mode)
		if strings.Contains(view, selBg) {
			t.Errorf("[%v] input-active frame must NOT paint a selected row (bg.selection %q present):\n%s", mode, selBg, escSeq(view))
		}
		// And no violet selector bar glyph on a SESSION ROW (the §3.3 selection
		// signal). The header wordmark also uses the ▌ caret, so scope the check to
		// lines that carry a fab* session name — no such line may lead with the bar.
		for _, line := range strings.Split(ansi.Strip(view), "\n") {
			if strings.Contains(line, "fab") && strings.Contains(line, selectorBar) {
				t.Errorf("[%v] input-active session row must NOT render the selector bar %q while typing:\n%s", mode, selectorBar, line)
			}
		}
	}
}

// TestFiltering_InputActiveFooter asserts §7.1: the input-active footer reads
// `type to filter · ↵/↓ browse results · esc clear`, replacing the standard
// condensed footer.
func TestFiltering_InputActiveFooter(t *testing.T) {
	m := enterInputActive(t, theme.Dark)
	view := ansi.Strip(m.View().Content)

	for _, want := range []string{"type to filter", "browse results", "esc clear"} {
		if !strings.Contains(view, want) {
			t.Errorf("input-active footer missing %q:\n%s", want, view)
		}
	}
	// The standard condensed footer entries must NOT be shown while filtering.
	if strings.Contains(view, "switch view") {
		t.Errorf("input-active footer must replace the standard footer (found 'switch view'):\n%s", view)
	}
}

// TestFiltering_InputActiveFooterColours asserts §7.1 per-word colours: the
// filter-specific action word reads in accent.orange, the nav glyphs in
// accent.blue, the labels in text.detail.
func TestFiltering_InputActiveFooterColours(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderFilteringFooter(filteringReskinWidth, mode, false)

		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing accent.orange action-word SGR %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing accent.blue nav-glyph SGR %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing text.detail label SGR %q", mode, seq)
		}
	}
}

// TestFiltering_ListActiveLockedQueryOrange asserts §7.1: after committing
// (list-active) the query renders as a locked accent.orange `/ query` with NO
// cursor — the cursor-less locked query signals the list is filtered.
func TestFiltering_ListActiveLockedQueryOrange(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := enterListActive(t, mode)
		header := renderFilterQueryHeader(m.sessionList.FilterValue(), filteringReskinWidth, mode, false)

		orange := tokenFgSeq(t, theme.MV.AccentOrange, mode)
		if !strings.Contains(header, orange) {
			t.Errorf("[%v] list-active locked query missing accent.orange SGR %q:\n%s", mode, orange, ansi.Strip(header))
		}
		vis := ansi.Strip(header)
		if !strings.Contains(vis, "/ fab") {
			t.Errorf("[%v] list-active locked query = %q, want it to contain %q", mode, vis, "/ fab")
		}
	}
}

// TestFiltering_ListActiveSelectedRowNoInputTint asserts §7.1: in list-active a row
// IS selected (the §3.3 violet bar + bg.selection band) but the filter input
// itself carries NO background tint.
func TestFiltering_ListActiveSelectedRowNoInputTint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := enterListActive(t, mode)
		view := m.View().Content
		selBg := selectionBgParams(t, mode)

		// A row IS selected — the bg.selection band must be present somewhere.
		if !strings.Contains(view, selBg) {
			t.Errorf("[%v] list-active frame must paint a selected row (bg.selection %q absent):\n%s", mode, selBg, escSeq(view))
		}
		// The locked query header itself carries no bg.selection tint.
		header := renderFilterQueryHeader(m.sessionList.FilterValue(), filteringReskinWidth, mode, false)
		if strings.Contains(header, selBg) {
			t.Errorf("[%v] list-active filter input must have NO bg tint (bg.selection %q present):\n%s", mode, selBg, escSeq(header))
		}
	}
}

// TestFiltering_ListActiveFooter asserts §7.1: the list-active footer reads
// `↵ attach · ↑↓ navigate · esc clear filter`.
func TestFiltering_ListActiveFooter(t *testing.T) {
	m := enterListActive(t, theme.Dark)
	view := ansi.Strip(m.View().Content)

	for _, want := range []string{"attach", "navigate", "esc clear filter"} {
		if !strings.Contains(view, want) {
			t.Errorf("list-active footer missing %q:\n%s", want, view)
		}
	}
}

// TestFiltering_ListActiveFooterClearIsOrange asserts §7.1: the `esc` clear-filter
// key reads accent.orange in the list-active footer.
func TestFiltering_ListActiveFooterClearIsOrange(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderFilterAppliedFooter(filteringReskinWidth, mode, false)
		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] list-active footer `esc` clear-filter key missing accent.orange SGR %q", mode, seq)
		}
	}
}

// TestFiltering_EnterOrDownCommitsInputToList asserts §7.2: enter OR down commits
// input-active → list-active (Filtering → FilterApplied). The engine owns this
// transition; the test verifies it holds for both keys.
func TestFiltering_EnterOrDownCommitsInputToList(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := enterInputActive(t, theme.Dark)
			var model tea.Model = m
			model, _ = model.Update(tc.key)
			out := model.(Model)
			if out.sessionList.FilterState() != list.FilterApplied {
				t.Errorf("after %s: filter state = %v, want FilterApplied", tc.name, out.sessionList.FilterState())
			}
		})
	}
}

// TestFiltering_EscClearsFromEitherMode asserts §7.2: Esc clears the filter from
// BOTH the input-active and list-active modes (parity with the engine).
func TestFiltering_EscClearsFromEitherMode(t *testing.T) {
	t.Run("from input-active", func(t *testing.T) {
		m := enterInputActive(t, theme.Dark)
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		out := model.(Model)
		if out.sessionList.FilterState() != list.Unfiltered {
			t.Errorf("after Esc from input-active: filter state = %v, want Unfiltered", out.sessionList.FilterState())
		}
	})
	t.Run("from list-active", func(t *testing.T) {
		m := enterListActive(t, theme.Dark)
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		out := model.(Model)
		if out.sessionList.FilterState() != list.Unfiltered {
			t.Errorf("after Esc from list-active: filter state = %v, want Unfiltered", out.sessionList.FilterState())
		}
	})
}

// TestFiltering_TypingFlattensGroupedView asserts §5.1: typing a query makes
// grouped headings vanish (flatten-on-filter via HeaderItem.FilterValue()=="").
// This is preserved, not changed — a query removes every HeaderItem from the
// visible set.
func TestFiltering_TypingFlattensGroupedView(t *testing.T) {
	m := filteringTestModel(t, theme.Dark)
	// Switch to a grouped mode so HeaderItems are present pre-filter.
	m.sessionListMode = prefs.ModeByProject
	(&m).rebuildSessionList()

	// Precondition: at least one HeaderItem is visible before filtering.
	preHeaders := 0
	for _, it := range m.sessionList.VisibleItems() {
		if _, ok := it.(HeaderItem); ok {
			preHeaders++
		}
	}
	if preHeaders == 0 {
		t.Skip("grouped fixture produced no header rows; flatten precondition not met")
	}

	m = pressSlash(t, m)
	m = typeKeys(t, m, "fab")

	// After typing, no HeaderItem may remain in the visible set (headings vanish).
	for _, it := range m.sessionList.VisibleItems() {
		if _, ok := it.(HeaderItem); ok {
			t.Errorf("grouped heading did not vanish on filter: %+v", it)
		}
	}
}

// TestFiltering_SLiteralWhileInputActive asserts §7: `s` is a literal filter
// character while input-active — it appends to the query rather than cycling the
// grouping mode. The dispatch case sits below the SettingFilter() guard (task 2-1).
func TestFiltering_SLiteralWhileInputActive(t *testing.T) {
	m := filteringTestModel(t, theme.Dark)
	startMode := m.sessionListMode
	m = pressSlash(t, m)
	m = typeKeys(t, m, "s")

	if m.sessionListMode != startMode {
		t.Errorf("`s` while filtering cycled the grouping mode (%v → %v); it must be a literal filter char", startMode, m.sessionListMode)
	}
	if !strings.Contains(m.sessionList.FilterValue(), "s") {
		t.Errorf("`s` while filtering did not append to the query; filter value = %q", m.sessionList.FilterValue())
	}
}

// TestFiltering_NoMatchCountShown asserts §7: no match-count is shown anywhere — in
// either filter mode. The bubbles/list status bar (which would render "N filtered")
// stays suppressed.
func TestFiltering_NoMatchCountShown(t *testing.T) {
	t.Run("input-active", func(t *testing.T) {
		m := enterInputActive(t, theme.Dark)
		vis := ansi.Strip(m.View().Content)
		if strings.Contains(vis, "filtered") || strings.Contains(vis, "matched") {
			t.Errorf("input-active frame shows a match-count:\n%s", vis)
		}
	})
	t.Run("list-active", func(t *testing.T) {
		m := enterListActive(t, theme.Dark)
		vis := ansi.Strip(m.View().Content)
		if strings.Contains(vis, "filtered") || strings.Contains(vis, "matched") {
			t.Errorf("list-active frame shows a match-count:\n%s", vis)
		}
	})
}
