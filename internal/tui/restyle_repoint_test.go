package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
)

const (
	probeRedBefore = 0xAA
	probeRedAfter  = 0xBB
)

func probeThemeBefore(t *testing.T) theme.Theme {
	t.Helper()
	before, _ := themetest.SyntheticPair(t, probeRedBefore, probeRedAfter)
	return before
}

func probeThemeAfter(t *testing.T) theme.Theme {
	t.Helper()
	_, after := themetest.SyntheticPair(t, probeRedBefore, probeRedAfter)
	return after
}

func newRestyleProbeModel(t *testing.T, before theme.Theme) Model {
	t.Helper()
	return populateRestyleProbe(t, Build(Deps{Lister: fakeLister{}, Theme: theme.ConstantNomination(before)}))
}

func populateRestyleProbe(t *testing.T, m Model) Model {
	t.Helper()
	const w, h = 120, 24

	sessions := make([]tmux.Session, 0, 60)
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	projects := make([]project.Project, 0, 40)
	for i := range 40 {
		projects = append(projects, project.Project{
			Name: nameN(i),
			Path: "/Users/leeovery/Code/" + nameN(i),
		})
	}

	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())

	if m.sessionList.Paginator.TotalPages < 2 {
		t.Fatalf("probe setup: want a multi-page session list, got TotalPages=%d", m.sessionList.Paginator.TotalPages)
	}
	if m.projectList.Paginator.TotalPages < 2 {
		t.Fatalf("probe setup: want a multi-page project list, got TotalPages=%d", m.projectList.Paginator.TotalPages)
	}

	// Render both pages: the caches under test are assigned at construction,
	// and a model rendered only after the swap would pass trivially.
	m.activePage = PageSessions
	_ = m.viewSessionList()
	m.activePage = PageProjects
	_ = m.viewProjectList()
	m.activePage = PageSessions

	return m
}

func restyleTo(m *Model, after theme.Theme) {
	m.ApplyTheme(after)
}

type probedList struct {
	name string
	list *list.Model
}

// The panel's third list exists only while it is open; restyledSurfaces covers it.
func probedLists(m *Model) []probedList {
	return []probedList{
		{"sessions", &m.sessionList},
		{"projects", &m.projectList},
	}
}

func assertRepointed(t *testing.T, what, rendered, want, stale string) {
	t.Helper()
	if !strings.Contains(rendered, want) {
		t.Errorf("%s does not carry the post-swap SGR %q — the restyle path did not re-point it: %q", what, want, escSeq(rendered))
	}
	if strings.Contains(rendered, stale) {
		t.Errorf("%s still carries the pre-swap SGR %q — the previous theme's colour survived the restyle: %q", what, stale, escSeq(rendered))
	}
}

func TestRestylePath_RepointsListOwnedStyles(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newRestyleProbeModel(t, before)
	restyleTo(&m, after)

	for _, pl := range probedLists(&m) {
		t.Run(pl.name, func(t *testing.T) {
			assertRepointed(t, pl.name+" Styles.HelpStyle",
				pl.list.Styles.HelpStyle.Render("x"),
				tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			activeDot := pl.list.Styles.ActivePaginationDot.String()
			assertRepointed(t, pl.name+" Styles.ActivePaginationDot foreground",
				activeDot, tokenFgSeq(t, after.AccentPrimary), tokenFgSeq(t, before.AccentPrimary))
			assertRepointed(t, pl.name+" Styles.ActivePaginationDot background",
				activeDot, tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			inactiveDot := pl.list.Styles.InactivePaginationDot.String()
			assertRepointed(t, pl.name+" Styles.InactivePaginationDot foreground",
				inactiveDot, tokenFgSeq(t, after.TextFaint), tokenFgSeq(t, before.TextFaint))
			assertRepointed(t, pl.name+" Styles.InactivePaginationDot background",
				inactiveDot, tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			// list.New snapshots the dot strings from the styles at construction,
			// so re-pointing only the styles leaves the old theme's dots forever.
			assertRepointed(t, pl.name+" Paginator.ActiveDot",
				pl.list.Paginator.ActiveDot, tokenFgSeq(t, after.AccentPrimary), tokenFgSeq(t, before.AccentPrimary))
			assertRepointed(t, pl.name+" Paginator.InactiveDot",
				pl.list.Paginator.InactiveDot, tokenFgSeq(t, after.TextFaint), tokenFgSeq(t, before.TextFaint))

			// The paginator escalates to the arabic form whenever the dot row is
			// wider than the list, so it must follow the swap like the dots.
			arabic := pl.list.Styles.ArabicPagination.Render("x")
			assertRepointed(t, pl.name+" Styles.ArabicPagination foreground",
				arabic, tokenFgSeq(t, after.TextFaint), tokenFgSeq(t, before.TextFaint))
			assertRepointed(t, pl.name+" Styles.ArabicPagination background",
				arabic, tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			noItems := pl.list.Styles.NoItems.Render("x")
			assertRepointed(t, pl.name+" Styles.NoItems foreground",
				noItems, tokenFgSeq(t, after.TextMuted), tokenFgSeq(t, before.TextMuted))
			assertRepointed(t, pl.name+" Styles.NoItems background",
				noItems, tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			assertRepointed(t, pl.name+" Styles.TitleBar",
				pl.list.Styles.TitleBar.Render("x"),
				tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			assertRepointed(t, pl.name+" Styles.PaginationStyle",
				pl.list.Styles.PaginationStyle.Render("x"),
				tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			if titleRun := pl.list.Styles.Title.Render("x"); strings.ContainsRune(titleRun, '\x1b') {
				t.Errorf("%s Styles.Title emits colour %q — nothing paints the title box (the section header replaces the row), so it must carry none", pl.name, escSeq(titleRun))
			}
		})
	}
}

func TestRestylePath_RepointsBothFilterInputs(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newRestyleProbeModel(t, before)
	restyleTo(&m, after)

	wantSeq := tokenFgSeq(t, after.AccentAttention)
	staleSeq := tokenFgSeq(t, before.AccentAttention)

	for _, pl := range probedLists(&m) {
		t.Run(pl.name, func(t *testing.T) {
			styles := pl.list.FilterInput.Styles()
			assertRepointed(t, pl.name+" FilterInput Focused.Prompt",
				styles.Focused.Prompt.Render(filterPromptPrefix), wantSeq, staleSeq)
			assertRepointed(t, pl.name+" FilterInput Focused.Text",
				styles.Focused.Text.Render("query"), wantSeq, staleSeq)
			assertRepointed(t, pl.name+" FilterInput Cursor.Color",
				lipgloss.NewStyle().Foreground(styles.Cursor.Color).Render("x"), wantSeq, staleSeq)
		})
	}
}

// bubbles/list keeps the delegate unexported, so a rendered row is the only
// available observation.
func TestRestylePath_RepointsBothDelegates(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newRestyleProbeModel(t, before)
	restyleTo(&m, after)

	wantSeq := tokenFgSeq(t, after.TextOnSelection)
	staleSeq := tokenFgSeq(t, before.TextOnSelection)

	m.activePage = PageSessions
	assertRepointed(t, "SessionDelegate selected-row name", m.viewSessionList(), wantSeq, staleSeq)

	m.activePage = PageProjects
	assertRepointed(t, "ProjectDelegate selected-row name", m.viewProjectList(), wantSeq, staleSeq)
}

func TestRestylePath_RepointsPreviewChrome(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newRestyleProbeModel(t, before)

	pv := newPreviewModelForHelpers(t, "alpha", []tmux.WindowGroup{{WindowIndex: 0, PaneIndices: []int{0}}}, 0, 0)
	pv.th = before
	m.preview = pv

	restyleTo(&m, after)

	assertRepointed(t, "previewModel chrome (accent.mode marker)",
		chromeLineForTest(m.preview),
		tokenFgSeq(t, after.AccentMode), tokenFgSeq(t, before.AccentMode))
}

// Enough rows to paginate at the probe height: without the dot row on the
// frame, the scan below finds nothing and passes vacuously.
const restylePanelProbeRows = 20

// Every union row carries `before`: opening applies the cursor row's theme, so
// any other palette would repaint the model and the swap would not be a swap.
func newRestylePanelProbeModel(t *testing.T, before theme.Theme) Model {
	t.Helper()

	rows := make([]theme.Row, 0, restylePanelProbeRows)
	for i := range restylePanelProbeRows {
		rows = append(rows, theme.Row{Slug: arrowSlug(i), Source: theme.SourceBuiltin, Theme: before})
	}

	m := populateRestyleProbe(t, Build(newArrowPanelDeps(t, rows, arrowSlug(0))))
	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("probe setup: the panel did not open, so its list instance was never built")
	}
	if got := m.themePanel.list.Paginator.TotalPages; got < 2 {
		t.Fatalf("probe setup: want a multi-page panel list, got TotalPages=%d", got)
	}
	if m.themeState.active != before {
		t.Fatalf("probe setup: the open painted canvas %s, want the pre-swap %s", m.themeState.active.Canvas.Value, before.Canvas.Value)
	}
	_ = m.View()

	return m
}

type restyledSurface struct {
	name     string
	rendered string
}

func restyledSurfaces(t *testing.T, m *Model) []restyledSurface {
	t.Helper()

	m.activePage = PageSessions
	sessions := m.viewSessionList()
	m.activePage = PageProjects
	projects := m.viewProjectList()
	m.activePage = PageSessions

	return []restyledSurface{
		{"sessions", sessions},
		{"projects", projects},
		{"theme panel", renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless)},
	}
}

func TestRestylePath_NoStaleColourSurvivesOnAnyList(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newRestylePanelProbeModel(t, before)

	for _, surface := range restyledSurfaces(t, &m) {
		if !strings.Contains(surface.rendered, tokenBgSeq(t, before.Canvas)) {
			t.Fatalf("probe setup: the %s surface carries no pre-swap canvas, so a clean scan after the swap would prove nothing about it", surface.name)
		}
	}

	restyleTo(&m, after)

	for _, surface := range restyledSurfaces(t, &m) {
		t.Run(surface.name, func(t *testing.T) {
			if stale := staleRuns(t, surface.rendered, before); len(stale) > 0 {
				t.Errorf("the %s surface still renders %v from the pre-swap palette — a cached style on that list was never re-pointed: %q", surface.name, stale, escSeq(surface.rendered))
			}
			if !strings.Contains(surface.rendered, tokenBgSeq(t, after.Canvas)) {
				t.Errorf("the %s surface carries no post-swap canvas, so the clean scan above is an unpainted surface rather than a re-pointed one: %q", surface.name, escSeq(surface.rendered))
			}
		})
	}
}

func staleRuns(t *testing.T, rendered string, palette theme.Theme) []string {
	t.Helper()
	var found []string
	for _, tok := range palette.All() {
		if strings.Contains(rendered, tokenFgSeq(t, tok)) {
			found = append(found, tok.Name+" (foreground)")
		}
		if strings.Contains(rendered, tokenBgSeq(t, tok)) {
			found = append(found, tok.Name+" (background)")
		}
	}
	return found
}
