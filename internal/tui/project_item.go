package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/project"
)

var (
	projectNameStyle = lipgloss.NewStyle().Bold(true)
	projectPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// ProjectItem wraps a project.Project and implements the list.Item interface
// for use with bubbles/list.
type ProjectItem struct {
	Project project.Project
}

// FilterValue returns the project name for filtering.
func (i ProjectItem) FilterValue() string {
	return i.Project.Name
}

// Title returns the project name for display.
func (i ProjectItem) Title() string {
	return i.Project.Name
}

// Description returns the project path for display.
func (i ProjectItem) Description() string {
	return i.Project.Path
}

// ProjectDelegate implements list.ItemDelegate for rendering project items.
// Each item renders as two lines: bold name on line 1, dimmed path on line 2.
type ProjectDelegate struct{}

// Height returns 2, matching the two-line item display (name + path).
func (d ProjectDelegate) Height() int { return 2 }

// Spacing returns 0, no gap between items.
func (d ProjectDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d ProjectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders a project item with cursor indicator, bold name on the first
// line, and dimmed path on the second line.
func (d ProjectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(ProjectItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("> ")
	}

	name := projectNameStyle.Render(pi.Project.Name)
	path := projectPathStyle.Render(pi.Project.Path)

	line1 := fmt.Sprintf("%s%s", cursor, name)
	line2 := fmt.Sprintf("  %s", path)

	_, _ = fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// ProjectsToListItems converts a slice of projects to a slice of list.Item.
func ProjectsToListItems(projects []project.Project) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = ProjectItem{Project: p}
	}
	return items
}
