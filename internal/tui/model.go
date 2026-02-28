// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/ui"
)

// page tracks which page the TUI is currently displaying.
type page int

const (
	// PageSessions displays the sessions list.
	PageSessions page = iota
	// PageProjects displays the projects list.
	PageProjects
	// pageFileBrowser displays the file browser sub-view.
	pageFileBrowser
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

// ProjectsLoadedMsg carries the result of loading projects from the store.
type ProjectsLoadedMsg struct {
	Projects []project.Project
	Err      error
}

// Model is the Bubble Tea model for the session list TUI.
type Model struct {
	sessionList     list.Model
	sessions        []tmux.Session
	selected        string
	sessionLister   SessionLister
	sessionKiller   SessionKiller
	sessionRenamer  SessionRenamer
	projectStore    ProjectStore
	sessionCreator  SessionCreator
	dirLister       DirLister
	startPath       string
	cwd             string
	activePage      page
	projectList     list.Model
	projectPicker   ui.ProjectPickerModel
	fileBrowser     ui.FileBrowserModel
	initialFilter   string
	insideTmux      bool
	currentSession  string
	modal           modalState
	pendingKillName string
	renameInput     textinput.Model
	renameTarget    string
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

// SessionListItems returns the current items in the session list, for testing.
func (m Model) SessionListItems() []list.Item {
	return m.sessionList.Items()
}

// SessionListTitle returns the session list title, for testing.
func (m Model) SessionListTitle() string {
	return m.sessionList.Title
}

// SessionListSize returns the session list dimensions, for testing.
func (m Model) SessionListSize() (int, int) {
	return m.sessionList.Width(), m.sessionList.Height()
}

// SessionListFilterState returns the current filter state of the session list, for testing.
func (m Model) SessionListFilterState() list.FilterState {
	return m.sessionList.FilterState()
}

// SessionListVisibleItems returns the visible (filtered) items in the session list, for testing.
func (m Model) SessionListVisibleItems() []list.Item {
	return m.sessionList.VisibleItems()
}

// SessionListFilterValue returns the current filter text, for testing.
func (m Model) SessionListFilterValue() string {
	return m.sessionList.FilterValue()
}

// ProjectListItems returns the current items in the project list, for testing.
func (m Model) ProjectListItems() []list.Item {
	return m.projectList.Items()
}

// ProjectListSize returns the project list dimensions, for testing.
func (m Model) ProjectListSize() (int, int) {
	return m.projectList.Width(), m.projectList.Height()
}

// ActivePage returns the currently active page, for testing.
func (m Model) ActivePage() page {
	return m.activePage
}

// WithInitialFilter returns a copy of the Model with the initial filter set.
// The filter is applied to the session list after items load.
func (m Model) WithInitialFilter(filter string) Model {
	m.initialFilter = filter
	return m
}

// WithCommand returns a copy of the Model with the given command set.
// When command is non-empty, the TUI starts in command-pending mode:
// the session list is skipped and the project picker is shown directly.
func (m Model) WithCommand(command []string) Model {
	m.command = command
	if len(command) > 0 {
		m.commandPending = true
		m.activePage = PageProjects
		if m.projectStore != nil {
			m.projectPicker = ui.NewProjectPicker(m.projectStore)
		}
	}
	return m
}

// WithInsideTmux returns a copy of the Model configured as running inside tmux
// with the given current session name. The current session is excluded from the
// session list and the list title shows the current session name.
func (m Model) WithInsideTmux(currentSession string) Model {
	m.insideTmux = true
	m.currentSession = currentSession
	// Re-filter and update list items if sessions are already populated
	filtered := m.filteredSessions()
	m.sessionList.SetItems(ToListItems(filtered))
	m.sessionList.Title = fmt.Sprintf("Sessions (current: %s)", currentSession)
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

// WithCWD sets the current working directory on the model.
// Used by the n key to create a new session in the current directory.
func WithCWD(path string) Option {
	return func(m *Model) {
		m.cwd = path
	}
}

// WithDirLister sets the directory lister and starting path for the file browser.
func WithDirLister(d DirLister, startPath string) Option {
	return func(m *Model) {
		m.dirLister = d
		m.startPath = startPath
	}
}

// sessionHelpKeys returns key.Binding entries for session-specific actions.
func sessionHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "attach")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "projects")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
	}
}

// newSessionList creates and configures a new bubbles/list.Model for sessions.
func newSessionList(items []list.Item) list.Model {
	l := list.New(items, SessionDelegate{}, 0, 0)
	l.Title = "Sessions"
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = sessionHelpKeys
	l.AdditionalFullHelpKeys = sessionHelpKeys
	l.SetStatusBarItemName("session", "sessions running")
	return l
}

// projectHelpKeys returns key.Binding entries for the projects page.
func projectHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "new session")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sessions")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
	}
}

// newProjectList creates and configures a bubbles/list.Model for projects.
func newProjectList() list.Model {
	l := list.New(nil, ProjectDelegate{}, 0, 0)
	l.Title = "Projects"
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = projectHelpKeys
	l.AdditionalFullHelpKeys = projectHelpKeys
	l.SetStatusBarItemName("project", "saved projects")
	return l
}

// New creates a Model that fetches sessions from the given SessionLister.
// Optional dependencies are configured via functional options.
func New(lister SessionLister, opts ...Option) Model {
	m := Model{
		sessionLister: lister,
		sessionList:   newSessionList(nil),
		projectList:   newProjectList(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// NewModelWithSessions creates a Model pre-populated with sessions, for testing.
func NewModelWithSessions(sessions []tmux.Session) Model {
	items := ToListItems(sessions)
	l := newSessionList(items)
	l.SetSize(80, 24)
	pl := newProjectList()
	pl.SetSize(80, 24)
	m := Model{
		sessions:    sessions,
		sessionList: l,
		projectList: pl,
	}
	return m
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

// loadProjects returns a command that cleans stale projects and loads the list.
func (m Model) loadProjects() tea.Cmd {
	if m.projectStore == nil {
		return nil
	}
	return func() tea.Msg {
		_, _ = m.projectStore.CleanStale()
		projects, err := m.projectStore.List()
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

// Init returns a command that fetches tmux sessions, or loads projects
// when in command-pending mode.
func (m Model) Init() tea.Cmd {
	if m.commandPending && m.projectStore != nil {
		return m.projectPicker.Init()
	}
	fetchSessions := func() tea.Msg {
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
	loadProjects := m.loadProjects()
	if loadProjects != nil {
		return tea.Batch(fetchSessions, loadProjects)
	}
	return fetchSessions
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward WindowSizeMsg to both page lists so they have correct dimensions
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.sessionList.SetSize(wsm.Width, wsm.Height)
		m.projectList.SetSize(wsm.Width, wsm.Height)
	}

	// Handle cross-view messages regardless of view state
	switch msg := msg.(type) {
	case ui.BackMsg:
		if m.commandPending {
			return m, tea.Quit
		}
		m.activePage = PageSessions
		return m, nil
	case ui.ProjectSelectedMsg:
		return m, m.createSession(msg.Path)
	case ui.BrowseSelectedMsg:
		m.fileBrowser = ui.NewFileBrowser(m.startPath, m.dirLister)
		m.activePage = pageFileBrowser
		return m, nil
	case ui.BrowserDirSelectedMsg:
		return m, m.createSession(msg.Path)
	case ui.BrowserCancelMsg:
		m.activePage = PageProjects
		return m, nil
	case ProjectsLoadedMsg:
		if msg.Err == nil {
			items := ProjectsToListItems(msg.Projects)
			m.projectList.SetItems(items)
		}
		return m, nil
	case SessionCreatedMsg:
		m.selected = msg.SessionName
		return m, tea.Quit
	case sessionCreateErrMsg:
		// On error, return to current page
		return m, nil
	}

	// Delegate to the active view
	switch m.activePage {
	case PageProjects:
		if m.commandPending {
			return m.updateProjectPicker(msg)
		}
		return m.updateProjectsPage(msg)
	case pageFileBrowser:
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

// selectedProjectItem returns the currently selected ProjectItem from the list, if any.
func (m Model) selectedProjectItem() (ProjectItem, bool) {
	item := m.projectList.SelectedItem()
	if item == nil {
		return ProjectItem{}, false
	}
	pi, ok := item.(ProjectItem)
	return pi, ok
}

func (m Model) updateProjectsPage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.projectList.SettingFilter() {
			break
		}
		switch {
		case msg.Type == tea.KeyEsc:
			if m.projectList.FilterState() == list.FilterApplied {
				break
			}
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "s":
			m.activePage = PageSessions
			return m, nil
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "x":
			m.activePage = PageSessions
			return m, nil
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "n":
			return m.handleNewInCWD()
		case msg.Type == tea.KeyEnter:
			return m.handleProjectEnter()
		}
	}

	var cmd tea.Cmd
	m.projectList, cmd = m.projectList.Update(msg)
	return m, cmd
}

func (m Model) handleProjectEnter() (tea.Model, tea.Cmd) {
	pi, ok := m.selectedProjectItem()
	if !ok {
		return m, nil
	}
	if m.sessionCreator == nil {
		return m, nil
	}
	return m, m.createSession(pi.Project.Path)
}

func (m Model) updateFileBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.fileBrowser.Update(msg)
	if fb, ok := updated.(ui.FileBrowserModel); ok {
		m.fileBrowser = fb
	}
	return m, cmd
}

// selectedSessionItem returns the currently selected SessionItem from the list, if any.
func (m Model) selectedSessionItem() (SessionItem, bool) {
	item := m.sessionList.SelectedItem()
	if item == nil {
		return SessionItem{}, false
	}
	si, ok := item.(SessionItem)
	return si, ok
}

func (m Model) updateSessionList(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle active modal first — route all input to modal handler
	if m.modal != modalNone {
		return m.updateModal(msg)
	}

	switch msg := msg.(type) {
	case SessionsMsg:
		if msg.Err != nil {
			return m, tea.Quit
		}
		m.sessions = msg.Sessions
		filtered := m.filteredSessions()
		items := ToListItems(filtered)
		m.sessionList.SetItems(items)

		if m.insideTmux && m.currentSession != "" {
			m.sessionList.Title = fmt.Sprintf("Sessions (current: %s)", m.currentSession)
		}

		if m.initialFilter != "" {
			m.sessionList.SetFilterText(m.initialFilter)
			m.sessionList.SetFilterState(list.FilterApplied)
			m.initialFilter = ""
		}

	case ui.ProjectsLoadedMsg:
		// Forward to project picker if we're transitioning
		updated, cmd := m.projectPicker.Update(msg)
		if picker, ok := updated.(ui.ProjectPickerModel); ok {
			m.projectPicker = picker
		}
		m.activePage = PageProjects
		return m, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// When the list is actively filtering, let it handle all key input
		if m.sessionList.SettingFilter() {
			break
		}
		switch {
		case msg.Type == tea.KeyEsc:
			// Progressive back: if filter is active, let the list clear it;
			// otherwise quit.
			if m.sessionList.FilterState() == list.FilterApplied {
				break
			}
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
			return m, tea.Quit
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "k":
			return m.handleKillKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "r":
			return m.handleRenameKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "n":
			return m.handleNewInCWD()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "p":
			m.activePage = PageProjects
			return m, nil
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "x":
			m.activePage = PageProjects
			return m, nil
		case msg.Type == tea.KeyEnter:
			return m.handleSessionListEnter()
		}
	}

	// Delegate remaining key handling to the list (cursor navigation, filtering, etc.)
	var cmd tea.Cmd
	m.sessionList, cmd = m.sessionList.Update(msg)
	return m, cmd
}

func (m Model) handleKillKey() (tea.Model, tea.Cmd) {
	si, ok := m.selectedSessionItem()
	if !ok {
		return m, nil
	}
	if m.sessionKiller == nil {
		return m, nil
	}
	m.modal = modalKillConfirm
	m.pendingKillName = si.Session.Name
	return m, nil
}

func (m Model) updateModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ctrl+C always force-quits regardless of modal state
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.modal {
	case modalKillConfirm:
		return m.updateKillConfirmModal(msg)
	case modalRename:
		return m.updateRenameModal(msg)
	default:
		return m, nil
	}
}

func (m Model) updateKillConfirmModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "y":
		name := m.pendingKillName
		m.modal = modalNone
		m.pendingKillName = ""
		return m, m.killAndRefresh(name)
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "n",
		keyMsg.Type == tea.KeyEsc:
		m.modal = modalNone
		m.pendingKillName = ""
		return m, nil
	}
	// Ignore all other keys while modal is active
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
	si, ok := m.selectedSessionItem()
	if !ok {
		return m, nil
	}
	if m.sessionRenamer == nil {
		return m, nil
	}
	m.modal = modalRename
	m.renameTarget = si.Session.Name
	ti := textinput.New()
	ti.Prompt = "New name: "
	ti.SetValue(m.renameTarget)
	ti.Focus()
	m.renameInput = ti
	return m, nil
}

func (m Model) updateRenameModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			newName := strings.TrimSpace(m.renameInput.Value())
			if newName == "" {
				return m, nil
			}
			oldName := m.renameTarget
			m.modal = modalNone
			m.renameTarget = ""
			return m, m.renameAndRefresh(oldName, newName)
		case tea.KeyEsc:
			m.modal = modalNone
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

func (m Model) handleNewInCWD() (tea.Model, tea.Cmd) {
	if m.sessionCreator == nil {
		return m, nil
	}
	return m, m.createSessionInCWD()
}

func (m Model) createSessionInCWD() tea.Cmd {
	return m.createSession(m.cwd)
}

func (m Model) handleSessionListEnter() (tea.Model, tea.Cmd) {
	si, ok := m.selectedSessionItem()
	if !ok {
		return m, nil
	}
	m.selected = si.Session.Name
	return m, tea.Quit
}

// View renders the current view.
func (m Model) View() string {
	switch m.activePage {
	case PageProjects:
		if m.commandPending {
			var b strings.Builder
			b.WriteString("Command: ")
			b.WriteString(strings.Join(m.command, " "))
			b.WriteString("\n\n")
			b.WriteString(m.projectPicker.View())
			return b.String()
		}
		return m.projectList.View()
	case pageFileBrowser:
		return m.fileBrowser.View()
	default:
		return m.viewSessionList()
	}
}

// viewSessionList renders the session list using bubbles/list.
func (m Model) viewSessionList() string {
	listView := m.sessionList.View()

	w, h := m.sessionList.Width(), m.sessionList.Height()
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	// Overlay modal on top of list when active
	switch m.modal {
	case modalKillConfirm:
		content := fmt.Sprintf("Kill %s? (y/n)", m.pendingKillName)
		return renderModal(content, listView, w, h)
	case modalRename:
		return renderModal(m.renameInput.View(), listView, w, h)
	}

	return listView
}
