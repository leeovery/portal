package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §6 composed Projects-page gate. These tests assert viewProjectList composes
// the §3.1 PORTAL header block, the §6 Projects section header (state.green label +
// text.detail count + right-aligned `/ to filter` hint), the two-line MV rows, and
// the §6.3 condensed footer — replacing the legacy plain bubbles/list title + the
// three-column renderKeymapFooter. Matches testdata/vhs/reference/projects-mv.png.
//
// No t.Parallel() — the package's shared canvas/mock helpers make parallelism
// unsafe across these tests.

// newProjectsPageTestModel builds a Model on the Projects page seeded with the
// given projects at the given size and canvas mode, sized so pagination/footer
// budgets are applied (mirroring newCanvasTestModel for the Projects page).
func newProjectsPageTestModel(t *testing.T, w, h int, mode theme.Mode, projects []project.Project) Model {
	t.Helper()
	m := New(fakeLister{}, WithCanvasMode(mode))
	m.termWidth = w
	m.termHeight = h
	m.activePage = PageProjects
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	return m
}

func sampleProjects() []project.Project {
	return []project.Project{
		{Name: "flow-v1-api", Path: "/Users/leeovery/Code/fabric/flowv1/flow-v1-api"},
		{Name: "portal", Path: "/Users/leeovery/Code/portal"},
		{Name: "mint", Path: "/Users/leeovery/Code/mint"},
		{Name: "agntc", Path: "/Users/leeovery/Code/agntc"},
	}
}

// TestViewProjectList_ComposesHeaderSectionAndFooter asserts the composed Projects
// view renders the PORTAL wordmark header block, the Projects section header
// (state.green label + text.detail count + `/ to filter` hint), and the §6.3
// condensed footer — none of the legacy chrome.
func TestViewProjectList_ComposesHeaderSectionAndFooter(t *testing.T) {
	m := newProjectsPageTestModel(t, 90, 24, theme.Dark, sampleProjects())
	view := m.viewProjectList()
	visible := ansi.Strip(view)

	// §3.1 PORTAL header block.
	if !strings.Contains(visible, "P O R T A L") {
		t.Errorf("composed Projects view missing the PORTAL wordmark:\n%s", visible)
	}
	// §6 section header: `Projects` in state.green, count in text.detail.
	if !strings.Contains(visible, "Projects") {
		t.Errorf("composed Projects view missing the Projects section label:\n%s", visible)
	}
	if seq := tokenFgSeq(t, theme.MV.StateGreen, theme.Dark); !strings.Contains(view, seq) {
		t.Errorf("composed Projects view missing the state.green label role sequence %q", seq)
	}
	countRun := headerStyle(theme.MV.TextDetail, theme.Dark, false).Render("4")
	if !strings.Contains(view, countRun) {
		t.Errorf("composed Projects view missing the text.detail count run for 4 projects:\n%s", view)
	}
	if !strings.Contains(view, sectionFilterHint) {
		t.Errorf("composed Projects view missing the %q hint:\n%s", sectionFilterHint, view)
	}

	// §6.3 condensed footer copy (replaces the legacy three-column footer).
	for _, want := range []string{"⏎ new session", "x sessions", "e edit", "/ filter", "? help"} {
		if !strings.Contains(visible, want) {
			t.Errorf("composed Projects view missing the condensed footer entry %q:\n%s", want, visible)
		}
	}
	// The legacy three-column footer copy must be gone (e.g. `new in cwd`, `delete`
	// were the manual footer's wording).
	for _, banned := range []string{"new in cwd", "delete"} {
		if strings.Contains(visible, banned) {
			t.Errorf("composed Projects view leaked legacy three-column footer copy %q:\n%s", banned, visible)
		}
	}
}

// TestViewProjectList_HeaderSectionRowsShareLeftEdge is the cross-element alignment
// guard: the PORTAL wordmark, the `Projects` section-header label, and the selected
// row's ▌ bar must all start at the SAME column — the content's left edge.
func TestViewProjectList_HeaderSectionRowsShareLeftEdge(t *testing.T) {
	m := newProjectsPageTestModel(t, 90, 24, theme.Dark, sampleProjects())
	view := m.viewProjectList()

	var wordmarkCol, sectionCol, barCol = -1, -1, -1
	for _, line := range strings.Split(view, "\n") {
		stripped := strings.TrimLeft(ansi.Strip(line), " ")
		switch {
		case strings.HasPrefix(stripped, "P O R T A L"):
			wordmarkCol = leadingPrintableCol(line)
		case strings.HasPrefix(stripped, "Projects"):
			sectionCol = leadingPrintableCol(line)
		case strings.HasPrefix(stripped, "▌") && barCol < 0:
			barCol = leadingPrintableCol(line)
		}
	}
	if wordmarkCol < 0 || sectionCol < 0 || barCol < 0 {
		t.Fatalf("composed view missing a measured row: wordmarkCol=%d sectionCol=%d barCol=%d\n%s", wordmarkCol, sectionCol, barCol, view)
	}
	if wordmarkCol != sectionCol || sectionCol != barCol {
		t.Errorf("left edges differ: PORTAL=%d Projects=%d bar=%d; all three must share the content's left edge", wordmarkCol, sectionCol, barCol)
	}
}

// TestViewProjectList_ModalClearsToCanvas asserts the §8.1/13.5 blank-screen modal
// layer is preserved by the reskin: when a project modal is open the page is cleared
// to the centred panel (no list/header/footer chrome composed behind it).
func TestViewProjectList_ModalClearsToCanvas(t *testing.T) {
	m := newProjectsPageTestModel(t, 90, 24, theme.Dark, sampleProjects())
	m.modal = modalDeleteProject
	m.pendingDeleteName = "portal"
	view := m.viewProjectList()
	visible := ansi.Strip(view)

	if !strings.Contains(visible, "Delete portal? (y/n)") {
		t.Errorf("delete modal not rendered on the cleared canvas:\n%s", visible)
	}
	// The list/footer chrome must NOT be composed behind the modal.
	if strings.Contains(visible, "⏎ new session") || strings.Contains(visible, "P O R T A L") {
		t.Errorf("modal frame leaked the list/header/footer chrome:\n%s", visible)
	}
}
