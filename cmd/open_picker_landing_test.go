package cmd

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// landPicker drives a freshly built picker to its landing, the point an initial
// filter and a search form have been applied to the list.
func landPicker(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{
		Sessions: []tmux.Session{{Name: "portal-a1b2"}, {Name: "blog-c3d4"}},
	})
	model, _ = model.Update(tui.ProjectsLoadedMsg{})

	m, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	return m
}

func TestBuildTUIModel_PickerLanding(t *testing.T) {
	t.Run("it hands the picker a search form carrying the term", func(t *testing.T) {
		m := buildTUIModel(defaultTestTUIConfig(), pickerLanding{search: &tui.SearchForm{Term: "port"}}, nil)

		if got := m.InitialFilter(); got != "" {
			t.Errorf("InitialFilter() = %q, want empty: a search term is not -f's text", got)
		}

		landed := landPicker(t, m)

		if got := landed.SessionListFilterValue(); got != "port" {
			t.Errorf("SessionListFilterValue() = %q, want %q", got, "port")
		}
		if got := landed.SessionListFilterState(); got != list.FilterApplied {
			t.Errorf("SessionListFilterState() = %v, want FilterApplied", got)
		}
	})

	t.Run("it hands the picker a term-less search form for a bare sigil", func(t *testing.T) {
		m := landPicker(t, buildTUIModel(defaultTestTUIConfig(), pickerLanding{search: &tui.SearchForm{}}, nil))

		if got := m.SessionListFilterState(); got != list.Filtering {
			t.Errorf("SessionListFilterState() = %v, want Filtering: a term-less form opens the input focused", got)
		}
		if got := m.SessionListFilterValue(); got != "" {
			t.Errorf("SessionListFilterValue() = %q, want empty", got)
		}
	})

	t.Run("it hands the picker the deferred decision closure on the cold route", func(t *testing.T) {
		landing := pickerLanding{search: &tui.SearchForm{
			Term:   "port",
			Decide: func() (string, error) { return "portal-a1b2", nil },
		}}

		cfg := defaultTestTUIConfig()
		cfg.serverStarted = true
		cfg.progressReceiver = func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} }

		var model tea.Model = buildTUIModel(cfg, landing, nil)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})

		m, ok := model.(tui.Model)
		if !ok {
			t.Fatalf("model type = %T, want tui.Model", model)
		}
		if !m.SearchAttached() {
			t.Fatal("SearchAttached() = false: the decision closure never reached the picker")
		}
		if got := m.Selected(); got != "portal-a1b2" {
			t.Errorf("Selected() = %q, want %q", got, "portal-a1b2")
		}
	})

	t.Run("it hands the picker -f's text as the initial filter with no search form", func(t *testing.T) {
		m := buildTUIModel(defaultTestTUIConfig(), pickerLanding{filter: "blog"}, nil)

		if got := m.InitialFilter(); got != "blog" {
			t.Errorf("InitialFilter() = %q, want %q", got, "blog")
		}

		landed := landPicker(t, m)

		if got := landed.SessionListFilterValue(); got != "blog" {
			t.Errorf("SessionListFilterValue() = %q, want %q", got, "blog")
		}
		if got := landed.SessionListFilterState(); got != list.FilterApplied {
			t.Errorf("SessionListFilterState() = %v, want FilterApplied", got)
		}
	})

	t.Run("it hands the picker neither for a bare open", func(t *testing.T) {
		m := buildTUIModel(defaultTestTUIConfig(), pickerLanding{}, nil)

		if got := m.InitialFilter(); got != "" {
			t.Errorf("InitialFilter() = %q, want empty", got)
		}

		m = landPicker(t, m)

		if got := m.SessionListFilterState(); got != list.Unfiltered {
			t.Errorf("SessionListFilterState() = %v, want Unfiltered", got)
		}
		if got := m.SessionListFilterValue(); got != "" {
			t.Errorf("SessionListFilterValue() = %q, want empty", got)
		}
	})
}
