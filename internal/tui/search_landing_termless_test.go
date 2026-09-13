package tui

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

func TestSearchFormLanding_TermlessForm(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "myapp-dev"},
		{Name: "other"},
		{Name: "myapp-prod"},
	}

	t.Run("it lands the term-less form on a focused empty sessions filter", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "", sessions, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterState(); got != list.Filtering {
			t.Errorf("session filter state = %v, want Filtering", got)
		}
		if got := m.sessionList.FilterValue(); got != "" {
			t.Errorf("session filter value = %q, want empty", got)
		}
	})

	t.Run("it keeps every live session visible under the empty focused filter", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "", sessions, nil)

		want := []string{"myapp-dev", "myapp-prod", "other"}
		got := sessionNames(m.sessionList.VisibleItems())
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("visible sessions = %v, want %v", got, want)
		}
	})

	for _, mode := range []struct {
		label string
		mode  prefs.SessionListMode
	}{
		{"By Project", prefs.ModeByProject},
		{"By Tag", prefs.ModeByTag},
	} {
		t.Run("it keeps every session visible in "+mode.label, func(t *testing.T) {
			grouped, projects := groupedSessions(t)
			m := searchLanding(t, mode.mode, "", grouped, projects)

			want := []string{"myapp-alpha", "myapp-bravo", "other"}
			got := sessionNames(m.sessionList.VisibleItems())
			slices.Sort(got)
			got = slices.Compact(got)
			if !slices.Equal(got, want) {
				t.Errorf("visible sessions = %v, want %v", got, want)
			}
		})

		t.Run("it puts the cursor on a session row rather than a group header in "+mode.label, func(t *testing.T) {
			grouped, projects := groupedSessions(t)
			m := searchLanding(t, mode.mode, "", grouped, projects)

			var headers int
			for _, it := range m.sessionList.VisibleItems() {
				if _, ok := it.(HeaderItem); ok {
					headers++
				}
			}
			if headers == 0 {
				t.Fatalf("precondition: the empty query left no group headers in %s", mode.label)
			}
			if _, isHeader := m.sessionList.SelectedItem().(HeaderItem); isHeader {
				t.Fatalf("selection rests on a group header, want a session row")
			}
			if _, ok := m.sessionList.SelectedItem().(SessionItem); !ok {
				t.Fatalf("selected item = %T, want SessionItem", m.sessionList.SelectedItem())
			}
		})
	}

	t.Run("it narrows on the first typed character", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "", sessions, nil)

		m = typeKeys(t, m, "o")

		if got := m.sessionList.FilterValue(); got != "o" {
			t.Errorf("session filter value = %q, want %q", got, "o")
		}
		want := []string{"myapp-prod", "other"}
		got := sessionNames(m.sessionList.VisibleItems())
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("visible sessions = %v, want %v", got, want)
		}
	})

	t.Run("it treats s as a filter character while the input is focused", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "", sessions, nil)

		m = typeKeys(t, m, "s")

		if got := m.sessionList.FilterValue(); got != "s" {
			t.Errorf("session filter value = %q, want %q", got, "s")
		}
		if got := m.sessionListMode; got != prefs.ModeFlat {
			t.Errorf("session list mode = %v, want it unswitched (%v)", got, prefs.ModeFlat)
		}
	})

	t.Run("it does not panic with zero live sessions", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "", nil, nil)
		m.termWidth, m.termHeight = 120, 40

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterState(); got != list.Filtering {
			t.Errorf("session filter state = %v, want Filtering", got)
		}
		if got := m.sessionList.FilterValue(); got != "" {
			t.Errorf("session filter value = %q, want empty", got)
		}
		if got := len(m.sessionList.VisibleItems()); got != 0 {
			t.Errorf("visible rows = %d, want none", got)
		}
		_ = m.View()
	})
}

func groupedSessions(t *testing.T) ([]tmux.Session, []project.Project) {
	t.Helper()
	dirA, dirB := t.TempDir(), t.TempDir()
	return []tmux.Session{
		{Name: "myapp-alpha", Dir: dirA},
		{Name: "other", Dir: dirB},
		{Name: "myapp-bravo", Dir: dirB},
	}, []project.Project{
		{Path: dirA, Name: "Alpha", Tags: []string{"work"}},
		{Path: dirB, Name: "Bravo", Tags: []string{"play"}},
	}
}
