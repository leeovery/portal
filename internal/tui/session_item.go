package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/tmux"
)

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	nameStyle     = lipgloss.NewStyle().Bold(true)
	detailStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	attachedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
)

// SessionItem wraps a tmux.Session and implements the list.Item interface
// for use with bubbles/list.
type SessionItem struct {
	Session tmux.Session
}

// FilterValue returns the session name for filtering.
func (i SessionItem) FilterValue() string {
	return i.Session.Name
}

// Title returns the session name for display.
func (i SessionItem) Title() string {
	return i.Session.Name
}

// Description returns the window count with correct pluralization
// and the attached badge if the session is attached.
func (i SessionItem) Description() string {
	windowLabel := fmt.Sprintf("%d windows", i.Session.Windows)
	if i.Session.Windows == 1 {
		windowLabel = "1 window"
	}

	if i.Session.Attached {
		return windowLabel + "  ● attached"
	}

	return windowLabel
}

// SessionDelegate implements list.ItemDelegate for rendering session items.
type SessionDelegate struct{}

// Height returns 1, matching the single-line item display.
func (d SessionDelegate) Height() int { return 1 }

// Spacing returns 0, no gap between items.
func (d SessionDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d SessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders a session item with cursor indicator, styled name,
// dimmed window count, and green attached badge.
func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(SessionItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("> ")
	}

	name := nameStyle.Render(si.Session.Name)

	windowLabel := fmt.Sprintf("%d windows", si.Session.Windows)
	if si.Session.Windows == 1 {
		windowLabel = "1 window"
	}
	detail := detailStyle.Render(windowLabel)

	if si.Session.Attached {
		detail += "  " + attachedStyle.Render("● attached")
	}

	line := fmt.Sprintf("%s%s  %s", cursor, name, detail)
	_, _ = fmt.Fprint(w, line)
}

// ToListItems converts a slice of tmux sessions to a slice of list.Item.
func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
