// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/ui"
)

// viewState tracks which view the TUI is currently displaying.
type viewState int

const (
	viewSessionList  viewState = iota
	viewProjectPicker
)

// SessionLister defines the interface for listing tmux sessions.
type SessionLister interface {
	ListSessions() ([]tmux.Session, error)
}

// ProjectStore defines the interface for loading and cleaning projects.
type ProjectStore interface {
	List() ([]project.Project, error)
	CleanStale() (int, error)
}

// SessionCreator defines the interface for creating sessions from directories.
type SessionCreator interface {
	CreateFromDir(dir string) (string, error)
}

// SessionsMsg carries the result of fetching tmux sessions.
type SessionsMsg struct {
	Sessions []tmux.Session
	Err      error
}

// SessionCreatedMsg is emitted when a session has been successfully created.
type SessionCreatedMsg struct {
	SessionName string
}

// sessionCreateErrMsg is emitted when session creation fails.
type sessionCreateErrMsg struct {
	Err error
}

// Model is the Bubble Tea model for the session list TUI.
type Model struct {
	sessions       []tmux.Session
	cursor         int
	selected       string
	loaded         bool
	sessionLister  SessionLister
	projectStore   ProjectStore
	sessionCreator SessionCreator
	view           viewState
	projectPicker  ui.ProjectPickerModel
}

// Selected returns the name of the session chosen by the user, or empty if
// the user quit without selecting.
func (m Model) Selected() string {
	return m.selected
}

// New creates a Model that fetches sessions from the given SessionLister.
func New(lister SessionLister) Model {
	return Model{
		sessionLister: lister,
	}
}

// NewWithDeps creates a Model with all dependencies for full functionality.
func NewWithDeps(lister SessionLister, store ProjectStore, creator SessionCreator) Model {
	return Model{
		sessionLister:  lister,
		projectStore:   store,
		sessionCreator: creator,
	}
}

// NewModelWithSessions creates a Model pre-populated with sessions, for testing.
func NewModelWithSessions(sessions []tmux.Session) Model {
	return Model{
		sessions: sessions,
		cursor:   0,
		loaded:   true,
	}
}

// totalItems returns the count of navigable items in the session list view.
// This is the number of sessions plus 1 for the "new in project" option.
func (m Model) totalItems() int {
	return len(m.sessions) + 1
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
	// Handle project picker messages regardless of view state
	switch msg := msg.(type) {
	case ui.BackMsg:
		m.view = viewSessionList
		return m, nil
	case ui.ProjectSelectedMsg:
		return m, m.createSession(msg.Path)
	case SessionCreatedMsg:
		m.selected = msg.SessionName
		return m, tea.Quit
	case sessionCreateErrMsg:
		// On error, return to session list
		m.view = viewSessionList
		return m, nil
	}

	// Delegate to the active view
	switch m.view {
	case viewProjectPicker:
		return m.updateProjectPicker(msg)
	default:
		return m.updateSessionList(msg)
	}
}

func (m Model) createSession(dir string) tea.Cmd {
	return func() tea.Msg {
		name, err := m.sessionCreator.CreateFromDir(dir)
		if err != nil {
			return sessionCreateErrMsg{Err: err}
		}
		return SessionCreatedMsg{SessionName: name}
	}
}

func (m Model) updateProjectPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.projectPicker.Update(msg)
	picker, ok := updated.(ui.ProjectPickerModel)
	if ok {
		m.projectPicker = picker
	}
	return m, cmd
}

func (m Model) updateSessionList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return m, tea.Quit
		}
		m.sessions = msg.Sessions
		m.cursor = 0
		m.loaded = true

	case ui.ProjectsLoadedMsg:
		// Forward to project picker if we're transitioning
		updated, cmd := m.projectPicker.Update(msg)
		if picker, ok := updated.(ui.ProjectPickerModel); ok {
			m.projectPicker = picker
		}
		m.view = viewProjectPicker
		return m, cmd

	case tea.KeyMsg:
		switch {
		case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "n":
			// Jump to the "new in project" option
			m.cursor = len(m.sessions)
		case msg.Type == tea.KeyDown || (msg.Type == tea.KeyRunes && string(msg.Runes) == "j"):
			if m.cursor < m.totalItems()-1 {
				m.cursor++
			}
		case msg.Type == tea.KeyUp || (msg.Type == tea.KeyRunes && string(msg.Runes) == "k"):
			if m.cursor > 0 {
				m.cursor--
			}
		case msg.Type == tea.KeyEnter:
			return m.handleSessionListEnter()
		}
	}
	return m, nil
}

func (m Model) handleSessionListEnter() (tea.Model, tea.Cmd) {
	// Cursor on a session
	if m.cursor < len(m.sessions) {
		m.selected = m.sessions[m.cursor].Name
		return m, tea.Quit
	}

	// Cursor on the "new in project" option
	if m.cursor == len(m.sessions) {
		if m.projectStore != nil {
			m.projectPicker = ui.NewProjectPicker(m.projectStore)
			return m, m.projectPicker.Init()
		}
	}

	return m, nil
}

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	nameStyle     = lipgloss.NewStyle().Bold(true)
	detailStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	attachedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	dividerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// View renders the current view.
func (m Model) View() string {
	switch m.view {
	case viewProjectPicker:
		return m.projectPicker.View()
	default:
		return m.viewSessionList()
	}
}

// viewSessionList renders the session list with the "new in project" option.
func (m Model) viewSessionList() string {
	var b strings.Builder

	if m.loaded && len(m.sessions) == 0 {
		b.WriteString("No active sessions")
	} else {
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
			b.WriteString("\n")
		}
	}

	// Divider and new option
	b.WriteString("\n")
	b.WriteString(dividerStyle.Render("  ─────────────────────────────"))
	b.WriteString("\n")

	newCursor := "  "
	if m.cursor == len(m.sessions) {
		newCursor = cursorStyle.Render("> ")
	}
	fmt.Fprintf(&b, "%s[n] new in project...", newCursor)

	return b.String()
}
