// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/tmux"
)

// SessionLister defines the interface for listing tmux sessions.
type SessionLister interface {
	ListSessions() ([]tmux.Session, error)
}

// SessionsMsg carries the result of fetching tmux sessions.
type SessionsMsg struct {
	Sessions []tmux.Session
	Err      error
}

// Model is the Bubble Tea model for the session list TUI.
type Model struct {
	sessions      []tmux.Session
	cursor        int
	sessionLister SessionLister
}

// New creates a Model that fetches sessions from the given SessionLister.
func New(lister SessionLister) Model {
	return Model{
		sessionLister: lister,
	}
}

// NewModelWithSessions creates a Model pre-populated with sessions, for testing.
func NewModelWithSessions(sessions []tmux.Session) Model {
	return Model{
		sessions: sessions,
		cursor:   0,
	}
}

// Init returns a command that fetches tmux sessions.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return m, tea.Quit
		}
		m.sessions = msg.Sessions
		m.cursor = 0
	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyDown || (msg.Type == tea.KeyRunes && string(msg.Runes) == "j"):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case msg.Type == tea.KeyUp || (msg.Type == tea.KeyRunes && string(msg.Runes) == "k"):
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	nameStyle     = lipgloss.NewStyle().Bold(true)
	detailStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	attachedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
)

// View renders the session list.
func (m Model) View() string {
	var b strings.Builder

	for i, s := range m.sessions {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
		}

		windowLabel := fmt.Sprintf("%d windows", s.Windows)
		if s.Windows == 1 {
			windowLabel = "1 window"
		}

		detail := detailStyle.Render(windowLabel)

		if s.Attached {
			detail += "  " + attachedStyle.Render("● attached")
		}

		line := fmt.Sprintf("%s%s  %s", cursor, nameStyle.Render(s.Name), detail)
		b.WriteString(line)

		if i < len(m.sessions)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
