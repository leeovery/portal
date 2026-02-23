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
	CleanStale() (int, error)
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

// ProjectPickerModel is the Bubble Tea model for the project picker view.
type ProjectPickerModel struct {
	store      ProjectStore
	projects   []project.Project
	cursor     int
	loaded     bool
	filtering  bool
	filterText string
}

// NewProjectPicker creates a new ProjectPickerModel with the given store.
func NewProjectPicker(store ProjectStore) ProjectPickerModel {
	return ProjectPickerModel{
		store: store,
	}
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
