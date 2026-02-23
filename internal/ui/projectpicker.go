// Package ui provides Bubble Tea TUI components for Portal.
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
)

// ProjectStore defines the interface for loading and cleaning projects.
type ProjectStore interface {
	List() ([]project.Project, error)
	CleanStale() ([]project.Project, error)
}

// ProjectEditor defines the interface for renaming projects.
type ProjectEditor interface {
	Rename(path, newName string) error
}

// AliasEditor defines the interface for managing aliases in edit mode.
type AliasEditor interface {
	Load() (map[string]string, error)
	Set(name, path string)
	Delete(name string) bool
	Save() error
}

// ProjectsLoadedMsg carries the result of loading projects from the store.
type ProjectsLoadedMsg struct {
	Projects []project.Project
	Err      error
}

// ProjectSelectedMsg is emitted when the user selects a project.
type ProjectSelectedMsg struct {
	Path string
}

// BrowseSelectedMsg is emitted when the user selects the browse option.
type BrowseSelectedMsg struct{}

// BackMsg is emitted when the user presses Esc to return to the session list.
type BackMsg struct{}

// editField tracks which field has focus in edit mode.
type editField int

const (
	editFieldName editField = iota
	editFieldAliases
)

// ProjectPickerModel is the Bubble Tea model for the project picker view.
type ProjectPickerModel struct {
	store      ProjectStore
	editor     ProjectEditor
	aliasStore AliasEditor
	projects   []project.Project
	cursor     int
	loaded     bool
	filtering  bool
	filterText string

	// Edit mode state
	editMode        bool
	editProject     project.Project
	editName        string
	editAliases     []string // current alias names for the project's directory
	editRemoved     []string // alias names removed during this edit session
	editNewAlias    string   // text input for adding a new alias
	editFocus       editField
	editAliasCursor int
	editError       string
}

// NewProjectPicker creates a new ProjectPickerModel with the given store.
func NewProjectPicker(store ProjectStore) ProjectPickerModel {
	return ProjectPickerModel{
		store: store,
	}
}

// WithEditor returns a copy of the ProjectPickerModel with edit mode support.
func (m ProjectPickerModel) WithEditor(editor ProjectEditor, aliasStore AliasEditor) ProjectPickerModel {
	m.editor = editor
	m.aliasStore = aliasStore
	return m
}

// WithFilter returns a copy of the ProjectPickerModel with the filter pre-filled.
// The picker starts in filtering mode with the given text.
func (m ProjectPickerModel) WithFilter(text string) ProjectPickerModel {
	if text != "" {
		m.filtering = true
		m.filterText = text
	}
	return m
}

// Init calls CleanStale and then loads projects from the store.
func (m ProjectPickerModel) Init() tea.Cmd {
	return func() tea.Msg {
		_, _ = m.store.CleanStale()
		projects, err := m.store.List()
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

// filteredProjects returns the projects that match the current filter text.
func (m ProjectPickerModel) filteredProjects() []project.Project {
	if m.filterText == "" {
		return m.projects
	}
	var filtered []project.Project
	lowerFilter := strings.ToLower(m.filterText)
	for _, p := range m.projects {
		if fuzzyMatch(strings.ToLower(p.Name), lowerFilter) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// fuzzyMatch checks if the pattern characters appear in order within the text.
func fuzzyMatch(text, pattern string) bool {
	pi := 0
	for ti := 0; ti < len(text) && pi < len(pattern); ti++ {
		if text[ti] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// totalItems returns the count of visible items (filtered projects + browse option).
func (m ProjectPickerModel) totalItems() int {
	return len(m.filteredProjects()) + 1 // +1 for browse option
}

// Update handles messages and key input for the project picker.
func (m ProjectPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProjectsLoadedMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.projects = msg.Projects
		m.loaded = true
		m.cursor = 0

	case tea.KeyMsg:
		if m.editMode {
			return m.updateEditMode(msg)
		}
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m ProjectPickerModel) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC:
		return m, func() tea.Msg { return BackMsg{} }

	case msg.Type == tea.KeyDown || (msg.Type == tea.KeyRunes && string(msg.Runes) == "j"):
		if m.cursor < m.totalItems()-1 {
			m.cursor++
		}

	case msg.Type == tea.KeyUp || (msg.Type == tea.KeyRunes && string(msg.Runes) == "k"):
		if m.cursor > 0 {
			m.cursor--
		}

	case msg.Type == tea.KeyRunes && string(msg.Runes) == "e":
		return m.handleEditKey()

	case msg.Type == tea.KeyRunes && string(msg.Runes) == "/":
		m.filtering = true
		m.filterText = ""

	case msg.Type == tea.KeyEnter:
		return m.handleEnter()
	}
	return m, nil
}

func (m ProjectPickerModel) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filterText = ""
		m.cursor = 0

	case tea.KeyBackspace:
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			// Clamp cursor to valid range after filter change
			if m.cursor >= m.totalItems() {
				m.cursor = m.totalItems() - 1
			}
		} else {
			// Empty filter + backspace: exit filter mode
			m.filtering = false
		}

	case tea.KeyEnter:
		return m.handleEnter()

	case tea.KeyDown:
		if m.cursor < m.totalItems()-1 {
			m.cursor++
		}

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}

	case tea.KeyRunes:
		m.filterText += string(msg.Runes)
		m.cursor = 0 // Reset cursor when filter changes
	}
	return m, nil
}

func (m ProjectPickerModel) handleEditKey() (tea.Model, tea.Cmd) {
	filtered := m.filteredProjects()
	// No-op on browse option or if no editor configured
	if m.cursor >= len(filtered) || m.editor == nil || m.aliasStore == nil {
		return m, nil
	}

	p := filtered[m.cursor]
	m.editMode = true
	m.editProject = p
	m.editName = p.Name
	m.editFocus = editFieldName
	m.editNewAlias = ""
	m.editError = ""
	m.editRemoved = nil
	m.editAliasCursor = 0

	// Load aliases matching this project's directory
	allAliases, err := m.aliasStore.Load()
	if err != nil {
		m.editAliases = nil
	} else {
		var matching []string
		for name, path := range allAliases {
			if path == p.Path {
				matching = append(matching, name)
			}
		}
		m.editAliases = matching
	}

	return m, nil
}

func (m ProjectPickerModel) updateEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.editMode = false
		m.editError = ""
		return m, nil

	case tea.KeyTab:
		if m.editFocus == editFieldName {
			m.editFocus = editFieldAliases
		} else {
			m.editFocus = editFieldName
		}
		return m, nil

	case tea.KeyEnter:
		return m.handleEditConfirm()

	case tea.KeyBackspace:
		if m.editFocus == editFieldName {
			if len(m.editName) > 0 {
				m.editName = m.editName[:len(m.editName)-1]
			}
		} else if m.editAliasCursor == len(m.editAliases) {
			// On Add input
			if len(m.editNewAlias) > 0 {
				m.editNewAlias = m.editNewAlias[:len(m.editNewAlias)-1]
			}
		}
		return m, nil

	case tea.KeyDown:
		if m.editFocus == editFieldAliases && m.editAliasCursor < len(m.editAliases) {
			m.editAliasCursor++
		}
		return m, nil

	case tea.KeyUp:
		if m.editFocus == editFieldAliases && m.editAliasCursor > 0 {
			m.editAliasCursor--
		}
		return m, nil

	case tea.KeyRunes:
		text := string(msg.Runes)
		// In alias area, on an existing alias entry: x removes it
		if m.editFocus == editFieldAliases && text == "x" && m.editAliasCursor < len(m.editAliases) {
			removed := m.editAliases[m.editAliasCursor]
			m.editRemoved = append(m.editRemoved, removed)
			m.editAliases = append(m.editAliases[:m.editAliasCursor], m.editAliases[m.editAliasCursor+1:]...)
			if m.editAliasCursor > len(m.editAliases) {
				m.editAliasCursor = len(m.editAliases)
			}
			return m, nil
		}
		// In alias area, on Add input: type into new alias
		if m.editFocus == editFieldAliases && m.editAliasCursor == len(m.editAliases) {
			m.editNewAlias += text
			m.editError = ""
			return m, nil
		}
		if m.editFocus == editFieldName {
			m.editName += text
		}
		m.editError = ""
		return m, nil
	}
	return m, nil
}

func (m ProjectPickerModel) handleEditConfirm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.editName)
	if name == "" {
		m.editError = "Project name cannot be empty"
		return m, nil
	}

	// Save project name if changed
	if name != m.editProject.Name {
		if err := m.editor.Rename(m.editProject.Path, name); err != nil {
			m.editError = "Failed to save project name"
			return m, nil
		}
	}

	// Handle alias removals
	for _, removed := range m.editRemoved {
		m.aliasStore.Delete(removed)
	}

	// Handle new alias addition
	newAlias := strings.TrimSpace(m.editNewAlias)
	if newAlias != "" {
		// Check for collision
		allAliases, err := m.aliasStore.Load()
		if err == nil {
			if existingPath, ok := allAliases[newAlias]; ok && existingPath != m.editProject.Path {
				m.editError = fmt.Sprintf("Alias '%s' already exists", newAlias)
				return m, nil
			}
		}
		m.aliasStore.Set(newAlias, m.editProject.Path)
	}

	// Save alias changes
	if err := m.aliasStore.Save(); err != nil {
		m.editError = "Failed to save aliases"
		return m, nil
	}

	m.editMode = false
	m.editError = ""

	// Refresh project list
	return m, func() tea.Msg {
		_, _ = m.store.CleanStale()
		projects, err := m.store.List()
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

func (m ProjectPickerModel) handleEnter() (tea.Model, tea.Cmd) {
	filtered := m.filteredProjects()
	if m.cursor < len(filtered) {
		path := filtered[m.cursor].Path
		return m, func() tea.Msg { return ProjectSelectedMsg{Path: path} }
	}
	// Browse option
	return m, func() tea.Msg { return BrowseSelectedMsg{} }
}

// View renders the project picker.
func (m ProjectPickerModel) View() string {
	if !m.loaded {
		return ""
	}

	if m.editMode {
		return m.viewEditMode()
	}

	var b strings.Builder

	b.WriteString("Select a project:\n\n")

	filtered := m.filteredProjects()

	if len(m.projects) == 0 && !m.filtering {
		b.WriteString("  No saved projects yet.\n")
	} else if len(filtered) == 0 && m.filtering {
		b.WriteString("  No matches.\n")
	} else {
		for i, p := range filtered {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			fmt.Fprintf(&b, "%s%s\n", cursor, p.Name)
		}
	}

	b.WriteString("\n  ─────────────────────────────\n")

	browseCursor := "  "
	if m.cursor == len(filtered) {
		browseCursor = "> "
	}
	fmt.Fprintf(&b, "%sbrowse for directory...\n", browseCursor)

	if m.filtering {
		fmt.Fprintf(&b, "\nfilter: %s", m.filterText)
	}

	return b.String()
}

func (m ProjectPickerModel) viewEditMode() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Edit project: %s\n\n", m.editProject.Name)

	nameIndicator := "  "
	if m.editFocus == editFieldName {
		nameIndicator = "> "
	}
	fmt.Fprintf(&b, "%sName: %s\n", nameIndicator, m.editName)

	b.WriteString("\n")

	aliasIndicator := "  "
	if m.editFocus == editFieldAliases {
		aliasIndicator = "> "
	}
	b.WriteString(aliasIndicator + "Aliases:\n")

	if len(m.editAliases) == 0 {
		b.WriteString("    (none)\n")
	} else {
		for i, a := range m.editAliases {
			marker := "    "
			if m.editFocus == editFieldAliases && m.editAliasCursor == i {
				marker = "  > "
			}
			fmt.Fprintf(&b, "%s[x] %s\n", marker, a)
		}
	}

	addMarker := "    "
	if m.editFocus == editFieldAliases && m.editAliasCursor == len(m.editAliases) {
		addMarker = "  > "
	}
	fmt.Fprintf(&b, "%sAdd: %s\n", addMarker, m.editNewAlias)

	if m.editError != "" {
		fmt.Fprintf(&b, "\n  Error: %s\n", m.editError)
	}

	b.WriteString("\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field")

	return b.String()
}
