// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
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
	Remove(path, via string) error
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

// ModePersister persists the session-list grouping mode. The production
// implementation is *prefs.Store (its Save(prefs.SessionListMode) error method
// satisfies this seam); the model imports prefs only for the SessionListMode
// value type and never constructs the store itself.
type ModePersister interface {
	Save(prefs.SessionListMode) error
}

// ProjectEditor defines the interface for renaming projects and mutating their
// tag set. AddTag/RemoveTag take the raw tag value and delegate canonicalisation
// and dedup to the store (Phase 1's project.Store.AddTag/RemoveTag); the modal
// never re-normalises. There is no via parameter — the store hardcodes via=cli
// for tag mutations (the projects-edit modal is the sole origin).
type ProjectEditor interface {
	Rename(path, newName, via string) error
	AddTag(path, rawTag string) error
	RemoveTag(path, rawTag string) error
}

// AliasEditor defines the interface for managing aliases in edit mode.
//
// The mutation surface is the audited combined store-seam methods
// (SetAndSave / DeleteAndSave) — not the raw Set / Delete / Save — so every TUI
// alias edit is uniformly audited under the aliases component. Load stays for
// the collision pre-check.
type AliasEditor interface {
	Load() (map[string]string, error)
	SetAndSave(name, path, via string) error
	DeleteAndSave(name, via string) (bool, error)
}

// editField tracks which field has focus in the edit modal.
type editField int

const (
	editFieldName editField = iota
	editFieldAliases
	editFieldTags
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
	sessionList list.Model
	sessions    []tmux.Session
	projects    []project.Project
	// projectIndex is a DERIVED CACHE of projects: a pre-canonicalised
	// dir→project lookup table (project.NewIndex) consumed by the grouping
	// builders so a grouped render is map lookups, not an
	// O(sessions × projects) CanonicalDirKey/EvalSymlinks scan. It MUST be
	// rebuilt whenever projects changes — always mutate both via setProjects.
	projectIndex      project.Index
	sessionListMode   prefs.SessionListMode
	modePersister     ModePersister
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

	// dirReader and dirRunner are the render-layer directory-resolution seam
	// consumed by rebuildSessionList's lazy fallback. When both are non-nil,
	// each session whose @portal-dir is absent is resolved live from its
	// active pane → git-root via session.ResolveSessionDir before the grouping
	// builders consume Session.Dir, and the result is cached back into
	// m.sessions (resolveSessionDirs) so subsequent rebuilds in the same picker
	// session skip the pane read. The derived directory is NOT stamped back to
	// tmux: a session's pane cwd can drift away from its origin directory, and
	// freezing that drift onto @portal-dir would permanently mis-group the
	// session (the .dotfiles-under-portal bug). New sessions are anchored at
	// creation instead (session.CreateFromDir / QuickStart stamp @portal-dir);
	// the lazy read here is a best-effort guess for legacy un-stamped sessions
	// that self-corrects on the next picker launch. Production wiring is
	// *tmux.Client + &resolver.RealCommandRunner{} via WithDirResolver; tests
	// that omit the option leave both nil, in which case the resolution pass is
	// skipped and un-stamped sessions route to Unknown/Untagged.
	dirReader session.PaneCurrentPathReader
	dirRunner resolver.CommandRunner

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

	// byTagSignpost is the persistent "No tags yet" signpost flag (spec §
	// Mode Persistence & Empty States → Empty states → By Tag with zero
	// tags). It is set true by rebuildSessionList whenever ModeByTag is
	// active AND no project carries any tag — the zero-tags-anywhere gate.
	// In that state the list is built with the plain flat builder (degrade
	// to flat) while viewSessionList renders a dimmed signpost row. Unlike
	// flashText (transient, cleared on the next actionable key), this is a
	// derived persistent flag recomputed on every rebuild — it clears the
	// moment the mode leaves ByTag or any tag appears.
	byTagSignpost bool

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

	// Tag-buffer modal state, parallel to the alias-buffer fields above.
	editTags        []string // working copy of the project's tags (persisted live)
	editTagsMutated bool     // a tag was added/removed this session (triggers a projects reload on modal close)
	editNewTag      string   // in-progress Add-input text (parallel to editNewAlias)
	editTagCursor   int      // highlighted row within the Tags block
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
	// Route item population (and the mode-aware + inside-tmux title) through the
	// single rebuildSessionList chokepoint so inside-tmux exclusion composes with
	// mode/grouping/dir-resolution rather than bypassing them via a direct
	// SetItems(ToListItems(...)) push. insideTmux/currentSession are set above so
	// rebuildSessionList reads them for the filtered view and the title. The
	// returned cmd is discarded: at construction m.sessions is empty so SetItems
	// yields no real cmd, matching the prior behaviour. WithInsideTmux is now a
	// chokepoint participant, not a chokepoint-bypassing path.
	(&m).rebuildSessionList()
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

// WithInitialMode sets the persisted session-list grouping mode that the model
// opens in. Production wiring reads it from prefs.json (via cmd/open.go's
// loadPrefsStore + Store.Load, tolerant to ModeFlat) and injects it here; the
// New constructor recomputes the list title after options apply so the first
// frame paints the correct mode heading. Flat is a valid explicit value, so the
// caller always passes the option. The mode is re-applied on every session
// ingestion (applySessions → rebuildSessionList), so it does not depend on
// sessions already being loaded at construction time.
func WithInitialMode(mode prefs.SessionListMode) Option {
	return func(m *Model) {
		m.sessionListMode = mode
	}
}

// WithModePersister sets the session-list mode persister dependency. Production
// wiring passes a *prefs.Store; tests that do not exercise the s toggle can omit
// this option, leaving modePersister nil (the handler tolerates a nil persister).
func WithModePersister(p ModePersister) Option {
	return func(m *Model) {
		m.modePersister = p
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
// the preview component's *slog.Logger); tests that do not exercise Enter
// can omit this option, leaving previewAttacher nil.
func WithPreviewAttachPipeline(p PreviewAttacher) Option {
	return func(m *Model) {
		m.previewAttacher = p
	}
}

// WithDirResolver wires the render-layer directory-resolution seam used by
// rebuildSessionList's lazy fallback. Production callers pass the concrete
// *tmux.Client (which satisfies session.PaneCurrentPathReader via
// ActivePaneCurrentPath) and a &resolver.RealCommandRunner{}. When wired, each
// session with an absent @portal-dir is resolved live from its active pane →
// git-root before grouping and cached in-memory (never stamped back to tmux);
// tests that omit this option leave both seams nil and the resolution pass is
// skipped.
func WithDirResolver(reader session.PaneCurrentPathReader, runner resolver.CommandRunner) Option {
	return func(m *Model) {
		m.dirReader = reader
		m.dirRunner = runner
	}
}

// sessionHelpKeys returns key.Binding entries for session-specific actions.
func sessionHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "attach")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill")),
		key.NewBinding(key.WithKeys("p"), key.WithHelp("p/x", "projects")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
		key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "preview")),
		// "s switch view" sits adjacent to the other view-related action
		// ("space/preview") so it reads naturally in the manual footer. The
		// footer column split is positional (keymapFooterColumnSize = 5, three
		// columns in source order): with nav/filter bindings ahead of these,
		// this entry lands in the trailing columns next to preview rather than
		// being orphaned in a column of its own. (spec § TUI Rendering &
		// Toggle Behaviour → Mode indication, Toggle key.)
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "switch view")),
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

// sessionListTitleForMode computes the session list title for the active
// grouping mode, reconciling it with the inside-tmux current-session
// decoration. The three base strings come from the spec (§ TUI Rendering &
// Toggle Behaviour → Mode indication): ModeFlat → "Sessions",
// ModeByProject → "Sessions — by project", ModeByTag → "Sessions — by tag".
// The separator is " — " (an em-dash U+2014 with surrounding spaces).
//
// DIVERGENCE FROM SPEC: the spec's title scheme specifies only those three
// base strings and does not mention the pre-existing "(current: %s)"
// decoration that the inside-tmux path already showed. Rather than dropping
// that decoration, this function preserves it as a suffix composed onto the
// mode base (e.g. "Sessions — by tag (current: foo)") — the minimal
// reconciliation. This is the single place to change that composition later.
func sessionListTitleForMode(mode prefs.SessionListMode, insideTmux bool, currentSession string) string {
	var base string
	switch mode {
	case prefs.ModeByProject:
		base = "Sessions — by project"
	case prefs.ModeByTag:
		base = "Sessions — by tag"
	default:
		base = "Sessions"
	}
	if insideTmux && currentSession != "" {
		return fmt.Sprintf("%s (current: %s)", base, currentSession)
	}
	return base
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
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s/x", "sessions")),
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

// listNavAndFilterBindings returns the shared navigation + filter-mode
// binding prefix common to both the sessions-page and projects-page manual
// keymap footers. Sourced directly from the list's KeyMap so future drift
// in the bubbles/list nav/filter binding set can never silently diverge
// between the two pages. Returned slice is freshly allocated so callers
// may safely append page-specific tail entries without aliasing.
func listNavAndFilterBindings(l *list.Model) []key.Binding {
	return []key.Binding{
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
}

// sessionFooterBindings returns the ordered key.Binding entries that feed
// the sessions-page manual keymap footer. The order mirrors what
// list.Model.FullHelp would surface for the sessions list: navigation keys
// from list.KeyMap, then filter-mode bindings, then the sessions-page-
// specific entries from sessionHelpKeys. Disabled bindings (e.g. filter
// keys outside filter mode, or AcceptWhileFiltering with an empty filter
// input) are filtered downstream by chunkBindingsIntoThreeColumns so the
// visible column lengths match what the renderer emits.
func sessionFooterBindings(l *list.Model) []key.Binding {
	return append(listNavAndFilterBindings(l), sessionHelpKeys()...)
}

// projectFooterBindings returns the ordered key.Binding entries for the
// projects-page manual keymap footer. Mirrors sessionFooterBindings but
// sources the page-specific entries from projectHelpKeys in normal mode
// and commandPendingHelpKeys in command-pending mode (matches the prior
// AdditionalFullHelpKeys swap performed by WithCommand).
func projectFooterBindings(l *list.Model, commandPending bool) []key.Binding {
	bindings := listNavAndFilterBindings(l)
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
func renderKeymapFooter(l *list.Model, bindings []key.Binding) string {
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
	// Recompute the title for the initial mode once options have been applied
	// (newSessionList sets the Flat default before mode / inside-tmux are known).
	// 3-9 injects the persisted mode via an Option, so opening in By Tag must
	// paint "Sessions — by tag" on the first frame.
	m.sessionList.Title = sessionListTitleForMode(m.sessionListMode, m.insideTmux, m.currentSession)
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
	// Apply construction-time fallback dimensions through the size helper
	// so each list reserves room for the manual keymap footer at every
	// SetSize call site (including this 80x24 test seed).
	m.applySessionListSize(80, 24)
	m.applyProjectListSize(80, 24)
	return m
}

// applyListSize sets the given list's dimensions, subtracting the rendered
// manual keymap footer height (computed from the supplied bindings) from
// the available height so the list does not overflow under the footer.
// Width passes through unchanged because the manual footer wraps inside
// the same width as the list view. Shared sizing core consumed by the
// per-page wrappers (applySessionListSize / applyProjectListSize), which
// own the list+bindings pairing invariant — callers should invoke a
// wrapper, not this core directly.
func (m *Model) applyListSize(l *list.Model, bindings []key.Binding, width, height int) {
	l.SetSize(width, height-lipgloss.Height(renderKeymapFooter(l, bindings)))
}

// applySessionListSize is the per-page wrapper that owns the
// (&m.sessionList, sessionFooterBindings(&m.sessionList)) pairing so call
// sites cannot pair the wrong list with the wrong bindings function.
func (m *Model) applySessionListSize(width, height int) {
	m.applyListSize(&m.sessionList, sessionFooterBindings(&m.sessionList), width, height)
}

// applyProjectListSize is the per-page wrapper that owns the
// (&m.projectList, projectFooterBindings(&m.projectList, m.commandPending))
// pairing — including the m.commandPending branch consumed by the bindings
// builder — so call sites cannot drift on either input.
func (m *Model) applyProjectListSize(width, height int) {
	m.applyListSize(&m.projectList, projectFooterBindings(&m.projectList, m.commandPending), width, height)
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
// previewSessionsRefreshedMsg handlers. The mode-aware (and inside-tmux
// current-session) title is now recomputed inside rebuildSessionList, so
// both handlers inherit the correct title without a call-site rewrite.
func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd {
	m.sessions = sessions
	return m.rebuildSessionList()
}

// rebuildSessionList is the single mode-aware re-render core for the session
// list. It recomputes the filtered (inside-tmux excluded) view, dispatches to
// the builder for the active sessionListMode, pushes the resulting items into
// sessionList, and re-applies the terminal size so pagination accounts for the
// full footer height.
//
// Builders are pure functions fed the cached project records (m.projects,
// populated from ProjectsLoadedMsg) — no synchronous store read happens in the
// render path. ModeFlat routes through ToListItems so its output is identical
// to the pre-grouping behaviour; ModeByProject / ModeByTag route through the
// grouping builders. Zero live sessions yields an empty list in every mode.
// nextSessionListMode advances the grouping mode one step in the fixed cycle
// Flat → By Project → By Tag → Flat. The wrap is unconditional (By Tag always
// advances to Flat). An out-of-range value collapses defensively to Flat so the
// cycle can never get stuck on an unrecognised mode.
func nextSessionListMode(mode prefs.SessionListMode) prefs.SessionListMode {
	switch mode {
	case prefs.ModeFlat:
		return prefs.ModeByProject
	case prefs.ModeByProject:
		return prefs.ModeByTag
	case prefs.ModeByTag:
		return prefs.ModeFlat
	default:
		return prefs.ModeFlat
	}
}

// anyTagsExist reports whether any project record carries at least one tag —
// the zero-tags-anywhere gate (spec § Tag Data Model → Tags are implicit: the
// set of tags that exists is the union of tags across all project records).
// Presence is all that matters: tags are already canonical from Phase 1, so a
// non-empty Tags slice on any project means "a tag exists somewhere." This is
// strictly directory-tag presence, independent of which sessions are live — so
// the tags-exist-but-all-sessions-tagged case (empty-Untagged-suppression) does
// NOT degrade to the signpost.
func anyTagsExist(projects []project.Project) bool {
	for _, p := range projects {
		if len(p.Tags) > 0 {
			return true
		}
	}
	return false
}

// setProjects updates the cached project records AND their derived lookup cache
// (projectIndex) together. It is the single production seam where m.projects
// changes, so the index can never go stale relative to the records the grouping
// builders consult. Always mutate projects through this helper.
func (m *Model) setProjects(projects []project.Project) {
	m.projects = projects
	m.projectIndex = project.NewIndex(projects)
}

// resolveSessionDirs is the render-layer chokepoint over the lazy
// directory-resolution fallback. For each session whose @portal-dir is absent
// (Session.Dir == ""), it derives the directory live from the active pane →
// git-root via session.ResolveSessionDir, and caches the result back into
// m.sessions so subsequent rebuilds in the same picker session take the fast
// path (no second pane read / git rev-parse — fixing the 2-3s "switch view"
// stall). A session whose Dir is already set is passed through untouched, so a
// fully-stamped install pays zero pane reads.
//
// The derived directory is deliberately NOT stamped back to tmux: a session's
// pane cwd can drift away from its origin directory, and freezing that drift
// onto @portal-dir would permanently mis-group the session. New sessions are
// anchored at creation instead; this lazy read is a best-effort guess for
// legacy un-stamped sessions that self-corrects on the next picker launch (when
// m.sessions is rebuilt fresh from ListSessions).
//
// It is invoked ONLY from the grouped render arms (ModeByProject / ModeByTag,
// non-signpost) in rebuildSessionList — the lazy fallback is a grouped-render
// mechanism. The Flat and byTagSignpost arms render via ToListItems, which
// ignores Session.Dir, so they consume the un-resolved sessions directly and
// pay zero pane reads.
//
// Resolution is best-effort and overwrite-on-success only: when it yields
// ok==false (unresolvable this pass — killed mid-resolve, blank pane, or no
// enclosing path) the session's Dir is left empty, so it still falls through to
// the Unknown (By Project) / Untagged (By Tag) catch-all and is re-attempted
// next rebuild. When the seam is unwired (either half nil, e.g. tests without
// WithDirResolver), the input slice is returned unchanged.
func (m *Model) resolveSessionDirs(sessions []tmux.Session) []tmux.Session {
	if m.dirReader == nil || m.dirRunner == nil {
		return sessions
	}

	resolved := make([]tmux.Session, len(sessions))
	for i, s := range sessions {
		if s.Dir == "" {
			if dir, ok, err := session.ResolveSessionDir(s.Name, m.dirReader, m.dirRunner); ok && err == nil {
				s.Dir = dir
				m.cacheSessionDir(s.Name, dir)
			}
		}
		resolved[i] = s
	}
	return resolved
}

// cacheSessionDir records a lazily-derived directory back onto the stored
// session (matched by name) so a subsequent rebuildSessionList in the same
// picker session sees Session.Dir set and takes resolveSessionDirs' fast path
// instead of re-reading the pane. A SessionsMsg refresh replaces m.sessions
// with a fresh ListSessions snapshot (Dir == "" again for un-stamped
// sessions), so the guess is re-derived then — it is never frozen.
func (m *Model) cacheSessionDir(name, dir string) {
	for i := range m.sessions {
		if m.sessions[i].Name == name {
			m.sessions[i].Dir = dir
			return
		}
	}
}

func (m *Model) rebuildSessionList() tea.Cmd {
	filtered := m.filteredSessions()

	// Zero-tags-anywhere gate (spec § Empty states → By Tag with zero tags):
	// when By Tag mode is active but no project carries any tag, degrade to the
	// plain flat list WITH a signpost rather than silently flattening. Build
	// with ToListItems so the rendered list is byte-for-byte the flat list (no
	// Untagged heading — there is nothing to group). The flag is recomputed on
	// every rebuild, so it clears automatically when the mode leaves ByTag or
	// when a tag appears.
	m.byTagSignpost = m.sessionListMode == prefs.ModeByTag && !anyTagsExist(m.projects)

	var items []list.Item
	switch {
	case m.byTagSignpost:
		items = ToListItems(filtered)
	case m.sessionListMode == prefs.ModeByProject:
		items = buildByProject(m.resolveSessionDirs(filtered), m.projectIndex)
	case m.sessionListMode == prefs.ModeByTag:
		items = buildByTag(m.resolveSessionDirs(filtered), m.projectIndex)
	default:
		items = ToListItems(filtered)
	}

	cmd := m.sessionList.SetItems(items)

	// Canonical mode-aware title set. Living here means the s-toggle
	// (handleSwitchViewKey → rebuildSessionList), the SessionsMsg refresh
	// (applySessions → rebuildSessionList), and WithInsideTmux (which now routes
	// item population + title through this core) all get the correct mode title —
	// reconciled with the inside-tmux current-session decoration — for free.
	m.sessionList.Title = sessionListTitleForMode(m.sessionListMode, m.insideTmux, m.currentSession)

	// Re-apply terminal size so pagination accounts for the manual keymap
	// footer height (see applyListSize). Without this re-apply the list
	// would size itself against pre-load defaults and overflow under the
	// footer once items populate.
	if m.termWidth > 0 || m.termHeight > 0 {
		m.applySessionListSize(m.termWidth, m.termHeight)
	}

	// Grouped lists lead with a non-selectable HeaderItem at index 0, so a
	// fresh build (or a refresh whose cursor now points at a header) would
	// leave the selection on a header with no visible cursor. Step it onto the
	// first real session row.
	m.ensureSessionRowSelected()
	return cmd
}

// ensureSessionRowSelected nudges the selection off a non-selectable HeaderItem
// onto the following session row. Grouped lists always lead with a header
// (index 0) and never place two headers adjacently, so a single downward step
// always lands on a session. It is a no-op in Flat mode (no headers), while
// filtering (headers are excluded from the visible set), and when the list is
// empty.
func (m *Model) ensureSessionRowSelected() {
	if _, isHeader := m.sessionList.SelectedItem().(HeaderItem); isHeader {
		m.sessionList.CursorDown()
	}
}

// skipHeaderRow keeps the cursor off the non-selectable group headers after the
// list has processed a navigation key. If the new selection is a HeaderItem it
// steps once more in the direction the user was moving — up for CursorUp /
// PrevPage / GoToStart, down otherwise. A single step always clears the header
// because no two headers are adjacent; at the very top (index 0 header) an
// upward intent flips to a downward step so the selection never falls off the
// list.
func (m *Model) skipHeaderRow(msg tea.KeyMsg) {
	if _, isHeader := m.sessionList.SelectedItem().(HeaderItem); !isHeader {
		return
	}
	km := m.sessionList.KeyMap
	if key.Matches(msg, km.CursorUp, km.PrevPage, km.GoToStart) && m.sessionList.Index() > 0 {
		m.sessionList.CursorUp()
		return
	}
	m.sessionList.CursorDown()
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
		// via=cli: the TUI delete is a user-facing mutation.
		if err := m.projectStore.Remove(path, "cli"); err != nil {
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
		// applySessions → rebuildSessionList sets the mode-aware title (and the
		// inside-tmux current-session decoration), so no title rewrite is needed
		// here.
		cmd := m.applySessions(msg.Sessions)

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
			// Cache the project records so the mode-aware session re-render core
			// (rebuildSessionList) can feed them to the grouping builders
			// without a synchronous store read in the render path.
			m.setProjects(msg.Projects)
			items := ProjectsToListItems(msg.Projects)
			setItemsCmd = m.projectList.SetItems(items)
			// Re-apply terminal size so pagination accounts for the manual
			// keymap footer height (see applyListSize).
			if m.termWidth > 0 || m.termHeight > 0 {
				m.applyProjectListSize(m.termWidth, m.termHeight)
			}
			// Startup-ordering correction: fetchSessions and loadProjects are
			// batched concurrently. If SessionsMsg arrives FIRST, applySessions
			// already ran rebuildSessionList against an EMPTY m.projects, so in a
			// persisted grouped mode every session landed in the Unknown/Untagged
			// catch-all. Now that the real project records are cached, re-group so
			// each session is placed under its project/tag heading. Only when a
			// grouped mode is active AND sessions are already ingested — Flat mode
			// and the no-sessions-yet case keep today's behaviour (no spurious
			// rebuild). The rebuild cmd is BATCHED with setItemsCmd so neither the
			// project-list nor the session-list SetItems command is dropped.
			grouped := m.sessionListMode == prefs.ModeByProject || m.sessionListMode == prefs.ModeByTag
			if grouped && len(m.sessions) > 0 {
				setItemsCmd = tea.Batch(setItemsCmd, (&m).rebuildSessionList())
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
			// Dispatch a mode-aware re-group refresh so tag edits made in the
			// projects modal are visible on return to the sessions page (spec
			// § Assigning & Managing Tags → Refresh contract). Routes through
			// applySessions, so the active grouping mode is respected. Empty
			// preserveName: no preview-anchored cursor on a page switch. The
			// cmd is nil when no SessionLister is wired (test harnesses).
			return m, m.refreshSessionsAfterPreviewCmd("")
		case isRuneKey(msg, "x"):
			if m.commandPending {
				return m, nil
			}
			m.activePage = PageSessions
			// Dispatch a mode-aware re-group refresh so tag edits made in the
			// projects modal are visible on return to the sessions page (spec
			// § Assigning & Managing Tags → Refresh contract). Routes through
			// applySessions, so the active grouping mode is respected. Empty
			// preserveName: no preview-anchored cursor on a page switch. The
			// cmd is nil when no SessionLister is wired (test harnesses).
			return m, m.refreshSessionsAfterPreviewCmd("")
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

	// Seed the tag buffer from a copy of the project's tags so mutating the
	// buffer never aliases the stored project slice. slices.Clone(nil)
	// returns nil, so a back-compat record with no tags seeds an empty buffer.
	m.editTags = slices.Clone(pi.Project.Tags)
	m.editTagsMutated = false
	m.editNewTag = ""
	m.editTagCursor = 0

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
		// Tags are persisted live (Enter adds, x removes), so closing with Esc
		// keeps them — unlike name/aliases, which are batched and discarded
		// here. Reload projects so the cached records + grouping index pick up
		// any live tag change before the By-Tag view is shown.
		if m.editTagsMutated {
			m.editTagsMutated = false
			return m, m.loadProjects()
		}
		return m, nil

	case tea.KeyTab:
		// Three-way forward cycle: Name → Aliases → Tags → wrap to Name.
		switch m.editFocus {
		case editFieldName:
			m.editFocus = editFieldAliases
		case editFieldAliases:
			m.editFocus = editFieldTags
			// Reset the tag cursor to the Add-input row on entry so the
			// highlight starts in a defined, in-bounds position (index 0 is
			// always valid, even with zero tags).
			m.editTagCursor = 0
		default:
			m.editFocus = editFieldName
		}
		return m, nil

	case tea.KeyEnter:
		// Tags field: Enter is field-scoped — it adds the typed tag and
		// persists it immediately (project.AddTag), so the visible "[x] tag"
		// reflects what is on disk and Esc never discards it. A
		// blank/whitespace-only or duplicate-after-normalisation input is a
		// no-op (and never confirms). Name/Aliases fall through to the batched
		// confirm path unchanged.
		if m.editFocus == editFieldTags {
			tag, ok := project.NormaliseTag(m.editNewTag)
			if !ok {
				// Blank/whitespace-only: no append, no confirm.
				return m, nil
			}
			if slices.Contains(m.editTags, tag) {
				// Duplicate after normalisation: no append, no confirm.
				m.editNewTag = ""
				return m, nil
			}
			if m.projectEditor != nil {
				if err := m.projectEditor.AddTag(m.editProject.Path, tag); err != nil {
					m.editError = "Failed to save tag"
					return m, nil
				}
			}
			m.editTags = append(m.editTags, tag)
			m.editNewTag = ""
			m.editError = ""
			m.editTagsMutated = true
			return m, nil
		}
		return m.handleEditProjectConfirm()

	case tea.KeyBackspace:
		if m.editFocus == editFieldName {
			if len(m.editName) > 0 {
				m.editName = m.editName[:len(m.editName)-1]
			}
		} else if m.editFocus == editFieldTags && m.editTagCursor == len(m.editTags) {
			// Tags Add input: trim only the in-progress new-tag text.
			if len(m.editNewTag) > 0 {
				m.editNewTag = m.editNewTag[:len(m.editNewTag)-1]
			}
		} else if m.editFocus == editFieldAliases && m.editAliasCursor == len(m.editAliases) {
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
		if m.editFocus == editFieldTags && m.editTagCursor < len(m.editTags) {
			m.editTagCursor++
		}
		return m, nil

	case tea.KeyUp:
		if m.editFocus == editFieldAliases && m.editAliasCursor > 0 {
			m.editAliasCursor--
		}
		if m.editFocus == editFieldTags && m.editTagCursor > 0 {
			m.editTagCursor--
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
		// In tags area, on an existing tag entry: x removes it and persists the
		// removal immediately (project.RemoveTag), mirroring the live Enter-add.
		if m.editFocus == editFieldTags && text == "x" && m.editTagCursor < len(m.editTags) {
			removed := m.editTags[m.editTagCursor]
			if m.projectEditor != nil {
				if err := m.projectEditor.RemoveTag(m.editProject.Path, removed); err != nil {
					m.editError = "Failed to remove tag"
					return m, nil
				}
			}
			m.editTags = append(m.editTags[:m.editTagCursor], m.editTags[m.editTagCursor+1:]...)
			if m.editTagCursor > len(m.editTags) {
				m.editTagCursor = len(m.editTags)
			}
			m.editError = ""
			m.editTagsMutated = true
			return m, nil
		}
		// In tags area, on Add input: type into new tag
		if m.editFocus == editFieldTags && m.editTagCursor == len(m.editTags) {
			m.editNewTag += text
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
		// via=cli: the TUI rename is a user-facing mutation.
		if err := m.projectEditor.Rename(m.editProject.Path, name, "cli"); err != nil {
			m.editError = "Failed to save project name"
			return m, nil
		}
	}

	// Alias mutations now flow through the audited combined store-seam methods
	// (SetAndSave / DeleteAndSave), each of which persists immediately. This
	// changes the TUI from ONE batched Save to per-op Saves. That is acceptable
	// because (1) the set-noop skip-persist requirement is only expressible by a
	// combined method (a separate Save cannot selectively skip), (2) a TUI alias
	// edit is tiny (0-1 removals plus 0-1 additions), and (3) routing every
	// mutation through the combined methods makes all alias edits — CLI and TUI
	// alike — uniformly audited under the aliases component. via=cli: the TUI
	// edit is a user-facing mutation.

	// Handle alias removals.
	for _, removed := range m.editRemoved {
		if _, err := m.aliasEditor.DeleteAndSave(removed, "cli"); err != nil {
			m.editError = "Failed to save aliases"
			return m, nil
		}
	}

	// Handle new alias addition.
	newAlias := strings.TrimSpace(m.editNewAlias)
	if newAlias != "" {
		// Check for collision before mutating.
		allAliases, err := m.aliasEditor.Load()
		if err == nil {
			if existingPath, ok := allAliases[newAlias]; ok && existingPath != m.editProject.Path {
				m.editError = fmt.Sprintf("Alias '%s' already exists", newAlias)
				return m, nil
			}
		}
		if err := m.aliasEditor.SetAndSave(newAlias, m.editProject.Path, "cli"); err != nil {
			m.editError = "Failed to save aliases"
			return m, nil
		}
	}

	// Tag mutations are NOT handled here: tags persist live as the user edits
	// them (Enter adds via AddTag, x removes via RemoveTag in
	// updateEditProjectModal), so by the time the batched name/alias confirm
	// runs the projects.json tag set is already current. The trailing
	// loadProjects() refreshes the cached records + grouping index for both
	// the name/alias changes and any live tag edits.

	m.modal = modalNone
	m.editError = ""
	m.editTagsMutated = false

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
		// s cycles the session-list grouping mode (Flat → By Project → By Tag
		// → Flat). This case MUST stay inside this rune switch, which sits below
		// the `if m.sessionList.SettingFilter() { break }` guard above — that
		// guard makes s a literal filter character while the / filter input is
		// focused. Do NOT hoist this case above that guard.
		case isRuneKey(msg, "s"):
			return m.handleSwitchViewKey()
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
	// Skip over non-selectable group headers so the cursor only ever rests on
	// a session row (grouped modes only; a no-op in Flat / while filtering).
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		m.skipHeaderRow(keyMsg)
	}
	return m, cmd
}

// handleSwitchViewKey advances the session-list grouping mode one step
// (Flat → By Project → By Tag → Flat), re-renders the list via the mode-aware
// core, and persists the new mode through the injected seam. The cycle is
// unconditional — it fires regardless of session count or tag count.
//
// Persistence is best-effort: a nil persister is skipped, and a non-nil Save
// error is swallowed so a persist failure never aborts the in-memory toggle or
// the re-render.
func (m Model) handleSwitchViewKey() (tea.Model, tea.Cmd) {
	m.sessionListMode = nextSessionListMode(m.sessionListMode)
	// rebuildSessionList is a pointer-receiver method; call it on the local
	// value copy's address so the re-render mutates this copy before return.
	cmd := (&m).rebuildSessionList()
	// Reset to the top of the first page on every view switch: the previous
	// page offset and cursor are meaningless under the new grouping. Land on
	// the first session row, stepping past the leading header in grouped modes.
	m.sessionList.ResetSelected()
	(&m).ensureSessionRowSelected()
	if m.modePersister != nil {
		// Persist exactly once per press. The error is intentionally swallowed:
		// remembering the last mode is a convenience, not a correctness
		// invariant, so a write failure must not abort the toggle.
		_ = m.modePersister.Save(m.sessionListMode)
	}
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
	footer := renderKeymapFooter(&m.projectList, projectFooterBindings(&m.projectList, m.commandPending))
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
	renderEditListField(&b, "Aliases", m.editFocus == editFieldAliases, m.editAliases, m.editAliasCursor, m.editNewAlias)

	b.WriteString("\n")
	renderEditListField(&b, "Tags", m.editFocus == editFieldTags, m.editTags, m.editTagCursor, m.editNewTag)

	if m.editError != "" {
		fmt.Fprintf(&b, "\n  Error: %s\n", m.editError)
	}

	b.WriteString("\n  [Enter] Save  [Esc] Cancel  [Tab] Switch field")

	return b.String()
}

// renderEditListField renders one editable list field (Aliases or Tags) of the
// edit-project modal into b: a focus-indicated "<label>:" heading, the entries
// (each a removable "[x] <entry>" row, or a "(none)" empty state), and a trailing
// "Add: <addInput>" row. The focus indicator and per-row cursor markers are driven
// by focused (whether the field currently has focus) combined with cursor (which
// selects an entry, or the Add row when it equals len(entries)).
func renderEditListField(b *strings.Builder, label string, focused bool, entries []string, cursor int, addInput string) {
	indicator := "  "
	if focused {
		indicator = "> "
	}
	b.WriteString(indicator + label + ":\n")

	if len(entries) == 0 {
		b.WriteString("    (none)\n")
	} else {
		for i, entry := range entries {
			marker := "    "
			if focused && cursor == i {
				marker = "  > "
			}
			fmt.Fprintf(b, "%s[x] %s\n", marker, entry)
		}
	}

	addMarker := "    "
	if focused && cursor == len(entries) {
		addMarker = "  > "
	}
	fmt.Fprintf(b, "%sAdd: %s\n", addMarker, addInput)
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
	if m.byTagSignpost {
		// Persistent "No tags yet" signpost (spec § Empty states → By Tag
		// with zero tags). Inserted additively beneath the title/filter row
		// — mirroring the flash-row insertion below — so the title, list,
		// and footer chrome are unchanged aside from the one inserted row.
		// Gated on byTagSignpost (persistent), NOT flashText (transient).
		listView = insertRowBelowTitle(listView, renderByTagSignpostRow())
	}
	if m.flashText != "" {
		// Split off the first line (title / filter input row) and insert
		// the flash row between it and the remainder. Using a manual split
		// keeps the existing list view byte-identical aside from the
		// inserted row, satisfying "no existing chrome replaced or
		// overlaid".
		listView = insertRowBelowTitle(listView, m.renderFlashRow())
	}
	footer := renderKeymapFooter(&m.sessionList, sessionFooterBindings(&m.sessionList))
	return lipgloss.JoinVertical(lipgloss.Left, listView, footer)
}

// insertRowBelowTitle inserts row between the first line (the list's
// title / filter input row) and the remainder of listView, preserving the
// rest byte-for-byte. A single-line (degenerate) listView appends the row
// below instead. Shared by the persistent By-Tag signpost and the transient
// flash so both use the identical additive-insertion mechanic.
func insertRowBelowTitle(listView, row string) string {
	idx := strings.IndexByte(listView, '\n')
	if idx < 0 {
		return listView + "\n" + row
	}
	return listView[:idx+1] + row + "\n" + listView[idx+1:]
}

// byTagSignpostStyle is the dimmed style for the persistent By-Tag "No tags
// yet" signpost row. Kept SEPARATE from flashRowStyle (the transient flash):
// the two rows have distinct lifecycles and the signpost must remain visually
// distinct from a transient flash. Package-level + immutable (lipgloss value
// semantics prevent mutation bleed across renders).
var byTagSignpostStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)

// byTagSignpostText is the exact, persistent signpost wording rendered in By
// Tag mode when no project carries any tag (spec § Empty states → By Tag with
// zero tags). It states the empty condition ("No tags yet") and points the
// user at where to add tags (the projects page). Placement: a dimmed row
// inserted directly beneath the title/filter row, above the (plain flat)
// session list.
const byTagSignpostText = "No tags yet — add tags on the projects page"

// renderByTagSignpostRow returns the styled persistent signpost row.
func renderByTagSignpostRow() string {
	return byTagSignpostStyle.Render(byTagSignpostText)
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
