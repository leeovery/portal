// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/ui"
)

// page tracks which page the TUI is currently displaying.
type page int

const (
	// PageLoading displays the loading interstitial during bootstrap.
	PageLoading page = iota
	// PageSessions displays the sessions list.
	PageSessions
	// PageProjects displays the projects list.
	PageProjects
	// pageFileBrowser displays the file browser sub-view.
	pageFileBrowser
	// pagePreview displays the session scrollback preview sub-view.
	pagePreview
)

// SessionLister defines the interface for listing tmux sessions.
type SessionLister interface {
	ListSessions() ([]tmux.Session, error)
}

// PreviewAttacher is the exported seam through which the preview page's
// Enter handler dispatches the pre-select pipeline (spec § Pre-select +
// attach sequence). Run returns a tea.Cmd that executes the three
// pre-select calls (has-session probe, select-window, select-pane) and
// emits a previewAttachSelectedMsg or previewAttachBailMsg; the connector
// handoff is post-TUI in cmd/open.go's processTUIResult. Production
// wiring is the previewAttachPipeline constructed via
// NewPreviewAttachPipeline in cmd/open.go; tests that do not exercise
// Enter pass nil.
type PreviewAttacher interface {
	Run(session string, window, pane int) tea.Cmd
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

// LoadingMinDuration is the minimum amount of time the loading page is
// displayed before being dismissed. Bootstrap may take longer (the page
// stays until bootstrap completes); if bootstrap is faster, the page
// is padded to this duration so it does not flash as a UI glitch.
const LoadingMinDuration = 1200 * time.Millisecond

// LoadingMinElapsedMsg signals that LoadingMinDuration has elapsed since
// the loading page was first shown.
type LoadingMinElapsedMsg struct{}

// BootstrapCompleteMsg signals that the bootstrap orchestrator has finished
// running. The TUI dismisses the loading page once both LoadingMinElapsedMsg
// and BootstrapCompleteMsg have been received.
//
// Warnings carries soft bootstrap warnings drained from the
// cmd-side BootstrapWarningsSink. The model buffers them through the
// loading window and flushes them to stderr (with alt-screen toggle)
// after the loading page dismisses; this preserves the rendered TUI
// while still surfacing diagnostics the user must see. May be nil/empty
// when bootstrap produced no warnings.
type BootstrapCompleteMsg struct {
	Warnings []BootstrapWarning
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

	// Bootstrap loading state
	serverStarted     bool
	minElapsed        bool
	bootstrapComplete bool

	// Bootstrap warnings: pending is set by openTUI before tea.NewProgram
	// runs Init; Init folds it into the BootstrapCompleteMsg payload so
	// Update can buffer it through the loading window. bufferedWarnings
	// holds the slice between BootstrapCompleteMsg and dismissal of the
	// loading page; flushBufferedWarningsCmd clears it on emit.
	pendingBootstrapWarnings []BootstrapWarning
	bufferedWarnings         []BootstrapWarning

	// Terminal dimensions (cached for re-applying after data loads)
	termWidth, termHeight int

	// Preview page seams and live model. enumerator and reader are
	// constructor-injected at TUI startup (wired in task 2-7) — declared
	// here so the page state machine can compile against the preview
	// arm. preview holds the live previewModel between pagePreview entry
	// (via Space on PageSessions) and dismissal back to PageSessions.
	// previewAttacher is the Enter pre-select + attach pipeline, wired
	// via WithPreviewAttachPipeline and propagated onto previewModel at
	// Space-handler construction so the preview page's Enter binding
	// dispatches without re-resolving the connector.
	enumerator      TmuxEnumerator
	reader          ScrollbackReader
	previewAttacher PreviewAttacher
	preview         previewModel

	// Sessions-page inline-flash state (spec § Inline flash —
	// feature-local infrastructure). flashText empty string means no
	// flash is active; non-empty is rendered between the filter input
	// and the Sessions list. flashGen is a monotonic counter bumped on
	// every setFlash call; tick handlers capture the generation at
	// schedule time and compare against the live value on fire so a
	// stale tick from a replaced flash cannot early-clear the
	// currently-displayed message (spec § Replacement on rapid
	// successive bails).
	flashText string
	flashGen  uint64

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

// ServerStarted returns whether the model was configured with server bootstrap, for testing.
func (m Model) ServerStarted() bool {
	return m.serverStarted
}

// MinElapsed reports whether LoadingMinElapsedMsg has been received, for testing.
func (m Model) MinElapsed() bool {
	return m.minElapsed
}

// BootstrapComplete reports whether BootstrapCompleteMsg has been received, for testing.
func (m Model) BootstrapComplete() bool {
	return m.bootstrapComplete
}

// BufferedWarnings returns the warnings buffered between
// BootstrapCompleteMsg and loading-page dismissal, for testing.
// The slice is cleared after flushBufferedWarningsCmd runs.
func (m Model) BufferedWarnings() []BootstrapWarning {
	return m.bufferedWarnings
}

// PendingBootstrapWarnings returns the warnings staged via
// SetPendingBootstrapWarnings, for testing.
func (m Model) PendingBootstrapWarnings() []BootstrapWarning {
	return m.pendingBootstrapWarnings
}

// SetPendingBootstrapWarnings stores warnings to be folded into the
// BootstrapCompleteMsg emitted from Init's first event-loop tick.
// openTUI calls this with the result of bootstrapWarnings.Drain
// (converted to []BootstrapWarning) before tea.NewProgram.Run, so the
// warnings ride the same gate that dismisses the loading page.
func (m *Model) SetPendingBootstrapWarnings(warnings []BootstrapWarning) {
	m.pendingBootstrapWarnings = warnings
}

// CommandPending returns whether the model is in command-pending mode, for testing.
func (m Model) CommandPending() bool {
	return m.commandPending
}

// Command returns the command slice, for testing.
func (m Model) Command() []string {
	return m.command
}

// InsideTmux returns whether the model is configured as running inside tmux, for testing.
func (m Model) InsideTmux() bool {
	return m.insideTmux
}

// CurrentSession returns the current tmux session name, for testing.
func (m Model) CurrentSession() string {
	return m.currentSession
}

// CWD returns the current working directory, for testing.
func (m Model) CWD() string {
	return m.cwd
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
// The manual keymap footer (see projectFooterBindings) switches to
// commandPendingHelpKeys when m.commandPending is true; no list-level
// AdditionalShortHelpKeys/AdditionalFullHelpKeys are assigned because
// the bubbles/list help renderer is disabled via SetShowHelp(false).
func (m Model) WithCommand(command []string) Model {
	m.command = command
	if len(command) > 0 {
		m.commandPending = true
		m.activePage = PageProjects
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
	if cmd := m.sessionList.SetItems(ToListItems(filtered)); cmd != nil {
		panic("unreachable: WithInsideTmux runs before any filter can be applied")
	}
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

// WithServerStarted configures the model to start on the loading page
// when the tmux server was just started by bootstrap.
func WithServerStarted(started bool) Option {
	return func(m *Model) {
		m.serverStarted = started
		if started {
			m.activePage = PageLoading
		}
	}
}

// WithDirLister sets the directory lister and starting path for the file browser.
func WithDirLister(d DirLister, startPath string) Option {
	return func(m *Model) {
		m.dirLister = d
		m.startPath = startPath
	}
}

// WithEnumerator wires the TmuxEnumerator seam used by the scrollback
// preview page. Production callers pass a *tmux.Client; tests that do not
// exercise preview entry can omit this option, leaving enumerator nil.
func WithEnumerator(e TmuxEnumerator) Option {
	return func(m *Model) {
		m.enumerator = e
	}
}

// WithScrollbackReader wires the ScrollbackReader seam used by the
// scrollback preview page. Production callers pass a
// scrollbackReaderAdapter constructed once at TUI startup with stateDir
// resolved via internal/state's paths helper; tests that do not exercise
// preview entry can omit this option, leaving reader nil.
func WithScrollbackReader(r ScrollbackReader) Option {
	return func(m *Model) {
		m.reader = r
	}
}

// WithPreviewAttachPipeline wires the PreviewAttacher seam used by the
// preview page's Enter binding. Production callers pass the pipeline
// constructed via NewPreviewAttachPipeline (closing over *tmux.Client +
// a nullable *state.Logger); tests that do not exercise Enter can omit
// this option, leaving previewAttacher nil.
func WithPreviewAttachPipeline(p PreviewAttacher) Option {
	return func(m *Model) {
		m.previewAttacher = p
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
		key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "preview")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// brightenHelpStyles makes the help bar text lighter so it's easier to read.
func brightenHelpStyles(l *list.Model) {
	keyColor := lipgloss.Color("#999999")
	descColor := lipgloss.Color("#777777")
	sepColor := lipgloss.Color("#555555")
	l.Help.Styles.ShortKey = lipgloss.NewStyle().Foreground(keyColor)
	l.Help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(descColor)
	l.Help.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(sepColor)
	l.Help.Styles.FullKey = lipgloss.NewStyle().Foreground(keyColor)
	l.Help.Styles.FullDesc = lipgloss.NewStyle().Foreground(descColor)
	l.Help.Styles.FullSeparator = lipgloss.NewStyle().Foreground(sepColor)
	l.Help.Styles.Ellipsis = lipgloss.NewStyle().Foreground(sepColor)
}

// newSessionList creates and configures a new bubbles/list.Model for sessions.
// The bubbles/list built-in help renderer is disabled via SetShowHelp(false)
// because the sessions/projects keymap footer is rendered manually as three
// fixed columns by viewSessionList / viewProjectList via the list's own
// help.Model.FullHelpView (see keymapFooter helpers below). brightenHelpStyles
// still populates l.Help.Styles.* so the manual render keeps the same colour
// and separator palette as the previous bubbles/list-driven bar.
func newSessionList(items []list.Item) list.Model {
	l := list.New(items, SessionDelegate{}, 0, 0)
	l.Title = "Sessions"
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName("session", "sessions running")
	l.SetShowHelp(false)
	l.KeyMap.ShowFullHelp.Unbind()
	l.KeyMap.CloseFullHelp.Unbind()
	l.InfiniteScrolling = true
	brightenHelpStyles(&l)
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
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// commandPendingHelpKeys returns key.Binding entries for command-pending mode.
// Only enter (run here), n, b, /, and q are shown; s, x, e, and d are omitted.
func commandPendingHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run here")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "browse")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// newProjectList creates and configures a bubbles/list.Model for projects.
// See newSessionList for the rationale behind SetShowHelp(false) and the
// retained brightenHelpStyles call.
func newProjectList() list.Model {
	l := list.New(nil, ProjectDelegate{}, 0, 0)
	l.Title = "Projects"
	l.DisableQuitKeybindings()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName("project", "saved projects")
	l.SetShowHelp(false)
	l.KeyMap.ShowFullHelp.Unbind()
	l.KeyMap.CloseFullHelp.Unbind()
	l.InfiniteScrolling = true
	brightenHelpStyles(&l)
	return l
}

// keymapFooterColumnSize is the fixed per-column entry count for the
// manually-rendered sessions/projects keymap footer. The footer is split
// into exactly three columns of this size in entry-source order; the third
// column may be short when the total entry count does not divide evenly.
// This constant is intentionally not dynamic — it does not branch on
// terminal width or visible entry count. Picked at ~5 to match the natural
// shape of the existing per-page binding sets (see specification).
const keymapFooterColumnSize = 5

// sessionFooterBindings returns the ordered key.Binding entries that feed
// the sessions-page manual keymap footer. The order mirrors what
// list.Model.FullHelp would surface for the sessions list: navigation keys
// from list.KeyMap, then filter-mode bindings, then the sessions-page-
// specific entries from sessionHelpKeys. Disabled bindings (e.g. filter
// keys outside filter mode, or AcceptWhileFiltering with an empty filter
// input) are filtered downstream by chunkBindingsIntoThreeColumns so the
// visible column lengths match what the renderer emits.
func sessionFooterBindings(l list.Model) []key.Binding {
	bindings := []key.Binding{
		l.KeyMap.CursorUp,
		l.KeyMap.CursorDown,
		l.KeyMap.NextPage,
		l.KeyMap.PrevPage,
		l.KeyMap.GoToStart,
		l.KeyMap.GoToEnd,
		l.KeyMap.Filter,
		l.KeyMap.ClearFilter,
		l.KeyMap.AcceptWhileFiltering,
		l.KeyMap.CancelWhileFiltering,
	}
	return append(bindings, sessionHelpKeys()...)
}

// projectFooterBindings returns the ordered key.Binding entries for the
// projects-page manual keymap footer. Mirrors sessionFooterBindings but
// sources the page-specific entries from projectHelpKeys in normal mode
// and commandPendingHelpKeys in command-pending mode (matches the prior
// AdditionalFullHelpKeys swap performed by WithCommand).
func projectFooterBindings(l list.Model, commandPending bool) []key.Binding {
	bindings := []key.Binding{
		l.KeyMap.CursorUp,
		l.KeyMap.CursorDown,
		l.KeyMap.NextPage,
		l.KeyMap.PrevPage,
		l.KeyMap.GoToStart,
		l.KeyMap.GoToEnd,
		l.KeyMap.Filter,
		l.KeyMap.ClearFilter,
		l.KeyMap.AcceptWhileFiltering,
		l.KeyMap.CancelWhileFiltering,
	}
	if commandPending {
		return append(bindings, commandPendingHelpKeys()...)
	}
	return append(bindings, projectHelpKeys()...)
}

// chunkBindingsIntoThreeColumns filters disabled bindings (so the visible
// column count matches what the manual footer renderer emits, mirroring
// help.Model.FullHelpView's own per-column Enabled() filter), then splits
// the survivors into exactly three columns of keymapFooterColumnSize
// entries in source order. The third column may be shorter when the
// remaining entry count does not divide evenly; that is acceptable per
// specification. Short trailing columns are not padded.
func chunkBindingsIntoThreeColumns(bindings []key.Binding) [][]key.Binding {
	enabled := make([]key.Binding, 0, len(bindings))
	for _, b := range bindings {
		if b.Enabled() {
			enabled = append(enabled, b)
		}
	}
	cols := make([][]key.Binding, 3)
	for i := 0; i < 3; i++ {
		start := i * keymapFooterColumnSize
		if start >= len(enabled) {
			cols[i] = nil
			continue
		}
		end := start + keymapFooterColumnSize
		if end > len(enabled) {
			end = len(enabled)
		}
		cols[i] = enabled[start:end]
	}
	return cols
}

// renderKeymapFooter renders the three-column manual keymap footer for the
// given list, using the list's own help.Model and styles so the rendered
// strip is byte-identical to the previous bubbles/list-driven bar in colour
// and separator characters. The list's Styles.HelpStyle is applied around
// the rendered columns to preserve the previous vertical padding.
func renderKeymapFooter(l list.Model, bindings []key.Binding) string {
	cols := chunkBindingsIntoThreeColumns(bindings)
	return l.Styles.HelpStyle.Render(l.Help.FullHelpView(cols))
}

// isRuneKey reports whether msg is a rune key matching the given character.
func isRuneKey(msg tea.KeyMsg, ch string) bool {
	return msg.Type == tea.KeyRunes && string(msg.Runes) == ch
}

// New creates a Model that fetches sessions from the given SessionLister.
// Optional dependencies are configured via functional options.
func New(lister SessionLister, opts ...Option) Model {
	m := Model{
		sessionLister: lister,
		sessionList:   newSessionList(nil),
		projectList:   newProjectList(),
		activePage:    PageSessions,
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
	pl := newProjectList()
	m := Model{
		sessions:    sessions,
		sessionList: l,
		projectList: pl,
		activePage:  PageSessions,
	}
	// Apply construction-time fallback dimensions through the size helpers
	// so the list reserves room for the manual keymap footer at every
	// SetSize call site (including this 80x24 test seed).
	m.applySessionListSize(80, 24)
	m.applyProjectListSize(80, 24)
	return m
}

// sessionFooterHeight returns the rendered line height of the sessions-page
// manual keymap footer for the current list state. Used to subtract footer
// rows from the available list height at every SetSize call site, so the
// list does not overflow under the manually-composed footer.
func (m Model) sessionFooterHeight() int {
	return lipgloss.Height(renderKeymapFooter(m.sessionList, sessionFooterBindings(m.sessionList)))
}

// projectFooterHeight is the projects-page counterpart to sessionFooterHeight.
// Sources its binding set from projectFooterBindings (which branches on
// commandPending to swap projectHelpKeys for commandPendingHelpKeys).
func (m Model) projectFooterHeight() int {
	return lipgloss.Height(renderKeymapFooter(m.projectList, projectFooterBindings(m.projectList, m.commandPending)))
}

// applySessionListSize sets the sessions-list dimensions, subtracting the
// manual keymap footer height from the available height so the list does
// not overflow under the footer. Width passes through unchanged because
// the manual footer wraps inside the same width as the list view.
func (m *Model) applySessionListSize(width, height int) {
	m.sessionList.SetSize(width, height-m.sessionFooterHeight())
}

// applyProjectListSize is the projects-list counterpart to
// applySessionListSize.
func (m *Model) applyProjectListSize(width, height int) {
	m.projectList.SetSize(width, height-m.projectFooterHeight())
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

// applySessions ingests a fresh session slice into the list model: it stores
// the slice on the model, recomputes the filtered (inside-tmux excluded)
// view, pushes the resulting items into sessionList, and re-applies the
// terminal size so pagination accounts for full help height. This is the
// single canonical sequence shared by the SessionsMsg and
// previewSessionsRefreshedMsg handlers; handler-specific tail logic
// (e.g. inside-tmux title rewrite) stays at the call site.
func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd {
	m.sessions = sessions
	filtered := m.filteredSessions()
	cmd := m.sessionList.SetItems(ToListItems(filtered))
	// Re-apply terminal size so pagination accounts for the manual keymap
	// footer height (see applySessionListSize). Without this re-apply the
	// list would size itself against pre-load defaults and overflow under
	// the footer once items populate.
	if m.termWidth > 0 || m.termHeight > 0 {
		m.applySessionListSize(m.termWidth, m.termHeight)
	}
	return cmd
}

// evaluateDefaultPage sets the active page based on loaded data.
// It only runs once both sessions and projects have been loaded.
// In command-pending mode, always sets PageProjects regardless of session list contents.
// In normal mode, if sessions exist (after inside-tmux filtering), show Sessions page;
// otherwise show Projects page.
// After determining the default page, applies any initial filter to that page's list.
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
	if m.commandPending {
		m.activePage = PageProjects
	} else if len(m.sessionList.Items()) > 0 {
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

// refreshSessionsAfterPreviewCmd builds the tea.Cmd that performs the live
// Sessions-list re-fetch dispatched when the user dismisses the preview
// page (see § Cross-cutting Seams > Externally-Killed Session During
// Preview). preserveName is forwarded into the resulting message so the
// handler can re-anchor the cursor by name without re-reading the model.
//
// Returns nil when no SessionLister is wired (test harnesses may construct
// Model without one) — callers must tolerate a nil cmd.
func (m Model) refreshSessionsAfterPreviewCmd(preserveName string) tea.Cmd {
	if m.sessionLister == nil {
		return nil
	}
	lister := m.sessionLister
	return func() tea.Msg {
		sessions, err := lister.ListSessions()
		return previewSessionsRefreshedMsg{
			Sessions:     sessions,
			Err:          err,
			PreserveName: preserveName,
		}
	}
}

// exitPreviewToSessions performs the shared preview-teardown prelude used by
// both previewDismissedMsg (Esc) and previewAttachBailMsg (HasSessionProbe
// reported gone) handlers. It flips activePage back to PageSessions, zeroes
// m.preview to release the viewport buffer, and returns the live Sessions
// refresh command anchored on preserveName. Callers MUST capture
// preserveName BEFORE invoking this helper — m.preview is zeroed in-place,
// so reading m.preview.session afterwards yields an empty string.
//
// The returned cmd may be nil when no SessionLister is wired (test
// harnesses); callers tea.Batch'ing the result must tolerate that.
func (m *Model) exitPreviewToSessions(preserveName string) tea.Cmd {
	m.activePage = PageSessions
	m.preview = previewModel{}
	return m.refreshSessionsAfterPreviewCmd(preserveName)
}

// reanchorSessionCursor moves the bubbles/list cursor onto the visible item
// whose session name matches name, after a SetItems update. If name is no
// longer present (e.g. the session was killed externally during preview),
// the cursor is clamped to the new last index, satisfying the "fall back
// to a valid neighbour without panic" contract.
//
// Operates on filtered (visible) order so the cursor lands on the
// expected item even with a committed filter.
func (m *Model) reanchorSessionCursor(name string) {
	if name == "" {
		return
	}
	visible := m.sessionList.VisibleItems()
	if len(visible) == 0 {
		return
	}
	for i, it := range visible {
		si, ok := it.(SessionItem)
		if !ok {
			continue
		}
		if si.Session.Name == name {
			m.sessionList.Select(i)
			return
		}
	}
	// Name no longer present — clamp to last visible index.
	m.sessionList.Select(len(visible) - 1)
}

// setFlash records an inline-flash message on the Sessions page. It bumps
// flashGen by one and assigns flashText. Callers that schedule a delayed
// clear must capture the post-bump generation and compare against the
// live flashGen on fire so a stale tick from a superseded flash cannot
// early-clear the current one (spec § Replacement on rapid successive
// bails). Pure state primitive — no render, no scheduling. setFlash("")
// still bumps the generation; the caller decides what counts as a flash.
func (m *Model) setFlash(text string) {
	m.flashGen++
	m.flashText = text
}

// clearFlash zeros flashText, leaving flashGen untouched. Idempotent: a
// clear of an already-cleared state is a no-op observable to callers.
// flashGen is preserved so any in-flight ticks scheduled before the clear
// continue to compare against the same monotonic sequence.
func (m *Model) clearFlash() {
	m.flashText = ""
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

// transitionFromLoading moves from the loading page to the normal sessions page.
// It marks sessions as loaded so evaluateDefaultPage can determine the correct
// landing page when projects also finish loading.
func (m *Model) transitionFromLoading() {
	m.activePage = PageSessions
	m.sessionsLoaded = true
	m.evaluateDefaultPage()
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
// when in command-pending mode. When on PageLoading, it also schedules
// a single LoadingMinElapsedMsg tick to enforce the loading page's
// minimum-display window. The matching BootstrapCompleteMsg is sent by
// the bootstrap orchestrator (see task 5-7); both are required to
// dismiss the loading page.
func (m Model) Init() tea.Cmd {
	if m.commandPending {
		return m.loadProjects()
	}
	fetchSessions := func() tea.Msg {
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
	loadProjects := m.loadProjects()

	if m.activePage == PageLoading {
		loadingPadTick := tea.Tick(LoadingMinDuration, func(time.Time) tea.Msg {
			return LoadingMinElapsedMsg{}
		})
		// Bubble Tea launches AFTER PersistentPreRunE has finished synchronously,
		// so the bootstrap orchestrator has already returned by the time Init
		// runs. Emit BootstrapCompleteMsg from the first event-loop tick to
		// satisfy the bootstrapComplete gate, carrying any pending warnings
		// drained from the package-level sink by openTUI. Loading dismissal
		// still requires the LoadingMinElapsedMsg tick (1.2s minimum-display
		// floor); warnings are flushed to stderr at that moment via
		// flushBufferedWarningsCmd.
		pending := m.pendingBootstrapWarnings
		bootstrapCompleteCmd := func() tea.Msg { return BootstrapCompleteMsg{Warnings: pending} }
		cmds := []tea.Cmd{fetchSessions, loadingPadTick, bootstrapCompleteCmd}
		if loadProjects != nil {
			cmds = append(cmds, loadProjects)
		}
		return tea.Batch(cmds...)
	}

	if loadProjects != nil {
		return tea.Batch(fetchSessions, loadProjects)
	}
	return fetchSessions
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward WindowSizeMsg to both page lists so they have correct dimensions
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = wsm.Width
		m.termHeight = wsm.Height
		m.applySessionListSize(wsm.Width, wsm.Height)
		m.applyProjectListSize(wsm.Width, wsm.Height)
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
		cmd := m.applySessions(msg.Sessions)

		if m.insideTmux && m.currentSession != "" {
			m.sessionList.Title = fmt.Sprintf("Sessions (current: %s)", m.currentSession)
		}

		// SessionsMsg no longer drives loading-page dismissal. The orchestrator
		// signals completion via BootstrapCompleteMsg, paired with the
		// LoadingMinElapsedMsg tick — both gate the transition. While on
		// PageLoading, we still ingest the session list so it is ready when
		// transitionFromLoading runs, but we do not flip sessionsLoaded here:
		// transitionFromLoading does that as part of evaluateDefaultPage.
		if m.activePage == PageLoading {
			return m, cmd
		}

		m.sessionsLoaded = true
		m.evaluateDefaultPage()
		return m, cmd
	case LoadingMinElapsedMsg:
		m.minElapsed = true
		if m.bootstrapComplete && m.activePage == PageLoading {
			m.transitionFromLoading()
			return m, m.flushBufferedWarningsCmd()
		}
		return m, nil
	case BootstrapCompleteMsg:
		m.bootstrapComplete = true
		// Only buffer warnings when still on the loading page; warnings
		// that arrive after dismissal (orphaned BootstrapCompleteMsg) are
		// dropped so they cannot accumulate dead state on the model.
		if m.activePage == PageLoading {
			m.bufferedWarnings = msg.Warnings
		}
		if m.minElapsed && m.activePage == PageLoading {
			m.transitionFromLoading()
			return m, m.flushBufferedWarningsCmd()
		}
		return m, nil
	case ProjectsLoadedMsg:
		var setItemsCmd tea.Cmd
		if msg.Err == nil {
			items := ProjectsToListItems(msg.Projects)
			setItemsCmd = m.projectList.SetItems(items)
			// Re-apply terminal size so pagination accounts for the manual
			// keymap footer height (see applyProjectListSize).
			if m.termWidth > 0 || m.termHeight > 0 {
				m.applyProjectListSize(m.termWidth, m.termHeight)
			}
		}
		m.projectsLoaded = true
		m.evaluateDefaultPage()
		return m, setItemsCmd
	case SessionCreatedMsg:
		m.selected = msg.SessionName
		return m, tea.Quit
	case sessionCreateErrMsg:
		// On error, return to current page
		return m, nil
	case previewDismissedMsg:
		// Esc inside preview: flip back to Sessions page. The sessionList
		// is intentionally NOT mutated here — cursor position and filter
		// state must round-trip byte-identically, so the only change is
		// activePage. Zero out preview to release the viewport buffer;
		// re-opening preview constructs a fresh previewModel via Space
		// (which re-runs enumeration and re-reads the tail-N slice).
		//
		// Per § Cross-cutting Seams > Externally-Killed Session During
		// Preview, dismissing must also re-fetch the live Sessions list so
		// a session killed externally during preview does not linger in the
		// post-dismiss view. The refresh is dispatched as a tea.Cmd that
		// emits previewSessionsRefreshedMsg, which the handler below
		// consumes; cursor is re-anchored by name so a still-existing
		// previously-highlighted session keeps its cursor and a removed
		// one falls back to a clamped neighbour.
		// Capture preserveName BEFORE invoking exitPreviewToSessions —
		// the helper zeroes m.preview, so reading m.preview.session
		// afterwards silently sends an empty value to the refresh handler.
		captured := m.preview.session
		return m, m.exitPreviewToSessions(captured)
	case previewAttachBailMsg:
		// Session-killed-externally bail path (spec § Session-killed-externally
		// bail path > Behaviour). Mirrors previewDismissedMsg: transition
		// pagePreview → PageSessions, zero m.preview, dispatch the sessions-list
		// refresh so the externally-killed session disappears from the post-bail
		// view. Reads from msg.Session (not m.preview.session) for robustness —
		// the pipeline owns the source of truth for the captured session name.
		//
		// Phase 2 (this handler): also emit the inline flash with the spec-exact
		// wording, then schedule a flashTickCmd capturing the POST-bump flashGen
		// (setFlash bumps the generation first). Refresh + tick are composed via
		// tea.Batch — NOT tea.Sequence — so the flash is rendered immediately on
		// the same render frame as the page flip, before the async refresh
		// resolves (spec § Render-frame ordering: "visible response first, list
		// consistency converges within a render or two"). tea.Batch tolerates
		// nil cmds, so a nil refresh (no SessionLister wired) is safe.
		refreshCmd := m.exitPreviewToSessions(msg.Session)
		m.setFlash(formatSessionGoneFlash(msg.Session))
		return m, tea.Batch(refreshCmd, flashTickCmd(m.flashGen))
	case previewAttachSelectedMsg:
		// Success terminal of the preview-Enter pipeline. The pre-select
		// tmux calls (HasSessionProbe + SelectWindow + SelectPane) have
		// already run inside the live tea.Cmd; we now record the captured
		// session on the model and quit so the TUI program shuts down
		// before the connector runs.
		//
		// Shape parity with handleSessionListEnter (the Sessions-page
		// Enter handler): set m.selected + tea.Quit; processTUIResult
		// performs the connector.Connect call AFTER tea.NewProgram.Run
		// returns. Inside-tmux this prevents the orphan portal process
		// regression where switch-client would move the surrounding tmux
		// client to the target session while portal kept event-looping
		// with no UI. Outside-tmux the connector's syscall.Exec replaces
		// the process post-TUI; same effect either way.
		m.selected = msg.Session
		return m, tea.Quit
	case previewSessionsRefreshedMsg:
		// Lister errors are non-fatal here: the user just dismissed
		// preview and expects to land on the Sessions list. A tea.Quit
		// would be hostile, and zeroing out the list would also be wrong
		// — the pre-refresh snapshot is the best information we still
		// have. Drop the error silently and leave the existing list
		// intact.
		if msg.Err != nil {
			return m, nil
		}
		cmd := m.applySessions(msg.Sessions)
		m.reanchorSessionCursor(msg.PreserveName)
		return m, cmd
	case flashTickMsg:
		// Generation-guard: a tick scheduled for an earlier flash must
		// not early-clear a flash that has since been replaced by a
		// rapid successive bail (spec § Replacement on rapid successive
		// bails). setFlash bumps flashGen monotonically; flashTickCmd
		// captures the gen at schedule time. On fire we clear only if
		// the captured gen still matches the live gen — otherwise the
		// tick belongs to a superseded flash and is silently dropped.
		// Also safely no-ops if the flash was cleared manually (e.g.
		// by a keystroke) since clearFlash leaves flashGen untouched
		// and clearFlash itself is idempotent.
		if msg.Gen == m.flashGen {
			m.clearFlash()
		}
		return m, nil
	}

	// Delegate to the active view
	switch m.activePage {
	case PageLoading:
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	case PageProjects:
		return m.updateProjectsPage(msg)
	case pageFileBrowser:
		return m.updateFileBrowser(msg)
	case pagePreview:
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
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
		return m.updateModal(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Swallow ? so the list can't toggle help off
		if isRuneKey(msg, "?") {
			return m, nil
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
		case isRuneKey(msg, "q"):
			return m, tea.Quit
		case isRuneKey(msg, "s"):
			if m.commandPending {
				return m, nil
			}
			m.activePage = PageSessions
			return m, nil
		case isRuneKey(msg, "x"):
			if m.commandPending {
				return m, nil
			}
			m.activePage = PageSessions
			return m, nil
		case isRuneKey(msg, "n"):
			return m.handleNewInCWD()
		case isRuneKey(msg, "d"):
			if m.commandPending {
				return m, nil
			}
			return m.handleDeleteProjectKey()
		case isRuneKey(msg, "e"):
			if m.commandPending {
				return m, nil
			}
			return m.handleEditProjectKey()
		case isRuneKey(msg, "b"):
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

func (m Model) updateDeleteProjectModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case isRuneKey(keyMsg, "y"):
		path := m.pendingDeletePath
		m.modal = modalNone
		m.pendingDeletePath = ""
		m.pendingDeleteName = ""
		return m, m.deleteAndRefreshProjects(path)
	case isRuneKey(keyMsg, "n"),
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
		// Spec § Inline flash > Clear conditions, § Flash interaction with
		// filter input: an actionable KeyMsg with an active flash clears
		// the flash as a side effect and the keystroke continues to its
		// normal handler — "one key, one intent". Non-KeyMsg events
		// (WindowSizeMsg, FocusMsg, BlurMsg, MouseMsg) never reach this
		// branch, so the flash survives them. When no flash is active
		// the check is a single bool read with no observable effect.
		if m.flashText != "" && isActionableKey(msg) {
			m.clearFlash()
			// Deliberate fall-through: do NOT return. The keystroke
			// continues to the existing handlers below.
		}
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Swallow ? so the list can't toggle help off
		if isRuneKey(msg, "?") {
			return m, nil
		}
		// When the list is actively filtering, let it handle all key input
		if m.sessionList.SettingFilter() {
			break
		}
		// Space opens the scrollback preview for the highlighted session.
		// Bound before the rest of the keymap so it never collides with
		// later rune-based handlers. No-op when the list is empty, when
		// no item is highlighted (committed filter narrowed to zero
		// matches), or when NewPreviewModel reports ok=false (enumeration
		// failure or empty enumeration — both observably identical).
		if msg.Type == tea.KeySpace {
			if len(m.sessionList.Items()) == 0 {
				return m, nil
			}
			si, ok := m.selectedSessionItem()
			if !ok {
				return m, nil
			}
			pmodel, ok := NewPreviewModel(si.Session.Name, m.enumerator, m.reader, m.previewAttacher, m.termWidth, m.termHeight)
			if !ok {
				return m, nil
			}
			m.preview = pmodel
			m.activePage = pagePreview
			return m, nil
		}
		switch {
		case msg.Type == tea.KeyEsc:
			// Progressive back: if filter is active, let the list clear it;
			// otherwise quit.
			if m.sessionList.FilterState() == list.FilterApplied {
				break
			}
			return m, tea.Quit
		case isRuneKey(msg, "q"):
			return m, tea.Quit
		case isRuneKey(msg, "k"):
			return m.handleKillKey()
		case isRuneKey(msg, "r"):
			return m.handleRenameKey()
		case isRuneKey(msg, "n"):
			return m.handleNewInCWD()
		case isRuneKey(msg, "p"):
			m.activePage = PageProjects
			return m, nil
		case isRuneKey(msg, "x"):
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
	case modalDeleteProject:
		return m.updateDeleteProjectModal(msg)
	case modalEditProject:
		return m.updateEditProjectModal(msg)
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
	case isRuneKey(keyMsg, "y"):
		name := m.pendingKillName
		m.modal = modalNone
		m.pendingKillName = ""
		return m, m.killAndRefresh(name)
	case isRuneKey(keyMsg, "n"),
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
	case PageLoading:
		return m.viewLoading()
	case PageProjects:
		if m.commandPending {
			listView := m.viewProjectList()
			statusLine := "Select project to run: " + strings.Join(m.command, " ")
			// Insert status line after the first line (title) of the list view
			if idx := strings.IndexByte(listView, '\n'); idx >= 0 {
				return listView[:idx+1] + statusLine + "\n" + listView[idx+1:]
			}
			return listView + "\n" + statusLine
		}
		return m.viewProjectList()
	case pageFileBrowser:
		return m.fileBrowser.View()
	case pagePreview:
		return m.preview.View()
	default:
		return m.viewSessionList()
	}
}

// viewLoading renders the loading interstitial with centered text.
func (m Model) viewLoading() string {
	w := m.termWidth
	h := m.termHeight
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	text := "Restoring sessions…"
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, text)
}

// viewProjectList renders the project list, with optional modal overlay,
// and composes the manual three-column keymap footer beneath the
// list-plus-modal block. The modal overlay sits inside renderListWithModal
// over the list view only; the manual footer must sit below the composed
// block so the modal never overlays it.
func (m Model) viewProjectList() string {
	var modalContent string
	switch m.modal {
	case modalDeleteProject:
		modalContent = fmt.Sprintf("Delete %s? (y/n)", m.pendingDeleteName)
	case modalEditProject:
		modalContent = m.renderEditProjectContent()
	}
	listView := renderListWithModal(m.projectList, modalContent)
	footer := renderKeymapFooter(m.projectList, projectFooterBindings(m.projectList, m.commandPending))
	return lipgloss.JoinVertical(lipgloss.Left, listView, footer)
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

// viewSessionList renders the session list using bubbles/list, composes the
// optional inline flash row between the list's title/filter row and the
// rest, then composes the manual three-column keymap footer beneath the
// resulting block. When the flash is inactive the list sits directly under
// the title/filter as before (spec § Inline flash — feature-local
// infrastructure > Render). The manual footer sits below the composed
// list+modal+flash block so the modal overlay never overlays it.
func (m Model) viewSessionList() string {
	var modalContent string
	switch m.modal {
	case modalKillConfirm:
		modalContent = fmt.Sprintf("Kill %s? (y/n)", m.pendingKillName)
	case modalRename:
		modalContent = m.renameInput.View()
	}
	listView := renderListWithModal(m.sessionList, modalContent)
	if m.flashText != "" {
		// Split off the first line (title / filter input row) and insert
		// the flash row between it and the remainder. Using a manual split
		// keeps the existing list view byte-identical aside from the
		// inserted row, satisfying "no existing chrome replaced or
		// overlaid".
		if idx := strings.IndexByte(listView, '\n'); idx < 0 {
			// Single-line list view (degenerate); append the flash below.
			listView = listView + "\n" + m.renderFlashRow()
		} else {
			listView = listView[:idx+1] + m.renderFlashRow() + "\n" + listView[idx+1:]
		}
	}
	footer := renderKeymapFooter(m.sessionList, sessionFooterBindings(m.sessionList))
	return lipgloss.JoinVertical(lipgloss.Left, listView, footer)
}

// flashRowStyle is a package-level immutable lipgloss style; lipgloss
// value semantics prevent mutation bleed across renders. Subdued / dim
// foreground keeps the chrome low-emphasis (spec § Inline flash —
// feature-local infrastructure > Render: "styled row").
var flashRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

// renderFlashRow returns the styled flash row for the Sessions page.
// flashText is rendered verbatim (no truncation, no transformation); the
// caller is responsible for the message wording (spec pins it as
// `session "<name>" no longer exists` at the call site).
func (m Model) renderFlashRow() string {
	return flashRowStyle.Render(m.flashText)
}
