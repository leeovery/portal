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

func (m *mockProjectStore) CleanStale() ([]project.Project, error) {
	m.cleanCalled = true
	return nil, nil
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

func keyTab() tea.Msg { return tea.KeyMsg{Type: tea.KeyTab} }

// mockProjectEditor implements ui.ProjectEditor for testing.
type mockProjectEditor struct {
	renamedPath string
	renamedName string
	renameErr   error
}

func (m *mockProjectEditor) Rename(path, newName string) error {
	m.renamedPath = path
	m.renamedName = newName
	return m.renameErr
}

// mockAliasEditor implements ui.AliasEditor for testing.
type mockAliasEditor struct {
	aliases    map[string]string
	loadErr    error
	saveErr    error
	setCalls   []setCall
	deletions  []string
	saveCalled bool
}

type setCall struct {
	name, path string
}

func newMockAliasEditor(aliases map[string]string) *mockAliasEditor {
	if aliases == nil {
		aliases = make(map[string]string)
	}
	return &mockAliasEditor{aliases: aliases}
}

func (m *mockAliasEditor) Load() (map[string]string, error) {
	return m.aliases, m.loadErr
}

func (m *mockAliasEditor) Set(name, path string) {
	m.setCalls = append(m.setCalls, setCall{name, path})
	m.aliases[name] = path
}

func (m *mockAliasEditor) Delete(name string) bool {
	_, ok := m.aliases[name]
	if ok {
		delete(m.aliases, name)
		m.deletions = append(m.deletions, name)
	}
	return ok
}

func (m *mockAliasEditor) Save() error {
	m.saveCalled = true
	return m.saveErr
}

// initEditModel creates a project picker with edit support and loads it with projects.
func initEditModel(store *mockProjectStore, editor *mockProjectEditor, aliasEditor *mockAliasEditor) tea.Model {
	m := ui.NewProjectPicker(store).WithEditor(editor, aliasEditor)
	updated, _ := m.Update(projectsLoaded(store.projects))
	return updated
}

func TestProjectPicker_EditMode_EntersWithCurrentName(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Press e on first project ("newest")
	m = sendKeys(m, keyRune('e'))

	view := m.View()
	if !strings.Contains(view, "Name:") {
		t.Errorf("edit mode should show Name field:\n%s", view)
	}
	if !strings.Contains(view, "newest") {
		t.Errorf("edit mode should show current project name 'newest':\n%s", view)
	}
}

func TestProjectPicker_EditMode_DisplaysAliasesForProjectDir(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliases := map[string]string{
		"new":   "/code/newest",
		"n":     "/code/newest",
		"other": "/code/middle",
	}
	aliasEditor := newMockAliasEditor(aliases)
	m := initEditModel(store, editor, aliasEditor)

	// Press e on first project ("newest", path=/code/newest)
	m = sendKeys(m, keyRune('e'))

	view := m.View()
	// Should show aliases "new" and "n" (both map to /code/newest)
	if !strings.Contains(view, "new") {
		t.Errorf("edit mode should display alias 'new':\n%s", view)
	}
	if !strings.Contains(view, "[x] n") {
		t.Errorf("edit mode should display alias 'n' with remove marker:\n%s", view)
	}
	// Should NOT show "other" (maps to /code/middle)
	if strings.Contains(view, "other") {
		t.Errorf("edit mode should not display alias 'other' (different path):\n%s", view)
	}
}

func TestProjectPicker_EditMode_NoAliasesShowsEmpty(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	m = sendKeys(m, keyRune('e'))

	view := m.View()
	if !strings.Contains(view, "(none)") {
		t.Errorf("edit mode with no aliases should show (none):\n%s", view)
	}
}

func TestProjectPicker_EditMode_TabCyclesBetweenFields(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode - starts on name field
	m = sendKeys(m, keyRune('e'))
	view := m.View()
	lines := strings.Split(view, "\n")
	var nameLine string
	for _, line := range lines {
		if strings.Contains(line, "Name:") {
			nameLine = line
			break
		}
	}
	if !strings.HasPrefix(nameLine, "> ") {
		t.Errorf("name field should have focus indicator initially, got: %q", nameLine)
	}

	// Tab to aliases
	m = sendKeys(m, keyTab())
	view = m.View()
	lines = strings.Split(view, "\n")
	var aliasLine string
	for _, line := range lines {
		if strings.Contains(line, "Aliases:") {
			aliasLine = line
			break
		}
	}
	if !strings.HasPrefix(aliasLine, "> ") {
		t.Errorf("aliases field should have focus indicator after Tab, got: %q", aliasLine)
	}

	// Tab again back to name
	m = sendKeys(m, keyTab())
	view = m.View()
	lines = strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Name:") {
			nameLine = line
			break
		}
	}
	if !strings.HasPrefix(nameLine, "> ") {
		t.Errorf("name field should have focus indicator after second Tab, got: %q", nameLine)
	}
}

func TestProjectPicker_EditMode_EnterSavesNameChange(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode on "newest"
	m = sendKeys(m, keyRune('e'))

	// Clear the name and type a new one
	for range len("newest") {
		m = sendKeys(m, keyBackspace())
	}
	m = sendKeys(m, keyRune('r'), keyRune('e'), keyRune('n'), keyRune('a'), keyRune('m'), keyRune('e'), keyRune('d'))

	// Press Enter to confirm
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter in edit mode, got nil")
	}

	if editor.renamedPath != "/code/newest" {
		t.Errorf("expected rename path %q, got %q", "/code/newest", editor.renamedPath)
	}
	if editor.renamedName != "renamed" {
		t.Errorf("expected rename name %q, got %q", "renamed", editor.renamedName)
	}
}

func TestProjectPicker_EditMode_EscCancelsWithoutSaving(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode, modify name
	m = sendKeys(m, keyRune('e'))
	m = sendKeys(m, keyBackspace(), keyBackspace(), keyBackspace())

	// Esc should cancel
	m = sendKeys(m, keyEsc())

	// Should be back to normal view
	view := m.View()
	if strings.Contains(view, "Name:") {
		t.Errorf("should have exited edit mode after Esc:\n%s", view)
	}

	// Editor should NOT have been called
	if editor.renamedPath != "" {
		t.Errorf("editor should not have been called on cancel, but renamedPath=%q", editor.renamedPath)
	}
	// Alias save should NOT have been called
	if aliasEditor.saveCalled {
		t.Error("alias store should not have been saved on cancel")
	}
}

func TestProjectPicker_EditMode_OnBrowseIsNoop(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Navigate to browse option (3 projects + browse)
	m = sendKeys(m, keyDown(), keyDown(), keyDown())

	// Press e - should be no-op
	m = sendKeys(m, keyRune('e'))

	view := m.View()
	if strings.Contains(view, "Name:") {
		t.Errorf("pressing e on browse option should not enter edit mode:\n%s", view)
	}
}

func TestProjectPicker_EditMode_EnterSavesAliasAddition(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode
	m = sendKeys(m, keyRune('e'))

	// Tab to alias area - with no existing aliases, cursor goes to Add input
	m = sendKeys(m, keyTab())

	// Type a new alias name
	m = sendKeys(m, keyRune('m'), keyRune('y'), keyRune('a'))

	// Press Enter to confirm
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter in edit mode, got nil")
	}

	// Verify alias was set
	if len(aliasEditor.setCalls) != 1 {
		t.Fatalf("expected 1 Set call, got %d", len(aliasEditor.setCalls))
	}
	if aliasEditor.setCalls[0].name != "mya" {
		t.Errorf("expected alias name %q, got %q", "mya", aliasEditor.setCalls[0].name)
	}
	if aliasEditor.setCalls[0].path != "/code/newest" {
		t.Errorf("expected alias path %q, got %q", "/code/newest", aliasEditor.setCalls[0].path)
	}
	if !aliasEditor.saveCalled {
		t.Error("expected Save to be called")
	}
}

func TestProjectPicker_EditMode_EnterSavesAliasRemoval(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliases := map[string]string{
		"old": "/code/newest",
	}
	aliasEditor := newMockAliasEditor(aliases)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode
	m = sendKeys(m, keyRune('e'))

	// Tab to alias area and press x to remove the alias
	m = sendKeys(m, keyTab(), keyRune('x'))

	// Press Enter to confirm
	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter in edit mode, got nil")
	}

	if len(aliasEditor.deletions) != 1 {
		t.Fatalf("expected 1 deletion, got %d", len(aliasEditor.deletions))
	}
	if aliasEditor.deletions[0] != "old" {
		t.Errorf("expected deletion of %q, got %q", "old", aliasEditor.deletions[0])
	}
}

func TestProjectPicker_EditMode_RefreshesAfterSave(t *testing.T) {
	projects := threeProjects()
	store := &mockProjectStore{projects: projects}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode and confirm without changes (name stays same)
	m = sendKeys(m, keyRune('e'))

	_, cmd := m.Update(keyEnter())
	if cmd == nil {
		t.Fatal("expected command from Enter in edit mode, got nil")
	}

	// The command should produce a ProjectsLoadedMsg (refresh)
	msg := cmd()
	if _, ok := msg.(ui.ProjectsLoadedMsg); !ok {
		t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
	}
}

func TestProjectPicker_EditMode_MultipleAliasesDisplayed(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliases := map[string]string{
		"a1": "/code/newest",
		"a2": "/code/newest",
		"a3": "/code/newest",
	}
	aliasEditor := newMockAliasEditor(aliases)
	m := initEditModel(store, editor, aliasEditor)

	m = sendKeys(m, keyRune('e'))
	view := m.View()

	for _, name := range []string{"a1", "a2", "a3"} {
		if !strings.Contains(view, name) {
			t.Errorf("edit mode should display alias %q:\n%s", name, view)
		}
	}
}

func TestProjectPicker_EditMode_AliasCollisionShowsError(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliases := map[string]string{
		"taken": "/code/other",
	}
	aliasEditor := newMockAliasEditor(aliases)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode
	m = sendKeys(m, keyRune('e'))

	// Tab to alias area and type a name that collides
	m = sendKeys(m, keyTab())
	for _, r := range "taken" {
		m = sendKeys(m, keyRune(r))
	}

	// Press Enter - should show error
	m, _ = m.Update(keyEnter())

	view := m.View()
	if !strings.Contains(view, "already exists") {
		t.Errorf("should show collision error:\n%s", view)
	}
}

func TestProjectPicker_EditMode_EmptyNameNotSaved(t *testing.T) {
	store := &mockProjectStore{projects: threeProjects()}
	editor := &mockProjectEditor{}
	aliasEditor := newMockAliasEditor(nil)
	m := initEditModel(store, editor, aliasEditor)

	// Enter edit mode
	m = sendKeys(m, keyRune('e'))

	// Clear the name entirely
	for range len("newest") {
		m = sendKeys(m, keyBackspace())
	}

	// Press Enter - should show validation error
	m, _ = m.Update(keyEnter())

	view := m.View()
	if !strings.Contains(view, "cannot be empty") {
		t.Errorf("should show validation error for empty name:\n%s", view)
	}

	// Should still be in edit mode
	if !strings.Contains(view, "Name:") {
		t.Errorf("should still be in edit mode:\n%s", view)
	}

	// Editor should NOT have been called
	if editor.renamedPath != "" {
		t.Error("editor should not be called with empty name")
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
