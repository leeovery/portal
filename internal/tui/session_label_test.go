package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func rowNames(items []list.Item) []string {
	var names []string
	for _, si := range sessionRows(items) {
		names = append(names, si.Session.Name)
	}
	return names
}

func TestFilterMatchesLabels(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz"},
	}
	for _, tc := range []struct{ query, want string }{
		{"checkout", "api-x7Kd9a"},
		{"x7Kd9a", "api-x7Kd9a"},
		{"Qm3pLz", "web-Qm3pLz"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)
			m.rebuildSessionList()

			m.sessionList.SetFilterText(tc.query)
			m.sessionList.SetFilterState(list.FilterApplied)

			if got := rowNames(m.sessionList.VisibleItems()); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("filter %q left %v, want [%s]", tc.query, got, tc.want)
			}
		})
	}
}

func TestSearchMatchesLabels(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz", Label: "landing page"},
		{Name: "checkout-Hq7wNe"},
	}

	m := searchColumnModel(t, prefs.ModeFlat, "checkout", sessions, nil)

	if got, want := rowNames(m.sessionList.VisibleItems()), []string{"api-x7Kd9a", "checkout-Hq7wNe"}; !slices.Equal(got, want) {
		t.Errorf("search narrowed to %v, want %v", got, want)
	}
}

func TestRowsOrderByDisplayName(t *testing.T) {
	dir := "/home/user/code/portal"
	projects := []project.Project{{Path: dir, Name: "portal", Tags: []string{"work"}}}
	// zeta's label reads first; alpha and beta show the same text, so their
	// names settle their order; the catch-all sorts the same way.
	sessions := []Session{
		{Name: "alpha", Label: "same", Dir: dir},
		{Name: "zeta", Label: "aardvark", Dir: dir},
		{Name: "beta", Label: "same", Dir: dir},
		{Name: "mid", Dir: dir},
		{Name: "loose-b", Label: "a loose one"},
		{Name: "loose-a"},
	}
	inGroup := []string{"zeta", "mid", "alpha", "beta"}
	catchAll := []string{"loose-b", "loose-a"}

	for _, tc := range []struct {
		mode prefs.SessionListMode
		want []string
	}{
		{prefs.ModeFlat, []string{"loose-b", "zeta", "loose-a", "mid", "alpha", "beta"}},
		{prefs.ModeByProject, slices.Concat(inGroup, catchAll)},
		{prefs.ModeByTag, slices.Concat(inGroup, catchAll)},
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			m := newRebuildTestModel(t, tc.mode, sessions, projects)
			m.rebuildSessionList()

			if got := rowNames(m.sessionList.Items()); !slices.Equal(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("the signpost's flat list too", func(t *testing.T) {
		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, []project.Project{{Path: dir, Name: "portal"}})
		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatal("precondition: no tags anywhere must raise the signpost")
		}
		if got, want := rowNames(m.sessionList.Items()), []string{"loose-b", "zeta", "loose-a", "mid", "alpha", "beta"}; !slices.Equal(got, want) {
			t.Errorf("rows = %v, want %v", got, want)
		}
	})
}

func TestCurrentSessionTitleShowsItsLabel(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz"},
	}
	m := Build(Deps{Lister: fakeLister{}, CurrentSession: "api-x7Kd9a"})
	m.termWidth, m.termHeight = 100, 24
	m.applySessionListSize(m.contentWidth(), m.contentHeight())

	if got := m.sessionList.Title; !strings.Contains(got, "(current: api-x7Kd9a)") {
		t.Errorf("title before the listing = %q, want the current session by its name", got)
	}

	m.applySessions(sessions, nil)

	if got := m.sessionList.Title; !strings.Contains(got, "(current: checkout flow)") {
		t.Errorf("title = %q, want the current session by its label", got)
	}
	if got := ansi.Strip(m.viewSessionList()); !strings.Contains(got, "current: checkout flow") {
		t.Errorf("section header does not name the current session by its label:\n%s", got)
	}
	if got := rowNames(m.sessionList.Items()); !slices.Equal(got, []string{"web-Qm3pLz"}) {
		t.Errorf("rows = %v, want the current session left out", got)
	}
}

func TestNoPendingOrSkeletonReaders(t *testing.T) {
	m := Build(Deps{Lister: fakeLister{}})

	if cmd := m.startPendingSettle(); cmd != nil {
		t.Error("with no readers the settle loop started")
	}
	if pending := m.readPendingSessions(); pending != nil {
		t.Errorf("pending = %v, want none with no reader", pending)
	}
	m.applySessions([]Session{{Name: "api-x7Kd9a"}}, m.readPendingSessions())
	if got := rowNames(m.sessionList.Items()); !slices.Equal(got, []string{"api-x7Kd9a"}) {
		t.Errorf("rows = %v, want the session listed", got)
	}
}

// A label is any text: the picker cuts a long current label, as rows are cut,
// so every line stays within the terminal and the filter hint survives.
func TestLongCurrentLabelStaysWithinTheScreen(t *testing.T) {
	const width, height = 63, 20
	long := strings.Repeat("very long label ", 8)
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: long},
		{Name: "web-Qm3pLz"},
	}
	for _, mode := range []prefs.SessionListMode{prefs.ModeFlat, prefs.ModeByProject, prefs.ModeByTag} {
		t.Run(mode.String(), func(t *testing.T) {
			m := Build(Deps{Lister: fakeLister{}, CurrentSession: "api-x7Kd9a", InitialMode: mode})
			m = asModel(t, dropCmd(m.Update(tea.WindowSizeMsg{Width: width, Height: height})))
			m.applySessions(sessions, nil)
			m.sessionsLoaded = true

			frame := ansi.Strip(m.View().Content)

			for i, line := range strings.Split(frame, "\n") {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("line %d is %d cells, wider than the %d-cell screen: %q", i, w, width, line)
				}
			}
			if !strings.Contains(frame, sectionFilterHint) {
				t.Errorf("the %q hint was pushed off the header:\n%s", sectionFilterHint, frame)
			}
			if !strings.Contains(frame, "(current: very long ") || !strings.Contains(frame, "…)") {
				t.Errorf("the header does not name the current session by its cut label:\n%s", frame)
			}
			if lipgloss.Width(m.sessionList.Title) > m.sessionList.Width() {
				t.Errorf("title %q is wider than the list's %d cells", m.sessionList.Title, m.sessionList.Width())
			}
		})
	}
}

func TestPreviewHeaderShowsTheLabel(t *testing.T) {
	long := strings.Repeat("checkout flow ", 10)
	sessions := []Session{{Name: "api-x7Kd9a", Label: long}, {Name: "web-Qm3pLz", Label: "landing page"}}
	history := historyFor(t, "hi", sessions)
	m := openPreviewOn(t, modelWithSeams(t, sessions, history))

	header := previewHeaderLine(t, m)

	if !strings.Contains(header, "checkout flow checkout") || !strings.Contains(header, "…") {
		t.Errorf("preview header = %q, want the label cut to fit", header)
	}
	if strings.Contains(header, "api-x7Kd9a") {
		t.Errorf("preview header = %q, want the label, not the name", header)
	}
	if w := lipgloss.Width(header); w > 80 {
		t.Errorf("preview header is %d cells, wider than the 80-cell screen", w)
	}
	if got := historyCalls(history); !slices.Equal(got, []string{"api-x7Kd9a"}) {
		t.Errorf("history read for %v, want the session's name", got)
	}

	updated, step := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated, read := asModel(t, updated).Update(step())
	m = landRead(t, asModel(t, updated), read)
	if header := previewHeaderLine(t, m); !strings.Contains(header, "landing page") {
		t.Errorf("stepped preview header = %q, want the next session's label", header)
	}
	if got := historyCalls(history); !slices.Equal(got, []string{"api-x7Kd9a", "web-Qm3pLz"}) {
		t.Errorf("history read for %v, want each session's name", got)
	}
}

func previewHeaderLine(t *testing.T, m Model) string {
	t.Helper()
	for line := range strings.SplitSeq(ansi.Strip(m.View().Content), "\n") {
		if strings.Contains(line, previewMarker) {
			return line
		}
	}
	t.Fatalf("no preview header in:\n%s", ansi.Strip(m.View().Content))
	return ""
}
