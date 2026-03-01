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
type ProjectStore interface {
	List() ([]project.Project, error)
	CleanStale() ([]project.Project, error)
	Remove(path string) error
}

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

// editField tracks which field has focus in the edit modal.
type editField int

const (
	editFieldName editField = iota
	editFieldAliases
)

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
	sessionList       list.Model
	sessions          []tmux.Session
	selected          string
	sessionLister     SessionLister
	sessionKiller     SessionKiller
	sessionRenamer    SessionRenamer
	projectStore      ProjectStore
	projectEditor     ProjectEditor
	aliasEditor       AliasEditor
	sessionCreator    SessionCreator
	dirLister         DirLister
	startPath         string
	cwd               string
	activePage        page
	projectList       list.Model
	fileBrowser       ui.FileBrowserModel
	initialFilter     string
	insideTmux        bool
	currentSession    string
	modal             modalState
	pendingKillName   string
	renameInput       textinput.Model
	renameTarget      string
	pendingDeletePath string
	pendingDeleteName string
	command           []string
	commandPending    bool

	// Data loading tracking
	sessionsLoaded       bool
	projectsLoaded       bool
	defaultPageEvaluated bool

	// Edit project modal state
	editProject     project.Project
	editName        string
	editAliases     []string
	editRemoved     []string
	editNewAlias    string
	editFocus       editField
	editAliasCursor int
	editError       string
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

// SetSessionListFilter sets the filter text and applies it on the session list, for testing.
func (m *Model) SetSessionListFilter(text string) {
	m.sessionList.SetFilterText(text)
	m.sessionList.SetFilterState(list.FilterApplied)
}

// ProjectListFilterValue returns the current filter text in the project list, for testing.
func (m Model) ProjectListFilterValue() string {
	return m.projectList.FilterValue()
}

// ProjectListItems returns the current items in the project list, for testing.
func (m Model) ProjectListItems() []list.Item {
	return m.projectList.Items()
}

// ProjectListSize returns the project list dimensions, for testing.
func (m Model) ProjectListSize() (int, int) {
	return m.projectList.Width(), m.projectList.Height()
}

// ProjectListFilterState returns the current filter state of the project list, for testing.
func (m Model) ProjectListFilterState() list.FilterState {
	return m.projectList.FilterState()
}

// ProjectListVisibleItems returns the visible (filtered) items in the project list, for testing.
func (m Model) ProjectListVisibleItems() []list.Item {
	return m.projectList.VisibleItems()
}

// SetProjectListFilter sets the filter text and applies it on the project list, for testing.
func (m *Model) SetProjectListFilter(text string) {
	m.projectList.SetFilterText(text)
	m.projectList.SetFilterState(list.FilterApplied)
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
// the session list is skipped and the projects page is shown directly.
// Help keys are updated to omit s, x, e, and d and show "run here" for enter.
func (m Model) WithCommand(command []string) Model {
	m.command = command
	if len(command) > 0 {
		m.commandPending = true
		m.activePage = PageProjects
		m.projectList.AdditionalShortHelpKeys = commandPendingHelpKeys
		m.projectList.AdditionalFullHelpKeys = commandPendingHelpKeys
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

// WithProjectEditor sets the project editor dependency for rename operations.
func WithProjectEditor(e ProjectEditor) Option {
	return func(m *Model) {
		m.projectEditor = e
	}
}

// WithAliasEditor sets the alias editor dependency for alias management.
func WithAliasEditor(a AliasEditor) Option {
	return func(m *Model) {
		m.aliasEditor = a
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
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "browse")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
	}
}

// commandPendingHelpKeys returns key.Binding entries for command-pending mode.
// Only enter (run here), n, b, /, and q are shown; s, x, e, and d are omitted.
func commandPendingHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run here")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "browse")),
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

// evaluateDefaultPage sets the active page based on loaded data.
// It only runs once both sessions and projects have been loaded.
// If sessions exist (after inside-tmux filtering), show Sessions page;
// otherwise show Projects page.
// After determining the default page, applies any initial filter to that page's list.
// In command-pending mode, always applies to the Projects page.
func (m *Model) evaluateDefaultPage() {
	if m.defaultPageEvaluated {
		return
	}
	if m.commandPending {
		if !m.projectsLoaded {
			return
		}
	} else if !m.sessionsLoaded || !m.projectsLoaded {
		return
	}
	m.defaultPageEvaluated = true
	if len(m.sessionList.Items()) > 0 {
		m.activePage = PageSessions
	} else {
		m.activePage = PageProjects
	}

	if m.initialFilter == "" {
		return
	}

	if m.activePage == PageSessions && !m.commandPending {
		m.sessionList.SetFilterText(m.initialFilter)
		m.sessionList.SetFilterState(list.FilterApplied)
	} else {
		m.projectList.SetFilterText(m.initialFilter)
		m.projectList.SetFilterState(list.FilterApplied)
	}
	m.initialFilter = ""
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

// deleteAndRefreshProjects returns a command that removes a project and reloads the list.
// Errors from Remove are propagated via ProjectsLoadedMsg.
func (m Model) deleteAndRefreshProjects(path string) tea.Cmd {
	return func() tea.Msg {
		if err := m.projectStore.Remove(path); err != nil {
			return ProjectsLoadedMsg{Err: fmt.Errorf("failed to delete project '%s': %w", path, err)}
		}
		_, _ = m.projectStore.CleanStale()
		projects, err := m.projectStore.List()
		return ProjectsLoadedMsg{Projects: projects, Err: err}
	}
}

// Init returns a command that fetches tmux sessions, or loads projects
// when in command-pending mode.
func (m Model) Init() tea.Cmd {
	if m.commandPending {
		return m.loadProjects()
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
	case ui.BrowserDirSelectedMsg:
		return m, m.createSession(msg.Path)
	case ui.BrowserCancelMsg:
		m.activePage = PageProjects
		return m, nil
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

		m.sessionsLoaded = true
		m.evaluateDefaultPage()
		return m, nil
	case ProjectsLoadedMsg:
		if msg.Err == nil {
			items := ProjectsToListItems(msg.Projects)
			m.projectList.SetItems(items)
		}
		m.projectsLoaded = true
		m.evaluateDefaultPage()
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
	// Handle active modal first — route all input to modal handler
	if m.modal != modalNone {
		return m.updateProjectModal(msg)
	}

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
			if m.commandPending {
				return m, nil
			}
			m.activePage = PageSessions
			return m, nil
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "x":
			if m.commandPending {
				return m, nil
			}
			m.activePage = PageSessions
			return m, nil
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "n":
			return m.handleNewInCWD()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "d":
			if m.commandPending {
				return m, nil
			}
			return m.handleDeleteProjectKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "e":
			if m.commandPending {
				return m, nil
			}
			return m.handleEditProjectKey()
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "b":
			return m.handleBrowseKey()
		case msg.Type == tea.KeyEnter:
			return m.handleProjectEnter()
		}
	}

	var cmd tea.Cmd
	m.projectList, cmd = m.projectList.Update(msg)
	return m, cmd
}

func (m Model) handleBrowseKey() (tea.Model, tea.Cmd) {
	if m.dirLister == nil {
		return m, nil
	}
	m.fileBrowser = ui.NewFileBrowser(m.startPath, m.dirLister)
	m.activePage = pageFileBrowser
	return m, nil
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

func (m Model) handleDeleteProjectKey() (tea.Model, tea.Cmd) {
	pi, ok := m.selectedProjectItem()
	if !ok {
		return m, nil
	}
	if m.projectStore == nil {
		return m, nil
	}
	m.modal = modalDeleteProject
	m.pendingDeletePath = pi.Project.Path
	m.pendingDeleteName = pi.Project.Name
	return m, nil
}

func (m Model) updateProjectModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ctrl+C always force-quits regardless of modal state
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}

	switch m.modal {
	case modalDeleteProject:
		return m.updateDeleteProjectModal(msg)
	case modalEditProject:
		return m.updateEditProjectModal(msg)
	default:
		return m, nil
	}
}

func (m Model) updateDeleteProjectModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "y":
		path := m.pendingDeletePath
		m.modal = modalNone
		m.pendingDeletePath = ""
		m.pendingDeleteName = ""
		return m, m.deleteAndRefreshProjects(path)
	case keyMsg.Type == tea.KeyRunes && string(keyMsg.Runes) == "n",
		keyMsg.Type == tea.KeyEsc:
		m.modal = modalNone
		m.pendingDeletePath = ""
		m.pendingDeleteName = ""
		return m, nil
	}
	// Ignore all other keys while modal is active
	return m, nil
}

func (m Model) handleEditProjectKey() (tea.Model, tea.Cmd) {
	pi, ok := m.selectedProjectItem()
	if !ok {
		return m, nil
	}
	if m.projectEditor == nil || m.aliasEditor == nil {
		return m, nil
	}

	m.modal = modalEditProject
	m.editProject = pi.Project
	m.editName = pi.Project.Name
	m.editFocus = editFieldName
	m.editNewAlias = ""
	m.editError = ""
	m.editRemoved = nil
	m.editAliasCursor = 0

	// Load aliases matching this project's directory
	allAliases, err := m.aliasEditor.Load()
	if err != nil {
		m.editAliases = nil
	} else {
		var matching []string
		for name, path := range allAliases {
			if path == pi.Project.Path {
				matching = append(matching, name)
			}
		}
		m.editAliases = matching
	}

	return m, nil
}

func (m Model) updateEditProjectModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		m.modal = modalNone
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
		return m.handleEditProjectConfirm()

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
		text := string(keyMsg.Runes)
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

func (m Model) handleEditProjectConfirm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.editName)
	if name == "" {
		m.editError = "Project name cannot be empty"
		return m, nil
	}

	// Save project name if changed
	if name != m.editProject.Name {
		if err := m.projectEditor.Rename(m.editProject.Path, name); err != nil {
			m.editError = "Failed to save project name"
			return m, nil
		}
	}

	// Handle alias removals
	for _, removed := range m.editRemoved {
		m.aliasEditor.Delete(removed)
	}

	// Handle new alias addition
	newAlias := strings.TrimSpace(m.editNewAlias)
	if newAlias != "" {
		// Check for collision
		allAliases, err := m.aliasEditor.Load()
		if err == nil {
			if existingPath, ok := allAliases[newAlias]; ok && existingPath != m.editProject.Path {
				m.editError = fmt.Sprintf("Alias '%s' already exists", newAlias)
				return m, nil
			}
		}
		m.aliasEditor.Set(newAlias, m.editProject.Path)
	}

	// Save alias changes
	if err := m.aliasEditor.Save(); err != nil {
		m.editError = "Failed to save aliases"
		return m, nil
	}

	m.modal = modalNone
	m.editError = ""

	return m, m.loadProjects()
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
			b.WriteString("Select project to run: ")
			b.WriteString(strings.Join(m.command, " "))
			b.WriteString("\n\n")
			b.WriteString(m.viewProjectList())
			return b.String()
		}
		return m.viewProjectList()
	case pageFileBrowser:
		return m.fileBrowser.View()
	default:
		return m.viewSessionList()
	}
}

// viewProjectList renders the project list, with optional modal overlay.
func (m Model) viewProjectList() string {
	listView := m.projectList.View()

	w, h := m.projectList.Width(), m.projectList.Height()
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	switch m.modal {
	case modalDeleteProject:
		content := fmt.Sprintf("Delete %s? (y/n)", m.pendingDeleteName)
		return renderModal(content, listView, w, h)
	case modalEditProject:
		return renderModal(m.renderEditProjectContent(), listView, w, h)
	}

	return listView
}

// renderEditProjectContent builds the content string for the edit project modal.
func (m Model) renderEditProjectContent() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Edit: %s\n\n", m.editProject.Name)

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
