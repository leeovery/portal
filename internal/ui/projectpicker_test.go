package ui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/ui"
)

// mockProjectStore implements ui.ProjectStore for testing.
type mockProjectStore struct {
	projects    []project.Project
	listErr     error
	cleanCalled bool
}

func (m *mockProjectStore) List() ([]project.Project, error) {
	return m.projects, m.listErr
}

func (m *mockProjectStore) CleanStale() (int, error) {
	m.cleanCalled = true
	return 0, nil
}

func projectsLoaded(projects []project.Project) ui.ProjectsLoadedMsg {
	return ui.ProjectsLoadedMsg{Projects: projects}
}

// initModel creates a project picker and loads it with projects from the store.
func initModel(store *mockProjectStore) tea.Model {
	m := ui.NewProjectPicker(store)
	updated, _ := m.Update(projectsLoaded(store.projects))
	return updated
}

// sendKeys applies a sequence of key messages to a model, returning the final model.
func sendKeys(m tea.Model, keys ...tea.Msg) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func keyDown() tea.Msg  { return tea.KeyMsg{Type: tea.KeyDown} }
func keyEnter() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyEsc() tea.Msg   { return tea.KeyMsg{Type: tea.KeyEsc} }
func keyRune(r rune) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
func keyBackspace() tea.Msg { return tea.KeyMsg{Type: tea.KeyBackspace} }

func threeProjects() []project.Project {
	return []project.Project{
		{Path: "/code/newest", Name: "newest", LastUsed: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{Path: "/code/middle", Name: "middle", LastUsed: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{Path: "/code/oldest", Name: "oldest", LastUsed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func TestProjectPicker_DisplaysProjectsSortedByLastUsed(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})
	view := m.View()

	newestIdx := strings.Index(view, "newest")
	middleIdx := strings.Index(view, "middle")
	oldestIdx := strings.Index(view, "oldest")

	if newestIdx == -1 || middleIdx == -1 || oldestIdx == -1 {
		t.Fatalf("not all project names found in view:\n%s", view)
	}
	if newestIdx >= middleIdx {
		t.Errorf("newest (idx %d) should appear before middle (idx %d)", newestIdx, middleIdx)
	}
	if middleIdx >= oldestIdx {
		t.Errorf("middle (idx %d) should appear before oldest (idx %d)", middleIdx, oldestIdx)
	}
}

func TestProjectPicker_ShowsBrowseOptionAtBottom(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})
	view := m.View()

	if !strings.Contains(view, "browse for directory...") {
		t.Errorf("view missing 'browse for directory...':\n%s", view)
	}

	browseIdx := strings.Index(view, "browse for directory...")
	oldestIdx := strings.Index(view, "oldest")
	if browseIdx <= oldestIdx {
		t.Errorf("browse option (idx %d) should appear after oldest project (idx %d)", browseIdx, oldestIdx)
	}
}

func TestProjectPicker_EmptyStateShowsMessage(t *testing.T) {
	m := initModel(&mockProjectStore{projects: []project.Project{}})
	view := m.View()

	if !strings.Contains(view, "No saved projects yet.") {
		t.Errorf("view missing empty state message:\n%s", view)
	}
	if !strings.Contains(view, "browse for directory...") {
		t.Errorf("view missing browse option in empty state:\n%s", view)
	}
}

func TestProjectPicker_EnterOnProjectEmitsPath(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Cursor starts at first project ("newest"); press Enter
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter, got nil")
	}

	msg := cmd()
	sel, ok := msg.(ui.ProjectSelectedMsg)
	if !ok {
		t.Fatalf("expected ProjectSelectedMsg, got %T", msg)
	}
	if sel.Path != "/code/newest" {
		t.Errorf("expected path %q, got %q", "/code/newest", sel.Path)
	}
}

func TestProjectPicker_EnterOnBrowseEmitsBrowseAction(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Navigate past all 3 projects to the browse option
	m = sendKeys(m, keyDown(), keyDown(), keyDown())

	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter on browse, got nil")
	}

	msg := cmd()
	if _, ok := msg.(ui.BrowseSelectedMsg); !ok {
		t.Fatalf("expected BrowseSelectedMsg, got %T", msg)
	}
}

func TestProjectPicker_EscEmitsBackMessage(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	_, cmd := m.Update(keyEsc())
	if cmd == nil {
		t.Fatal("expected command from Esc, got nil")
	}

	msg := cmd()
	if _, ok := msg.(ui.BackMsg); !ok {
		t.Fatalf("expected BackMsg, got %T", msg)
	}
}

func TestProjectPicker_NavigationUpDown(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// down to "middle"
	m = sendKeys(m, keyDown())
	_, cmd := m.Update(keyEnter())
	msg := cmd()
	sel := msg.(ui.ProjectSelectedMsg)
	if sel.Path != "/code/middle" {
		t.Errorf("expected path %q, got %q", "/code/middle", sel.Path)
	}
}

func TestProjectPicker_NavigationJK(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// j moves down twice to "oldest"
	m = sendKeys(m, keyRune('j'), keyRune('j'))
	_, cmd := m.Update(keyEnter())
	msg := cmd()
	sel := msg.(ui.ProjectSelectedMsg)
	if sel.Path != "/code/oldest" {
		t.Errorf("expected path %q after j/j, got %q", "/code/oldest", sel.Path)
	}

	// Reset and test k
	m = initModel(&mockProjectStore{projects: threeProjects()})
	m = sendKeys(m, keyRune('j'), keyRune('j'), keyRune('k'))
	// Should be on "middle" (index 1)
	_, cmd = m.Update(keyEnter())
	msg = cmd()
	sel = msg.(ui.ProjectSelectedMsg)
	if sel.Path != "/code/middle" {
		t.Errorf("expected path %q after j/j/k, got %q", "/code/middle", sel.Path)
	}
}

func TestProjectPicker_EmptyStateEnterOnBrowse(t *testing.T) {
	m := initModel(&mockProjectStore{projects: []project.Project{}})

	// In empty state, cursor should be on browse option
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter in empty state, got nil")
	}

	msg := cmd()
	if _, ok := msg.(ui.BrowseSelectedMsg); !ok {
		t.Fatalf("expected BrowseSelectedMsg in empty state, got %T", msg)
	}
}

func TestProjectPicker_SlashActivatesFilterMode(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	m = sendKeys(m, keyRune('/'))

	view := m.View()
	if !strings.Contains(view, "filter:") {
		t.Errorf("filter mode should show filter prompt:\n%s", view)
	}
}

func TestProjectPicker_TypingNarrowsProjectListByFuzzyMatch(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Activate filter mode and type "new"
	m = sendKeys(m, keyRune('/'), keyRune('n'), keyRune('e'), keyRune('w'))

	view := m.View()
	if !strings.Contains(view, "newest") {
		t.Errorf("filtered view should contain 'newest':\n%s", view)
	}
	if strings.Contains(view, "middle") {
		t.Errorf("filtered view should not contain 'middle':\n%s", view)
	}
	if strings.Contains(view, "oldest") {
		t.Errorf("filtered view should not contain 'oldest':\n%s", view)
	}
}

func TestProjectPicker_BrowseAlwaysVisibleDuringFilter(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Activate filter mode and type something that matches nothing
	m = sendKeys(m, keyRune('/'), keyRune('z'), keyRune('z'), keyRune('z'))

	view := m.View()
	if !strings.Contains(view, "browse for directory...") {
		t.Errorf("browse option should remain visible during filter:\n%s", view)
	}
}

func TestProjectPicker_BackspaceOnEmptyFilterExitsFilterMode(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Activate filter mode, type one char, backspace to empty, backspace again exits
	m = sendKeys(m, keyRune('/'), keyRune('a'), keyBackspace(), keyBackspace())

	view := m.View()
	if strings.Contains(view, "filter:") {
		t.Errorf("should have exited filter mode after backspace on empty filter:\n%s", view)
	}
}

func TestProjectPicker_EscClearsFilterAndExitsFilterMode(t *testing.T) {
	m := initModel(&mockProjectStore{projects: threeProjects()})

	// Activate filter mode, type something, then Esc
	m = sendKeys(m, keyRune('/'), keyRune('n'), keyRune('e'), keyRune('w'))

	// Verify filtering is active
	view := m.View()
	if !strings.Contains(view, "filter:") {
		t.Fatalf("expected to be in filter mode:\n%s", view)
	}

	// Esc should clear filter and exit filter mode
	m = sendKeys(m, keyEsc())

	view = m.View()
	if strings.Contains(view, "filter:") {
		t.Errorf("should have exited filter mode after Esc:\n%s", view)
	}
	// All projects should be visible again
	if !strings.Contains(view, "newest") || !strings.Contains(view, "middle") || !strings.Contains(view, "oldest") {
		t.Errorf("all projects should be visible after clearing filter:\n%s", view)
	}
}
