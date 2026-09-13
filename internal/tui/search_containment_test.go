package tui_test

import (
	"path/filepath"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

func containmentVisibleNames(m tui.Model) []string {
	names := make([]string, 0, len(m.SessionListVisibleItems()))
	for _, it := range m.SessionListVisibleItems() {
		if si, ok := it.(tui.SessionItem); ok {
			names = append(names, si.Session.Name)
		}
	}
	return names
}

func containmentDrain(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	updated, next := m.Update(msg)
	return containmentDrain(t, updated, next)
}

func containmentIngest(t *testing.T, m tui.Model, sessions []tmux.Session, projects []project.Project) tui.Model {
	t.Helper()
	var model tea.Model = m
	model, cmd := model.Update(tui.SessionsMsg{Sessions: sessions})
	model = containmentDrain(t, model, cmd)
	model, cmd = model.Update(tui.ProjectsLoadedMsg{Projects: projects})
	model = containmentDrain(t, model, cmd)
	return model.(tui.Model)
}

func containmentPicker(t *testing.T, deps tui.Deps, sessions []tmux.Session, projects []project.Project) tui.Model {
	t.Helper()
	if deps.Lister == nil {
		deps.Lister = &mockSessionLister{sessions: sessions}
	}
	if deps.ProjectStore == nil {
		deps.ProjectStore = &mockProjectStore{projects: projects}
	}
	return containmentIngest(t, tui.Build(deps), sessions, projects)
}

func assertVisible(t *testing.T, m tui.Model, want []string, context string) {
	t.Helper()
	got := containmentVisibleNames(m)
	if len(got) != len(want) {
		t.Fatalf("%s: visible sessions = %v, want %v", context, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: visible sessions = %v, want %v", context, got, want)
		}
	}
}

// scatteredSessions holds a session the fuzzy subsequence rule returns for the
// term "port" (p·o·r·t across "Projects/rust-tools") but containment does not.
func scatteredSessions(t *testing.T) []tmux.Session {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return []tmux.Session{
		{Name: "portal-a1b2", Windows: 1},
		{Name: "rust-tools", Windows: 1, Dir: filepath.Join(home, "Projects", "rust-tools")},
		{Name: "other", Windows: 1},
	}
}

func TestSearchContainmentNarrowsTheList(t *testing.T) {
	t.Run("it narrows a search-opened list to the containment set", func(t *testing.T) {
		sessions := scatteredSessions(t)

		m := containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
		}, sessions, nil)

		assertVisible(t, m, []string{"portal-a1b2"}, "search-opened picker")
	})

	t.Run("it returns that same session under -f, where the fuzzy rule applies", func(t *testing.T) {
		sessions := scatteredSessions(t)

		fuzzy := containmentPicker(t, tui.Deps{
			InitialMode:   prefs.ModeFlat,
			InitialFilter: "port",
		}, sessions, nil)

		assertVisible(t, fuzzy, []string{"portal-a1b2", "rust-tools"}, "-f picker")
	})
}

func TestSearchContainmentPreservesListOrder(t *testing.T) {
	// Fuzzy ranks "portal-a1b2" above "app-port"; the list order is the reverse.
	sessions := []tmux.Session{
		{Name: "app-port", Windows: 1},
		{Name: "portal-a1b2", Windows: 1},
	}

	t.Run("it keeps the list's own order and re-ranks nothing", func(t *testing.T) {
		m := containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
		}, sessions, nil)

		assertVisible(t, m, []string{"app-port", "portal-a1b2"}, "search-opened picker")
	})

	t.Run("it leaves a -f picker on the fuzzy rule", func(t *testing.T) {
		m := containmentPicker(t, tui.Deps{
			InitialMode:   prefs.ModeFlat,
			InitialFilter: "port",
		}, sessions, nil)

		assertVisible(t, m, []string{"portal-a1b2", "app-port"}, "-f picker")
	})

	t.Run("it leaves a picker opened with no filter on the fuzzy rule", func(t *testing.T) {
		m := containmentPicker(t, tui.Deps{InitialMode: prefs.ModeFlat}, sessions, nil)

		m.SetSessionListFilter("port")

		assertVisible(t, m, []string{"portal-a1b2", "app-port"}, "unfiltered picker")
	})
}

func TestSearchContainmentMatchesTheRecordedDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessions := []tmux.Session{
		{Name: "api-work", Windows: 1, Dir: filepath.Join(home, "Code", "portal")},
		// A subsequence match for "portal" and nothing a run of it appears in.
		{Name: "p-o-r-t-a-l", Windows: 1},
		{Name: "other", Windows: 1},
	}

	m := containmentPicker(t, tui.Deps{
		InitialMode: prefs.ModeFlat,
		Search:      &tui.SearchForm{Term: "portal"},
	}, sessions, nil)

	assertVisible(t, m, []string{"api-work"}, "directory-only match")
}

func TestSearchContainmentMatchesASpacedNameOnItsOwnFields(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "my session", Windows: 1, Dir: "/opt/api"},
	}

	m := containmentPicker(t, tui.Deps{
		InitialMode: prefs.ModeFlat,
		Search:      &tui.SearchForm{Term: "session /opt"},
	}, sessions, nil)

	assertVisible(t, m, nil, "term spanning the joined filter value")
}

type containmentEnumerator struct{ groups []tmux.WindowGroup }

func (e containmentEnumerator) ListWindowsAndPanesInSession(string) ([]tmux.WindowGroup, error) {
	return e.groups, nil
}

func containmentVisibleHeadings(m tui.Model) []string {
	var headings []string
	for _, it := range m.SessionListVisibleItems() {
		if h, ok := it.(tui.HeaderItem); ok {
			headings = append(headings, h.Heading)
		}
	}
	return headings
}

func assertNoHeadings(t *testing.T, m tui.Model, context string) {
	t.Helper()
	if got := containmentVisibleHeadings(m); len(got) != 0 {
		t.Errorf("%s: visible group headings = %v, want none", context, got)
	}
}

// groupedFixture is three sessions across two tagged projects, of which the
// term "port" contains-matches the first two.
func groupedFixture(t *testing.T) ([]tmux.Session, []project.Project) {
	t.Helper()
	dirA, dirB := t.TempDir(), t.TempDir()
	projects := []project.Project{
		{Path: dirA, Name: "Alpha", Tags: []string{"work"}},
		{Path: dirB, Name: "Bravo", Tags: []string{"play"}},
	}
	sessions := []tmux.Session{
		{Name: "app-port", Windows: 1, Dir: dirA},
		{Name: "portal-a1b2", Windows: 1, Dir: dirB},
		{Name: "other", Windows: 1, Dir: dirA},
	}
	return sessions, projects
}

func TestSearchContainmentAcrossGroupingModes(t *testing.T) {
	// The survivors keep each mode's own row order, so By Tag ("play" before
	// "work") lists them the other way round.
	modes := []struct {
		label string
		mode  prefs.SessionListMode
		want  []string
	}{
		{"Flat", prefs.ModeFlat, []string{"app-port", "portal-a1b2"}},
		{"By Project", prefs.ModeByProject, []string{"app-port", "portal-a1b2"}},
		{"By Tag", prefs.ModeByTag, []string{"portal-a1b2", "app-port"}},
	}

	for _, mode := range modes {
		t.Run("it drops group headers under a non-empty term in "+mode.label, func(t *testing.T) {
			sessions, projects := groupedFixture(t)

			m := containmentPicker(t, tui.Deps{
				InitialMode: mode.mode,
				Search:      &tui.SearchForm{Term: "port"},
			}, sessions, projects)

			assertVisible(t, m, mode.want, mode.label)
			assertNoHeadings(t, m, mode.label)

			sorted := slices.Sorted(slices.Values(containmentVisibleNames(m)))
			if !slices.Equal(sorted, []string{"app-port", "portal-a1b2"}) {
				t.Errorf("%s: containment set = %v, want the same set as every other mode", mode.label, sorted)
			}
		})
	}
}

func TestSearchContainmentSurvivesReRenders(t *testing.T) {
	t.Run("it reproduces the containment set after an s regroup", func(t *testing.T) {
		sessions, projects := groupedFixture(t)

		m := containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
		}, sessions, projects)

		regroups := []struct {
			label string
			want  []string
		}{
			{"after one s (By Project)", []string{"app-port", "portal-a1b2"}},
			{"after two s (By Tag)", []string{"portal-a1b2", "app-port"}},
		}
		for _, regroup := range regroups {
			updated, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
			m = containmentDrain(t, updated, cmd).(tui.Model)
			assertVisible(t, m, regroup.want, regroup.label)
			assertNoHeadings(t, m, regroup.label)
		}
	})

	t.Run("it reproduces the containment set after a preview and back", func(t *testing.T) {
		sessions, projects := groupedFixture(t)

		m := containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
			Enumerator:  containmentEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}},
			Reader:      stubScrollbackReader{bytes: []byte("hi")},
		}, sessions, projects)

		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		previewed := containmentDrain(t, updated, cmd).(tui.Model)
		if previewed.ActivePage() == m.ActivePage() {
			t.Fatalf("test setup invariant: Space did not enter the preview page")
		}

		updated2, cmd2 := previewed.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		back := containmentDrain(t, updated2, cmd2).(tui.Model)

		assertVisible(t, back, []string{"app-port", "portal-a1b2"}, "after preview and back")
	})

	t.Run("it reproduces the containment set after a refresh that drops a killed session", func(t *testing.T) {
		sessions, projects := groupedFixture(t)
		// Survives the refresh as a subsequence match for "port" (p·o·r·t
		// scattered across the path) that no run of "port" appears in.
		sessions = append(sessions, tmux.Session{Name: "rust-tools", Windows: 1, Dir: "/opt/projects/rust-tools"})

		m := containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
		}, sessions, projects)
		assertVisible(t, m, []string{"app-port", "portal-a1b2"}, "before the external kill")

		updated, cmd := m.Update(tui.SessionsMsg{Sessions: sessions[1:]})
		refreshed := containmentDrain(t, updated, cmd).(tui.Model)

		assertVisible(t, refreshed, []string{"portal-a1b2"}, "after an external kill")
	})
}

func TestSearchContainmentHoldsOnTheFilterText(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "app-port", Windows: 1},
		{Name: "portal-a1b2", Windows: 1},
		{Name: "pro-tools", Windows: 1},
	}
	searched := func(t *testing.T) tui.Model {
		t.Helper()
		return containmentPicker(t, tui.Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &tui.SearchForm{Term: "port"},
		}, sessions, nil)
	}

	t.Run("it returns to the fuzzy rule when the filter text is edited", func(t *testing.T) {
		m := searched(t)

		m.SetSessionListFilter("prt")

		assertVisible(t, m, []string{"pro-tools", "portal-a1b2", "app-port"}, "edited filter text")
	})

	t.Run("it restores containment when the text is edited back to the supplied term", func(t *testing.T) {
		m := searched(t)

		m.SetSessionListFilter("prt")
		m.SetSessionListFilter("port")

		assertVisible(t, m, []string{"app-port", "portal-a1b2"}, "term restored")
	})

	t.Run("it shows every session when the filter is cleared", func(t *testing.T) {
		m := searched(t)

		m.SetSessionListFilter("")

		assertVisible(t, m, []string{"app-port", "portal-a1b2", "pro-tools"}, "cleared filter")
	})
}

func TestSearchContainmentIsNotInstalledForATermLessForm(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "app-port", Windows: 1},
		{Name: "pro-tools", Windows: 1},
	}

	m := containmentPicker(t, tui.Deps{
		InitialMode: prefs.ModeFlat,
		Search:      &tui.SearchForm{Term: ""},
	}, sessions, nil)

	var model tea.Model = m
	for _, r := range "prt" {
		updated, cmd := model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		model = containmentDrain(t, updated, cmd)
	}

	// "pro-tools" is a fuzzy-only survivor: no run of "prt" appears in it.
	assertVisible(t, model.(tui.Model), []string{"pro-tools", "app-port"}, "term-less form")
}
