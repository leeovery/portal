package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

const filteringReskinWidth = 120
const filteringReskinHeight = 40

func filteringTestModel(t *testing.T, th theme.Theme) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 2, Attached: true},
		{Name: "fab-aws-migration", Windows: 1, Attached: false},
		{Name: "fabric-lk26UG", Windows: 1, Attached: false},
		{Name: "other-session", Windows: 1, Attached: false},
	}
	m := Build(Deps{Lister: fakeLister{}, Theme: theme.ConstantNomination(th)})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(sessions, nil)
	return m
}

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

func pressSlash(t *testing.T, m Model) Model {
	t.Helper()
	var model tea.Model = m
	model, _ = model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	return model.(Model)
}

func enterInputActive(t *testing.T, th theme.Theme) Model {
	t.Helper()
	m := filteringTestModel(t, th)
	m = pressSlash(t, m)
	m = typeKeys(t, m, "fab")
	if m.sessionList.FilterState() != list.Filtering {
		t.Fatalf("precondition: filter state = %v, want Filtering (input-active)", m.sessionList.FilterState())
	}
	return m
}

func enterListActive(t *testing.T, th theme.Theme) Model {
	t.Helper()
	m := enterInputActive(t, th)
	var model tea.Model = m
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := model.(Model)
	if out.sessionList.FilterState() != list.FilterApplied {
		t.Fatalf("precondition: filter state = %v, want FilterApplied (list-active)", out.sessionList.FilterState())
	}
	return out
}

func TestFiltering_InputActiveQueryOrange(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := enterInputActive(t, th)
		view := m.View().Content

		orange := tokenFgSeq(t, th.AccentAttention)
		if !strings.Contains(view, orange) {
			t.Errorf("[%v] input-active view missing accent.attention filter SGR %q:\n%s", themeLabel(th), orange, ansi.Strip(view))
		}
		vis := ansi.Strip(view)
		if !strings.Contains(vis, "/") {
			t.Errorf("[%v] input-active view missing the `/` prefix:\n%s", themeLabel(th), vis)
		}
		if !strings.Contains(vis, "fab") {
			t.Errorf("[%v] input-active view missing the live query %q:\n%s", themeLabel(th), "fab", vis)
		}
	}
}

func TestFiltering_InputActiveNoRowSelected(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := enterInputActive(t, th)
		view := m.View().Content

		selBg := selectionBgParams(t, th)
		if strings.Contains(view, selBg) {
			t.Errorf("[%v] input-active frame must NOT paint a selected row (bg.selection %q present):\n%s", themeLabel(th), selBg, escSeq(view))
		}
		for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
			if strings.Contains(line, "fab") && strings.Contains(line, selectorBar) {
				t.Errorf("[%v] input-active session row must NOT render the selector bar %q while typing:\n%s", themeLabel(th), selectorBar, line)
			}
		}
	}
}

func TestFiltering_InputActiveFooter(t *testing.T) {
	m := enterInputActive(t, testDarkTheme(t))
	view := ansi.Strip(m.View().Content)

	for _, want := range []string{"type to filter", "browse results", "esc clear"} {
		if !strings.Contains(view, want) {
			t.Errorf("input-active footer missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "switch view") {
		t.Errorf("input-active footer must replace the standard footer (found 'switch view'):\n%s", view)
	}
}

func TestFiltering_InputActiveFooterColours(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderFilteringFooter(filteringReskinWidth, th, false)

		if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing accent.attention action-word SGR %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing accent.key nav-glyph SGR %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(footer, seq) {
			t.Errorf("[%v] input-active footer missing text.muted label SGR %q", themeLabel(th), seq)
		}
	}
}

func TestFiltering_ListActiveLockedQueryOrange(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := enterListActive(t, th)
		header := renderFilterQueryHeader(m.sessionList.FilterValue(), filteringReskinWidth, th, false)

		orange := tokenFgSeq(t, th.AccentAttention)
		if !strings.Contains(header, orange) {
			t.Errorf("[%v] list-active locked query missing accent.attention SGR %q:\n%s", themeLabel(th), orange, ansi.Strip(header))
		}
		vis := ansi.Strip(header)
		if !strings.Contains(vis, "/ fab") {
			t.Errorf("[%v] list-active locked query = %q, want it to contain %q", themeLabel(th), vis, "/ fab")
		}
	}
}

func TestFiltering_ListActiveSelectedRowNoInputTint(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := enterListActive(t, th)
		view := m.View().Content
		selBg := selectionBgParams(t, th)

		if !strings.Contains(view, selBg) {
			t.Errorf("[%v] list-active frame must paint a selected row (bg.selection %q absent):\n%s", themeLabel(th), selBg, escSeq(view))
		}
		header := renderFilterQueryHeader(m.sessionList.FilterValue(), filteringReskinWidth, th, false)
		if strings.Contains(header, selBg) {
			t.Errorf("[%v] list-active filter input must have NO bg tint (bg.selection %q present):\n%s", themeLabel(th), selBg, escSeq(header))
		}
	}
}

func TestFiltering_ListActiveFooter(t *testing.T) {
	m := enterListActive(t, testDarkTheme(t))
	view := ansi.Strip(m.View().Content)

	for _, want := range []string{"attach", "navigate", "esc clear filter"} {
		if !strings.Contains(view, want) {
			t.Errorf("list-active footer missing %q:\n%s", want, view)
		}
	}
}

func TestFiltering_ListActiveFooterClearIsOrange(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderFilterAppliedFooter(filteringReskinWidth, th, false)
		if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(footer, seq) {
			t.Errorf("[%v] list-active footer `esc` clear-filter key missing accent.attention SGR %q", themeLabel(th), seq)
		}
	}
}

func TestFiltering_EnterOrDownCommitsInputToList(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := enterInputActive(t, testDarkTheme(t))
			var model tea.Model = m
			model, _ = model.Update(tc.key)
			out := model.(Model)
			if out.sessionList.FilterState() != list.FilterApplied {
				t.Errorf("after %s: filter state = %v, want FilterApplied", tc.name, out.sessionList.FilterState())
			}
		})
	}
}

func TestFiltering_EscClearsFromEitherMode(t *testing.T) {
	t.Run("from input-active", func(t *testing.T) {
		m := enterInputActive(t, testDarkTheme(t))
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		out := model.(Model)
		if out.sessionList.FilterState() != list.Unfiltered {
			t.Errorf("after Esc from input-active: filter state = %v, want Unfiltered", out.sessionList.FilterState())
		}
	})
	t.Run("from list-active", func(t *testing.T) {
		m := enterListActive(t, testDarkTheme(t))
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		out := model.(Model)
		if out.sessionList.FilterState() != list.Unfiltered {
			t.Errorf("after Esc from list-active: filter state = %v, want Unfiltered", out.sessionList.FilterState())
		}
	})
}

func TestFiltering_TypingFlattensGroupedView(t *testing.T) {
	m := filteringTestModel(t, testDarkTheme(t))
	m.sessionListMode = prefs.ModeByProject
	(&m).rebuildSessionList()

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

	for _, it := range m.sessionList.VisibleItems() {
		if _, ok := it.(HeaderItem); ok {
			t.Errorf("grouped heading did not vanish on filter: %+v", it)
		}
	}
}

func TestFiltering_SLiteralWhileInputActive(t *testing.T) {
	m := filteringTestModel(t, testDarkTheme(t))
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

func TestFiltering_NoMatchCountShown(t *testing.T) {
	t.Run("input-active", func(t *testing.T) {
		m := enterInputActive(t, testDarkTheme(t))
		vis := ansi.Strip(m.View().Content)
		if strings.Contains(vis, "filtered") || strings.Contains(vis, "matched") {
			t.Errorf("input-active frame shows a match-count:\n%s", vis)
		}
	})
	t.Run("list-active", func(t *testing.T) {
		m := enterListActive(t, testDarkTheme(t))
		vis := ansi.Strip(m.View().Content)
		if strings.Contains(vis, "filtered") || strings.Contains(vis, "matched") {
			t.Errorf("list-active frame shows a match-count:\n%s", vis)
		}
	})
}
