// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/fuzzy"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/ui"
)

// viewState tracks which view the TUI is currently displaying.
type viewState int

const (
	viewSessionList  viewState = iota
	viewProjectPicker
	viewFileBrowser
)

// SessionLister defines the interface for listing tmux sessions.
type SessionLister interface {
	ListSessions() ([]tmux.Session, error)
}

// ProjectStore abstracts project storage for testability.
type ProjectStore = ui.ProjectStore

// SessionKiller defines the interface for killing tmux sessions.
type SessionKiller interface {
	KillSession(name string) error
}

// SessionCreator defines the interface for creating sessions from directories.
type SessionCreator interface {
	CreateFromDir(dir string, command []string) (string, error)
}

// SessionRenamer defines the interface for renaming tmux sessions.
type SessionRenamer interface {
	RenameSession(oldName, newName string) error
}

// DirLister abstracts directory listing for testability.
type DirLister = ui.DirLister

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
	sessions        []tmux.Session
	cursor          int
	selected        string
	loaded          bool
	sessionLister   SessionLister
	sessionKiller   SessionKiller
	sessionRenamer  SessionRenamer
	projectStore    ProjectStore
	sessionCreator  SessionCreator
	dirLister       DirLister
	startPath       string
	view            viewState
	projectPicker   ui.ProjectPickerModel
	fileBrowser     ui.FileBrowserModel
	initialFilter   string
	insideTmux      bool
	currentSession  string
	confirmKill     bool
	pendingKillName string
	renameMode      bool
	renameInput     textinput.Model
	renameTarget    string
	filterMode      bool
	filterText      string
	command         []string
	commandPending  bool
}

// Selected returns the name of the session chosen by the user, or empty if
// the user quit without selecting.
func (m Model) Selected() string {
	return m.selected
}

// InitialFilter returns the initial filter text for the session list.
func (m Model) InitialFilter() string {
	return m.initialFilter
}

// WithInitialFilter returns a copy of the Model with the initial filter set.
// When in command-pending mode, the filter is applied to the project picker.
func (m Model) WithInitialFilter(filter string) Model {
	m.initialFilter = filter
	if m.commandPending && filter != "" {
		m.projectPicker = m.projectPicker.WithFilter(filter)
	}
	return m
}

// WithCommand returns a copy of the Model with the given command set.
// When command is non-empty, the TUI starts in command-pending mode:
// the session list is skipped and the project picker is shown directly.
func (m Model) WithCommand(command []string) Model {
	m.command = command
	if len(command) > 0 {
		m.commandPending = true
		m.view = viewProjectPicker
		if m.projectStore != nil {
			m.projectPicker = ui.NewProjectPicker(m.projectStore)
		}
	}
	return m
}

// WithInsideTmux returns a copy of the Model configured as running inside tmux
// with the given current session name. The current session is excluded from the
// session list and a header showing the current session name is rendered.
func (m Model) WithInsideTmux(currentSession string) Model {
	m.insideTmux = true
	m.currentSession = currentSession
	m.sessions = m.filteredSessions()
	return m
}

// Option configures an optional dependency on Model.
type Option func(*Model)

// WithKiller sets the session killer dependency.
func WithKiller(k SessionKiller) Option {
	return func(m *Model) {
		m.sessionKiller = k
	}
}

// WithRenamer sets the session renamer dependency.
func WithRenamer(r SessionRenamer) Option {
	return func(m *Model) {
		m.sessionRenamer = r
	}
}

// WithProjectStore sets the project store dependency.
func WithProjectStore(s ProjectStore) Option {
	return func(m *Model) {
		m.projectStore = s
	}
}

// WithSessionCreator sets the session creator dependency.
func WithSessionCreator(c SessionCreator) Option {
	return func(m *Model) {
		m.sessionCreator = c
	}
}

// WithDirLister sets the directory lister and starting path for the file browser.
func WithDirLister(d DirLister, startPath string) Option {
	return func(m *Model) {
		m.dirLister = d
		m.startPath = startPath
	}
}

// New creates a Model that fetches sessions from the given SessionLister.
// Optional dependencies are configured via functional options.
func New(lister SessionLister, opts ...Option) Model {
	m := Model{
		sessionLister: lister,
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// NewModelWithSessions creates a Model pre-populated with sessions, for testing.
func NewModelWithSessions(sessions []tmux.Session) Model {
	return Model{
		sessions: sessions,
		cursor:   0,
		loaded:   true,
	}
}

// filteredSessions returns sessions with the current session excluded when inside tmux.
func (m Model) filteredSessions() []tmux.Session {
	if !m.insideTmux || m.currentSession == "" {
		return m.sessions
	}
	filtered := make([]tmux.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if s.Name != m.currentSession {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// totalItems returns the count of navigable items in the session list view.
// This is the number of sessions plus 1 for the "new in project" option.
func (m Model) totalItems() int {
	return len(m.sessions) + 1
}

// Init returns a command that fetches tmux sessions, or loads projects
// when in command-pending mode.
func (m Model) Init() tea.Cmd {
	if m.commandPending && m.projectStore != nil {
		return m.projectPicker.Init()
	}
	return func() tea.Msg {
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle cross-view messages regardless of view state
	switch msg := msg.(type) {
	case ui.BackMsg:
		if m.commandPending {
			return m, tea.Quit
		}
		m.view = viewSessionList
		return m, nil
	case ui.ProjectSelectedMsg:
		return m, m.createSession(msg.Path)
	case ui.BrowseSelectedMsg:
		m.fileBrowser = ui.NewFileBrowser(m.startPath, m.dirLister)
		m.view = viewFileBrowser
		return m, nil
	case ui.BrowserDirSelectedMsg:
		return m, m.createSession(msg.Path)
	case ui.BrowserCancelMsg:
		m.view = viewProjectPicker
		return m, nil
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
	case viewFileBrowser:
		return m.updateFileBrowser(msg)
	default:
		return m.updateSessionList(msg)
	}
}

func (m Model) createSession(dir string) tea.Cmd {
	return func() tea.Msg {
		name, err := m.sessionCreator.CreateFromDir(dir, m.command)
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

func (m Model) updateFileBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.fileBrowser.Update(msg)
	if fb, ok := updated.(ui.FileBrowserModel); ok {
		m.fileBrowser = fb
	}
	return m, cmd
}

func (m Model) updateSessionList(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle confirmKill state first
	if m.confirmKill {
		return m.updateConfirmKill(msg)
	}

	// Handle rename mode
	if m.renameMode {
		return m.updateRename(msg)
	}

	// Handle filter mode
	if m.filterMode {
		return m.updateFilter(msg)
	}

	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return m, tea.Quit
		}
		m.sessions = msg.Sessions
		m.sessions = m.filteredSessions()
		if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
			m.cursor = len(m.sessions) - 1
		} else if len(m.sessions) == 0 {
			m.cursor = 0
		}
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
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "K":
			return m.handleKillKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "R":
			return m.handleRenameKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "/":
			m.filterMode = true
			m.filterText = ""
			m.cursor = 0
			return m, nil
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

func (m Model) handleKillKey() (tea.Model, tea.Cmd) {
	// No-op if cursor is on the [n] new in project option
	if m.cursor >= len(m.sessions) {
		return m, nil
	}
	// No-op if no session killer configured
	if m.sessionKiller == nil {
		return m, nil
	}
	m.confirmKill = true
	m.pendingKillName = m.sessions[m.cursor].Name
	return m, nil
}

func (m Model) updateConfirmKill(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "y":
		name := m.pendingKillName
		m.confirmKill = false
		m.pendingKillName = ""
		return m, m.killAndRefresh(name)
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "n",
		keyMsg.Type == tea.KeyEsc:
		m.confirmKill = false
		m.pendingKillName = ""
		return m, nil
	}
	// Ignore all other keys in confirmation mode
	return m, nil
}

func (m Model) killAndRefresh(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.sessionKiller.KillSession(name); err != nil {
			return SessionsMsg{Err: fmt.Errorf("failed to kill session '%s': %w", name, err)}
		}
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

func (m Model) handleRenameKey() (tea.Model, tea.Cmd) {
	// No-op if cursor is on the [n] new in project option
	if m.cursor >= len(m.sessions) {
		return m, nil
	}
	// No-op if no session renamer configured
	if m.sessionRenamer == nil {
		return m, nil
	}
	m.renameMode = true
	m.renameTarget = m.sessions[m.cursor].Name
	ti := textinput.New()
	ti.Prompt = "Rename: "
	ti.SetValue(m.renameTarget)
	ti.Focus()
	m.renameInput = ti
	return m, nil
}

func (m Model) updateRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			newName := strings.TrimSpace(m.renameInput.Value())
			if newName == "" {
				return m, nil
			}
			oldName := m.renameTarget
			m.renameMode = false
			m.renameTarget = ""
			return m, m.renameAndRefresh(oldName, newName)
		case tea.KeyEsc:
			m.renameMode = false
			m.renameTarget = ""
			return m, nil
		}
	}

	// Delegate to textinput for all other messages
	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

func (m Model) renameAndRefresh(oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		if err := m.sessionRenamer.RenameSession(oldName, newName); err != nil {
			return SessionsMsg{Err: fmt.Errorf("failed to rename session '%s': %w", oldName, err)}
		}
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		m.filterMode = false
		m.filterText = ""
		m.cursor = 0
		return m, nil
	case tea.KeyBackspace:
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.cursor = 0
		} else {
			m.filterMode = false
			m.filterText = ""
			m.cursor = 0
		}
		return m, nil
	case tea.KeyEnter:
		matched := m.filterMatchedSessions()
		if m.cursor < len(matched) {
			m.selected = matched[m.cursor].Name
			return m, tea.Quit
		}
		// Cursor on the [n] new in project option
		if m.cursor == len(matched) {
			if m.projectStore != nil {
				m.filterMode = false
				m.filterText = ""
				m.projectPicker = ui.NewProjectPicker(m.projectStore)
				return m, m.projectPicker.Init()
			}
		}
		return m, nil
	case tea.KeyDown:
		matched := m.filterMatchedSessions()
		// +1 for the [n] new in project option
		if m.cursor < len(matched) {
			m.cursor++
		}
		return m, nil
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case tea.KeyRunes:
		m.filterText += string(keyMsg.Runes)
		m.cursor = 0
		return m, nil
	}

	return m, nil
}

// filterMatchedSessions returns sessions whose names fuzzy-match the current filter text.
// Uses subsequence matching: each character in the filter must appear in order in the session name.
func (m Model) filterMatchedSessions() []tmux.Session {
	return fuzzy.Filter(m.sessions, m.filterText, func(s tmux.Session) string { return s.Name })
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
		var b strings.Builder
		if m.commandPending {
			b.WriteString("Command: ")
			b.WriteString(strings.Join(m.command, " "))
			b.WriteString("\n\n")
		}
		b.WriteString(m.projectPicker.View())
		return b.String()
	case viewFileBrowser:
		return m.fileBrowser.View()
	default:
		return m.viewSessionList()
	}
}

// displaySessions returns the sessions to display, applying filter when in filter mode.
func (m Model) displaySessions() []tmux.Session {
	if m.filterMode {
		return m.filterMatchedSessions()
	}
	return m.sessions
}

// viewSessionList renders the session list with the "new in project" option.
func (m Model) viewSessionList() string {
	var b strings.Builder

	if m.insideTmux && m.currentSession != "" {
		b.WriteString("Current: " + m.currentSession)
		b.WriteString("\n\n")
	}

	visible := m.displaySessions()

	if m.loaded && len(visible) == 0 && !m.filterMode {
		if m.insideTmux {
			b.WriteString("No other sessions")
		} else {
			b.WriteString("No active sessions")
		}
	} else {
		for i, s := range visible {
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
	if m.cursor == len(visible) {
		newCursor = cursorStyle.Render("> ")
	}
	fmt.Fprintf(&b, "%s[n] new in project...", newCursor)

	if m.confirmKill {
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "Kill session '%s'? (y/n)", m.pendingKillName)
	}

	if m.renameMode {
		b.WriteString("\n\n")
		b.WriteString(m.renameInput.View())
	}

	if m.filterMode {
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "filter: %s", m.filterText)
	}

	return b.String()
}
