// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
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
	projectIndex    project.Index
	sessionListMode prefs.SessionListMode
	modePersister   ModePersister
	// appearance is the persisted colour-scheme preference read once at TUI
	// construction (WithAppearance). The model only stores it here; honouring it
	// (skip detection + first-paint wait) is a later task. AppearanceAuto is the
	// zero-value default, so an omitted option leaves the model in auto.
	appearance prefs.Appearance
	// canvasMode is the RESOLVED light/dark appearance the owned canvas (§1) is
	// painted for — distinct from appearance (the pref). The leaf row/footer
	// styles take their Background(canvas) from it and the outer full-terminal
	// fill in View() sources its whitespace background from it, so both layers
	// always agree. theme.Dark is the zero value (the §2.6 no-answer fallback),
	// so an unconfigured model paints the dark canvas. It is the painted mirror
	// of gate.mode: every resolution (OSC 11 reply, timeout, or pin) syncs it via
	// applyResolvedMode, so the existing render path (View / applyCanvasMode)
	// keeps reading a single mode field.
	canvasMode theme.Mode
	// gate is the §2.6 detect-or-timeout first-paint mechanism and the SINGLE
	// source of truth for whether the real canvas may paint (modeResolved()
	// reads it). In auto mode Build opens its detect-or-timeout window via arm()
	// and it resolves on whichever of the OSC 11 BackgroundColorMsg or the
	// appearanceTimeoutMsg fires first; a pinned appearance (light/dark) and a
	// directly constructed model are already resolved (the zero-value gate is
	// resolved), so detection and the wait are skipped. canvasMode mirrors
	// gate.mode for the render path (see syncResolvedMode).
	gate appearanceGate
	// colourless is the SINGLE NO_COLOR carve-out flag (§2.5). It is set once at
	// construction from Deps.NoColor (the cmd layer reads os.Getenv("NO_COLOR");
	// internal/tui stays env-free) and is the one flag EVERY canvas-dependent
	// surface inherits — later phases (modal blank-screen, notice bands, preview
	// chrome) read THIS flag rather than re-deriving NO_COLOR. When set, Portal
	// paints no canvas at all: the leaf styles drop Background(canvas), the outer
	// fill (fillCanvas) becomes a no-op-background pass-through, View sets no OSC 11
	// BackgroundColor, and detection + its first-paint wait are skipped (the gate is
	// constructed colourless). Foreground hue is stripped FREE by the Bubble Tea v2
	// writer layer (colorprofile.Detect honours NO_COLOR), so state stays
	// glyph-distinct (§2.2: ● attached, ▌ selector, spaced headers) + bold/dim.
	colourless        bool
	selected          string
	sessionLister     SessionLister
	sessionKiller     SessionKiller
	sessionRenamer    SessionRenamer
	projectStore      ProjectStore
	projectEditor     ProjectEditor
	aliasEditor       AliasEditor
	sessionCreator    SessionCreator
	cwd               string
	activePage        page
	projectList       list.Model
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

	// originalBg is the terminal's ACTUAL background colour as reported by the
	// OSC 11 query (tea.RequestBackgroundColor) issued from Init, captured for
	// RESTORE-ON-EXIT only. It is a hex string like "#1e1e2e" (empty if no
	// response ever arrives). Distinct from canvasMode (Portal's CHOSEN canvas):
	// this is what the terminal looked like before Portal painted, so the launch
	// sites can SET it back on exit (OSC 11 set) — terminals that ignore the
	// OSC 111 reset (mosh/Blink) still honour the set, so the canvas colour does
	// not stick after Portal quits.
	//
	// Capture is ASYNC and NON-GATING: the first paint never waits on this. The
	// detect-or-timeout first-paint gate and the auto-appearance resolution that
	// consume this captured value are a later task (1-7) — only the fire-and-
	// forget capture lives here.
	originalBg string

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

// OriginalBackground returns the terminal's original background colour as
// captured via the OSC 11 query (a hex string like "#1e1e2e"), or empty if no
// response ever arrived. The program-launch sites read it after p.Run() returns
// and, when non-empty, SET it back on exit via RestoreTerminalBackground so the
// owned canvas does not stick on terminals that ignore the OSC 111 reset.
func (m Model) OriginalBackground() string {
	return m.originalBg
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

// WithAppearance sets the persisted colour-scheme preference (auto/light/dark) the
// model opens with. Production wiring reads it from prefs.json (via cmd/open.go's
// loadPrefsStore + Store.LoadAppearance, tolerant to AppearanceAuto) and injects it
// here, sibling to WithInitialMode. The model only STORES the value at this point;
// honouring it (skip detection + first-paint wait) is a later task. AppearanceAuto
// is the zero-value default, so omitting the option leaves the model in auto.
func WithAppearance(appearance prefs.Appearance) Option {
	return func(m *Model) {
		m.appearance = appearance
	}
}

// WithCanvasMode is the test/capture-only DIRECT override of the resolved canvas
// mode (§1): it pins canvasMode AND marks the appearance gate already resolved,
// so the model paints that exact canvas from frame one with no OSC 11 detection
// and no first-paint wait. It exists so tests and the offline capture harness can
// render a deterministic mode without driving the async detection race.
//
// PRODUCTION never uses this seam — cmd/open.go drives the mode through the
// appearance pref + OSC 11 detection (the appearance gate). When used, it must
// not be combined with a non-auto appearance (the pin would win the gate
// re-init in New); both light and dark canvases are owned-canvas paths, so a
// direct override and an appearance pin are mutually exclusive ways to land the
// same resolved mode.
func WithCanvasMode(mode theme.Mode) Option {
	return func(m *Model) {
		m.canvasMode = mode
		// pinned=true (and pending=false, the zero value) so the gate is resolved
		// and New's gate-init guard preserves this direct override instead of
		// rebuilding an auto gate over it.
		m.gate = appearanceGate{mode: mode, pinned: true}
	}
}

// WithColourless sets the NO_COLOR carve-out flag (§2.5). The cmd layer detects
// NO_COLOR (env var present and non-empty, the no-color.org convention) and
// injects the decision here via Build → Deps.NoColor, so internal/tui stays
// env-free. When set, the model paints no canvas at all and skips light/dark
// detection + the first-paint wait — there is no canvas to select. It is the
// single inheritable flag every canvas-dependent surface reads (rather than each
// re-deriving NO_COLOR). New consumes it after the options apply to construct the
// colourless gate and re-point the leaf styles colourless.
func WithColourless(colourless bool) Option {
	return func(m *Model) {
		m.colourless = colourless
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
//
// DEPRECATED for the keymap source of truth: the Sessions keymap descriptor
// (sessionsKeymap) is now authoritative for the footer (task 2-4) and the ?
// help modal (Phase 3, §8.5). These entries remain only to keep the legacy
// three-column footer plumbing compiling until 2-4 retires it; the §12.2
// revision is reflected here too — the former p/x "projects" binding is now the
// single x toggle (the dropped p alias no longer appears in any displayed
// label).
func sessionHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "attach")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rename")),
		key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "projects")),
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

// brightenHelpStyles re-points the footer keymap colours onto the §2.9 role
// tokens (colour source only — the structural footer restyle is Phase 2):
// key glyphs use the footer key-hint role (accent.blue), labels the detail
// role (text.detail), and the inter-entry separator / ellipsis is decorative
// chrome (text.faint — decorative-only, never functional text).
func brightenHelpStyles(l *list.Model) {
	keyColor := theme.MV.AccentBlue.Color()
	descColor := theme.MV.TextDetail.Color()
	sepColor := theme.MV.TextFaint.Color()
	l.Help.Styles.ShortKey = lipgloss.NewStyle().Foreground(keyColor)
	l.Help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(descColor)
	l.Help.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(sepColor)
	l.Help.Styles.FullKey = lipgloss.NewStyle().Foreground(keyColor)
	l.Help.Styles.FullDesc = lipgloss.NewStyle().Foreground(descColor)
	l.Help.Styles.FullSeparator = lipgloss.NewStyle().Foreground(sepColor)
	l.Help.Styles.Ellipsis = lipgloss.NewStyle().Foreground(sepColor)
}

// canvasHelpStyles re-points the footer keymap help styles onto the §2.9 role
// tokens AND paints each through Background(canvas) for the resolved mode, so the
// footer (a leaf surface of the foundation Sessions screen, §1) renders on the
// owned canvas. It mirrors brightenHelpStyles' colour roles exactly — only the
// canvas background and the mode-resolved foregrounds differ — and additionally
// backgrounds the HelpStyle wrapper so the footer's own padding cells are canvas
// too. The outer fill in View() pads the line-ends; this fills the cells behind
// the footer glyphs and their inter-glyph spacers.
func canvasHelpStyles(l *list.Model, mode theme.Mode) {
	canvas := theme.MV.Canvas.ColorFor(mode)
	keyColor := theme.MV.AccentBlue.ColorFor(mode)
	descColor := theme.MV.TextDetail.ColorFor(mode)
	sepColor := theme.MV.TextFaint.ColorFor(mode)
	onCanvas := func(fg color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Background(canvas)
	}
	l.Help.Styles.ShortKey = onCanvas(keyColor)
	l.Help.Styles.ShortDesc = onCanvas(descColor)
	l.Help.Styles.ShortSeparator = onCanvas(sepColor)
	l.Help.Styles.FullKey = onCanvas(keyColor)
	l.Help.Styles.FullDesc = onCanvas(descColor)
	l.Help.Styles.FullSeparator = onCanvas(sepColor)
	l.Help.Styles.Ellipsis = onCanvas(sepColor)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Background(canvas)
}

// colourlessHelpStyles strips the footer keymap help styles to bare styles for
// the NO_COLOR carve-out (§2.5): no canvas background and no foreground hue, so
// the footer renders on the terminal's native fg/bg. The footer keys stay
// glyph-legible by construction (§2.2 — the key glyphs and labels are the text
// itself, not colour-only state). Foreground hue would be stripped by the writer
// layer anyway; setting bare styles also drops the canvas background lipgloss
// would otherwise still emit.
func colourlessHelpStyles(l *list.Model) {
	bare := lipgloss.NewStyle()
	l.Help.Styles.ShortKey = bare
	l.Help.Styles.ShortDesc = bare
	l.Help.Styles.ShortSeparator = bare
	l.Help.Styles.FullKey = bare
	l.Help.Styles.FullDesc = bare
	l.Help.Styles.FullSeparator = bare
	l.Help.Styles.Ellipsis = bare
	l.Styles.HelpStyle = l.Styles.HelpStyle.UnsetBackground()
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
	pinArrowOnlyNav(&l.KeyMap)
	l.InfiniteScrolling = true
	brightenHelpStyles(&l)
	return l
}

// pinArrowOnlyNav rebinds the bubbles/list nav KeyMap to the §12.2 keymap
// revision: navigation is ↑/↓ only and paging is Ctrl+↑/↓ only. The v2
// DefaultKeyMap re-introduces the vim aliases (h/j/k/l, g/G) and the
// PgUp/PgDn/Home/End/b/u/f/d page-jump keys this codebase deliberately drops;
// it also binds k to CursorUp, which would shadow the Sessions k=kill verb.
// Rebinding here (rather than at the dispatch layer) keeps the §12.2 "arrows
// only" rule in the single place the list itself consults, so a banned key
// never reaches the list's own Update. GoToStart/GoToEnd are emptied (no g/G,
// no Home/End) — the cursor-skip logic in skipHeaderRow still key.Matches
// against the rebound bindings, so an empty binding simply never matches.
func pinArrowOnlyNav(km *list.KeyMap) {
	km.CursorUp.SetKeys("up")
	km.CursorDown.SetKeys("down")
	km.PrevPage.SetKeys("ctrl+up")
	km.NextPage.SetKeys("ctrl+down")
	km.GoToStart.SetKeys()
	km.GoToEnd.SetKeys()
}

// projectHelpKeys returns key.Binding entries for the projects page.
func projectHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "new session")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s/x", "sessions")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new in cwd")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

// commandPendingHelpKeys returns key.Binding entries for command-pending mode.
// Only enter (run here), n, /, and q are shown; s, x, e, and d are omitted.
func commandPendingHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "run here")),
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
	// Initialise the §2.6 detect-or-timeout gate from the appearance pref now that
	// WithAppearance has run. A pinned light/dark appearance resolves the canvas
	// mode immediately (paint from frame one, no detection, no wait); auto is
	// constructed RESOLVED to the dark fallback so a directly constructed model
	// (tests, non-program callers) paints immediately. PRODUCTION opens the
	// detect-or-timeout window explicitly via Build → arm (only for auto), so the
	// live picker gates the first paint while a direct New(...) does not.
	//
	// The NO_COLOR carve-out (§2.5) wins over both the appearance pin and the auto
	// gate: under NO_COLOR there is no canvas to select, so the gate is colourless
	// (already resolved, unarmable) and detection + its first-paint wait are
	// skipped. Checked first so the WithColourless option short-circuits the
	// appearance-driven gate construction below.
	if m.colourless {
		m.gate = newColourlessGate()
	} else if m.appearance != prefs.AppearanceAuto || !m.gate.pinned {
		// WithCanvasMode is a test/capture-only DIRECT override: when it was applied
		// in the options loop it set its own resolved gate AND left appearance at the
		// auto zero value, so reconstructing the gate from auto would discard that
		// override. Guard against that — only (re)build the gate from appearance when
		// WithCanvasMode did NOT pin a mode (the common path), or when an explicit
		// non-auto appearance was passed (a pin must win over a stray default gate).
		m.gate = newAppearanceGate(m.appearance)
	}
	m.syncResolvedMode()
	return m
}

// armAppearanceDetection opens the §2.6 detect-or-timeout first-paint window on
// an auto gate (a no-op when the appearance is pinned or a WithCanvasMode capture
// override is in force). It is the production entry point — Build calls it so the
// live picker holds the neutral blank frame until OSC 11 detection or the timeout
// resolves the mode. A directly constructed model (tests, non-program callers)
// never calls this, so it paints immediately. The method re-syncs the painted
// fields so View observes the now-unresolved state.
func (m *Model) armAppearanceDetection() {
	m.gate.arm()
	m.syncResolvedMode()
}

// modeResolved reports whether the §2.6 first-paint gate has resolved — the
// single read View uses to decide between the neutral blank frame and the real
// canvas. It delegates to the gate so there is no duplicated flag to keep in
// sync: a zero-value gate (a directly constructed test model) is resolved, an
// armed auto gate is unresolved until OSC 11 or the timeout fires.
func (m Model) modeResolved() bool {
	return m.gate.resolved()
}

// syncResolvedMode mirrors the gate's resolved mode onto the model's painted
// canvasMode and re-applies the leaf canvas styles. It is called after every gate
// transition (arm, OSC 11 reply, timeout, or pin) so the existing render path
// keeps reading m.canvasMode while the gate owns the single-resolution race. It
// is a no-op for the leaf styles when the mode is unchanged, but always cheap.
func (m *Model) syncResolvedMode() {
	m.canvasMode = m.gate.mode
	m.applyCanvasMode()
}

// applyCanvasMode re-points the foundation Sessions screen's leaf styles at the
// model's resolved canvasMode: the session row delegate paints every run through
// Background(canvas) (SessionDelegate.Mode), and the keymap footer's help styles
// carry the same canvas background. newSessionList builds with the Dark default
// before the WithCanvasMode option is known; this runs once in New after the
// options apply, so the first frame paints the correct canvas. Scope is the
// foundation Sessions screen (per the canvas task); the projects screen's leaf
// restyle is a later phase, and the outer fill in View() paints around it.
func (m *Model) applyCanvasMode() {
	// NO_COLOR carve-out (§2.5): paint no canvas at all. The delegate drops its
	// Background(canvas) leaf paint (Colourless), the footer help styles drop their
	// canvas background and foreground hue, and the title bar carries no canvas
	// background — every cell renders on the terminal's native bg. Foreground hue
	// is stripped FREE by the writer layer (colorprofile honours NO_COLOR), so
	// state stays glyph-distinct (§2.2).
	if m.colourless {
		m.sessionList.SetDelegate(SessionDelegate{Mode: m.canvasMode, Colourless: true})
		colourlessHelpStyles(&m.sessionList)
		m.sessionList.Styles.TitleBar = m.sessionList.Styles.TitleBar.UnsetBackground()
		// Strip the bubbles/list default Title box colours (its violet 48;5;62
		// background + bright foreground) so "Sessions" renders on the terminal's
		// native fg/bg — the title is a leaf canvas-dependent surface too. The
		// coloured path leaves this default box untouched (the wordmark/header chrome
		// restyle is Phase 2); under NO_COLOR it must carry no background SGR.
		m.sessionList.Styles.Title = m.sessionList.Styles.Title.UnsetBackground().UnsetForeground()
		return
	}
	m.sessionList.SetDelegate(SessionDelegate{Mode: m.canvasMode})
	canvasHelpStyles(&m.sessionList, m.canvasMode)
	// Background the title bar so its leading left-pad cells (bubbles/list's
	// TitleBar PaddingLeft) are canvas rather than the terminal background. The
	// "Sessions" title box keeps its own colour (the wordmark/header chrome
	// restyle is Phase 2); this only paints the padding around it.
	canvas := theme.MV.Canvas.ColorFor(m.canvasMode)
	m.sessionList.Styles.TitleBar = m.sessionList.Styles.TitleBar.Background(canvas)
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
		// The zero-value gate is already resolved to the dark canvas (pending is
		// false by default), so this directly constructed test model paints
		// immediately — the detect-or-timeout first-paint window is opened only by
		// the production Build path / explicit arm. Left implicit here; documented
		// so a reader knows the blank-frame gate does not apply to struct-literal
		// test models.
	}
	// Apply construction-time fallback dimensions through the size helper
	// so each list reserves room for the manual keymap footer at every
	// SetSize call site (including this 80x24 test seed). The dims come from
	// the inset content-region helpers (zero termWidth/Height → 80x24 fallback,
	// then the §3 gutter folded in) so the seed agrees with the live budget.
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
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
//
// It also subtracts the height of any active notice band (the §11.2 flash row
// and/or the By-Tag "No tags yet" signpost) inserted beneath the title by
// viewSessionList, so the composed Sessions view stays within termH. This is the
// "list height recompute underneath the fill" of §1: when a band appears or
// clears the list reserves one fewer / one more row and the outer canvas fill
// simply re-pads to termH — the one-row-per-delegate pagination invariant (§3.5
// / §4.1) holds and the frame never overflows the viewport.
func (m *Model) applySessionListSize(width, height int) {
	// Fold the §3.1 header block height out of the budget (in addition to the
	// notice bands) so bubbles/list paginates against the reduced height: the
	// header is part of the height budget, NOT an uncounted band (§3.5). The header
	// height is resolved against the SAME width this size-apply was called with, so
	// the budget and the viewSessionList render agree at every call site
	// (WindowSizeMsg, rebuildSessionList, the 80x24 construction seed).
	reserved := m.sessionBandHeight() + m.headerHeight(width)
	m.applyListSize(&m.sessionList, sessionFooterBindings(&m.sessionList), width, height-reserved)
}

// sessionBandHeight returns the total rendered height of the notice bands
// viewSessionList inserts beneath the title — the persistent By-Tag signpost and
// the transient inline flash. It is the amount applySessionListSize reserves out
// of the list's height budget so the bands never push the composed view past
// termH. Both are at most one row each in v1, but the height is measured (not
// hardcoded) so a future multi-line band stays correct.
func (m *Model) sessionBandHeight() int {
	h := 0
	if m.byTagSignpost {
		h += lipgloss.Height(renderByTagSignpostRow())
	}
	if m.flashText != "" {
		h += lipgloss.Height(m.renderFlashRow())
	}
	return h
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
		m.applySessionListSize(m.contentWidth(), m.contentHeight())
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
func (m *Model) skipHeaderRow(msg tea.KeyPressMsg) {
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
// bails). setFlash("") still bumps the generation; the caller decides what
// counts as a flash.
//
// It also re-syncs the session list layout (resyncSessionLayout) so the list
// reserves a row for the inserted flash band and the composed view stays within
// termH — the §1 "list height recompute underneath the fill". The re-sync is a
// no-op until the first WindowSizeMsg sets the dimensions, so the gen/text state
// primitive remains observable in isolation on a zero-size model.
func (m *Model) setFlash(text string) {
	m.flashGen++
	m.flashText = text
	m.resyncSessionLayout()
}

// clearFlash zeros flashText, leaving flashGen untouched. Idempotent: a
// clear of an already-cleared state is a no-op observable to callers.
// flashGen is preserved so any in-flight ticks scheduled before the clear
// continue to compare against the same monotonic sequence. Like setFlash it
// re-syncs the session list layout so the row reserved for the (now cleared)
// band is returned to the list under the fill.
func (m *Model) clearFlash() {
	m.flashText = ""
	m.resyncSessionLayout()
}

// resyncSessionLayout re-applies the session list size for the current notice
// band state, so a band appearing or clearing recomputes the list height
// underneath the outer canvas fill (§1). It no-ops before the first
// WindowSizeMsg (dimensions still zero), keeping the flash state primitives
// observable on a bare Model{} in unit tests.
func (m *Model) resyncSessionLayout() {
	if m.termWidth <= 0 || m.termHeight <= 0 {
		return
	}
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
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
	// Capture the terminal's original background for RESTORE-ON-EXIT (§
	// background restore-on-exit). tea.RequestBackgroundColor is itself a tea.Cmd
	// (func() tea.Msg) that emits the OSC 11 query ESC]11;? — its
	// tea.BackgroundColorMsg response is stored in Update. This is ASYNC and
	// NON-GATING: it is batched alongside whatever Init already returns, never
	// gating the first paint, and a missing response simply leaves originalBg
	// empty. It is distinct from canvasMode (Portal's chosen canvas); 1-7 will
	// later consume the captured value for auto-appearance resolution.
	requestBg := tea.Cmd(tea.RequestBackgroundColor)

	// NO_COLOR carve-out (§2.5 / §2.6): Portal paints no canvas, so there is no
	// OSC 11 set to undo on exit and no canvas to detect — skip the background
	// query entirely. The detect-or-timeout tick is already suppressed (the
	// colourless gate is resolved, so timeoutCmd returns nil), but nil-ing
	// requestBg too means colourless issues NO OSC 11 query at all.
	if m.colourless {
		requestBg = nil
	}

	// Arm the §2.6 detect-or-timeout deadline. The gate returns nil for a pinned
	// (already-resolved) appearance — the pin path skips the wait entirely — and
	// an appearanceTimeoutMsg tick for an unresolved auto gate so a non-responding
	// terminal still resolves to the dark fallback. Batched alongside the OSC 11
	// query so the two race; whichever fires first resolves the mode (Update),
	// the loser is ignored (no flip). A nil cmd is harmless inside tea.Batch.
	detectTimeout := m.gate.timeoutCmd()

	if m.commandPending {
		return tea.Batch(requestBg, detectTimeout, m.loadProjects())
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
		cmds := []tea.Cmd{requestBg, detectTimeout, fetchSessions, loadingPadTick, bootstrapCompleteCmd}
		if loadProjects != nil {
			cmds = append(cmds, loadProjects)
		}
		return tea.Batch(cmds...)
	}

	if loadProjects != nil {
		return tea.Batch(requestBg, detectTimeout, fetchSessions, loadProjects)
	}
	return tea.Batch(requestBg, detectTimeout, fetchSessions)
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward WindowSizeMsg to both page lists so they have correct dimensions
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = wsm.Width
		m.termHeight = wsm.Height
		// Fold the §3 content gutter into both budgets: the lists size to the INSET
		// content region (contentWidth × contentHeight), not the full terminal, so
		// the composed view fits inside the global gutter.
		m.applySessionListSize(m.contentWidth(), m.contentHeight())
		m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	}

	// Handle cross-view messages regardless of view state
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		// Two independent jobs ride this one message:
		//
		// 1. Capture the terminal's ORIGINAL background for restore-on-exit. The
		//    query was issued from Init; this stores the hex form so the launch
		//    sites can SET it back on quit (terminals that ignore the OSC 111
		//    reset still honour the set). The nil guard is required —
		//    BackgroundColorMsg.String() panics on a nil Color (a no-answer),
		//    which leaves originalBg empty.
		//
		// 2. Resolve the §2.6 appearance gate in AUTO mode. msg.IsDark() is
		//    nil-safe (nil → dark), so a no-answer-shaped reply collapses to the
		//    dark fallback. resolveFromDark is the single-resolution core: it is a
		//    no-op once the gate already resolved (a pinned appearance, or the
		//    timeout already won the race), so a late OSC 11 reply never flips the
		//    painted canvas. COLORFGBG is deliberately NOT consulted here — OSC 11
		//    is authoritative; the weak COLORFGBG hint must never override it.
		if msg.Color != nil {
			m.originalBg = msg.String()
		}
		if m.gate.resolveFromDark(msg.IsDark()) {
			m.syncResolvedMode()
		}
		return m, nil
	case appearanceTimeoutMsg:
		// The detect-or-timeout deadline fired. If the OSC 11 reply has not yet
		// resolved the gate, fall through to the dark fallback (§2.6). resolveDark
		// is a no-op once already resolved, so a timeout that lost the race (the
		// reply arrived first) never re-resolves — no second resolution, no flip.
		if m.gate.resolveDark() {
			m.syncResolvedMode()
		}
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
			// keymap footer height (see applyListSize), sized to the inset
			// content region (the §3 gutter folded into the budget).
			if m.termWidth > 0 || m.termHeight > 0 {
				m.applyProjectListSize(m.contentWidth(), m.contentHeight())
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
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyIsCtrlC(keyMsg) {
			return m, tea.Quit
		}
		return m, nil
	case PageProjects:
		return m.updateProjectsPage(msg)
	case pagePreview:
		// Forward to the preview with the §3 gutter folded in: a WindowSizeMsg is
		// rewritten to the inset content-region dims so the preview's framed chrome
		// resizes to sit inside the global gutter (m.termWidth/Height were already
		// updated from the raw msg at the top of Update).
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(insetWindowSizeMsg(msg, m.contentWidth(), m.contentHeight()))
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
	case tea.KeyPressMsg:
		if keyIsCtrlC(msg) {
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
		case keyIsCode(msg, tea.KeyEscape):
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
		case keyIsCode(msg, tea.KeyEnter):
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
	keyMsg, ok := msg.(tea.KeyPressMsg)
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
		keyIsCode(keyMsg, tea.KeyEscape):
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
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Code {
	case tea.KeyEscape:
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

	default:
		// Printable-rune key press. In v1 this was the tea.KeyRunes case; in
		// v2 a printable key carries its rune in Code (so it falls through the
		// named-key cases to default) and the characters in Text. Guard on a
		// non-empty Text with no modifiers so only real printable input lands
		// here — the exact v1 `string(msg.Runes)` semantics.
		if keyMsg.Mod != 0 || keyMsg.Text == "" {
			return m, nil
		}
		text := keyMsg.Text
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
	case tea.KeyPressMsg:
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
		if keyIsCtrlC(msg) {
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
		if keyIsCode(msg, tea.KeySpace) {
			if len(m.sessionList.Items()) == 0 {
				return m, nil
			}
			si, ok := m.selectedSessionItem()
			if !ok {
				return m, nil
			}
			// Size the preview to the INSET content region (the §3 gutter folded
			// into the budget) so its framed chrome sits inside the global gutter
			// like every other page.
			pmodel, ok := NewPreviewModel(si.Session.Name, m.enumerator, m.reader, m.previewAttacher, m.contentWidth(), m.contentHeight())
			if !ok {
				return m, nil
			}
			m.preview = pmodel
			m.activePage = pagePreview
			return m, nil
		}
		switch {
		case keyIsCode(msg, tea.KeyEscape):
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
		// x is the sole Sessions↔Projects toggle (§12.2). The former p alias
		// (Sessions → Projects) is dropped so each key has a single meaning.
		case isRuneKey(msg, "x"):
			m.activePage = PageProjects
			return m, nil
		case keyIsCode(msg, tea.KeyEnter):
			return m.handleSessionListEnter()
		}
	}

	// Delegate remaining key handling to the list (cursor navigation, filtering, etc.)
	var cmd tea.Cmd
	m.sessionList, cmd = m.sessionList.Update(msg)
	// Skip over non-selectable group headers so the cursor only ever rests on
	// a session row (grouped modes only; a no-op in Flat / while filtering).
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
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
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyIsCtrlC(keyMsg) {
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
	keyMsg, ok := msg.(tea.KeyPressMsg)
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
		keyIsCode(keyMsg, tea.KeyEscape):
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
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if ok {
		switch keyMsg.Code {
		case tea.KeyEnter:
			newName := strings.TrimSpace(m.renameInput.Value())
			if newName == "" {
				return m, nil
			}
			oldName := m.renameTarget
			m.modal = modalNone
			m.renameTarget = ""
			return m, m.renameAndRefresh(oldName, newName)
		case tea.KeyEscape:
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

// View renders the current view. In Bubble Tea v2 View returns a tea.View
// struct rather than a string; the rendered content is produced by viewString
// and the AltScreen field is set declaratively here (v2 dropped the
// tea.WithAltScreen program option / tea.EnterAltScreen command — alt-screen
// is now a View field, matching the prior cmd/open.go and capturetool launch
// behaviour). Centralising the wrap here keeps every page composer
// (viewLoading / viewProjectList / viewSessionList / preview.View) a plain
// string builder, so the render logic is unchanged from v1 (parity).
func (m Model) View() tea.View {
	// §2.6 / §10.2 first-paint gate: in auto mode, hold the real canvas/content
	// paint until the appearance gate resolves (the OSC 11 reply or the timeout).
	// Painting the real canvas before the mode is decided would risk a visible
	// flip — a defect. The wait is tens of ms (appearanceDetectTimeout), invisible
	// against the multi-hundred-ms bootstrap, so the user never sees the blank.
	// A pinned appearance constructs the gate already resolved, so this branch is
	// never taken for light/dark pins — they paint from frame one.
	//
	// Scope: the gate holds the owned-canvas pages (the foundation Sessions
	// screen and the views composed over fillCanvas). The cold-path loading page
	// (§10) keeps its current un-gated render here — Phase 5 reworks it to gate on
	// this same mechanism (the reusable appearanceGate). Excluding it now keeps
	// the loading-page behaviour unchanged while the gate is wired into Sessions.
	if !m.modeResolved() && m.activePage != PageLoading {
		v := tea.NewView(m.blankFrame())
		v.AltScreen = true
		return v
	}
	v := tea.NewView(m.fillCanvas(m.viewString()))
	v.AltScreen = true
	// NO_COLOR carve-out (§2.5): Portal imposes no hues, so it does NOT set the
	// screen background (OSC 11) — the terminal keeps its native bg. fillCanvas is
	// already a no-op-background pass-through under colourless, so leaving
	// BackgroundColor nil completes the canvas suppression (both layers).
	if m.colourless {
		return v
	}
	// Gutter aid for the owned canvas (§1): set the screen's background colour
	// (OSC 11) to the mode-matched canvas so the terminal's padding/gutter OUTSIDE
	// the rendered grid reads on the canvas too, on terminals that honour it
	// (Ghostty). In-grid correctness no longer DEPENDS on this — fillCanvas's
	// per-line backfill now paints every interior cell with an explicit canvas
	// SGR, so mosh/Blink (which ignore OSC 11) no longer bleed the terminal theme
	// through mid-line gaps. This stays as a belt-and-braces gutter fill only.
	v.BackgroundColor = theme.MV.Canvas.ColorFor(m.canvasMode)
	return v
}

// blankFrame is the neutral pre-resolution frame held by the §2.6 first-paint
// gate while the appearance is still being detected. It paints NO canvas
// background — the light/dark mode is undecided, so committing to either canvas
// here is exactly the flip the gate exists to avoid. It is a FULL-terminal block
// of plain spaces (no SGR, no content inset — there is no content to inset yet),
// sized to the cached terminal dimensions with the same 80x24 zero-size fallback
// as fillCanvas, so the terminal shows an empty screen (the user's own
// background) for the tens-of-ms wait rather than a half-painted canvas. The wait
// is invisible against the multi-hundred-ms bootstrap.
func (m Model) blankFrame() string {
	w, h := m.termDims()
	blank := strings.Repeat(" ", w)
	lines := make([]string, h)
	for i := range lines {
		lines[i] = blank
	}
	return strings.Join(lines, "\n")
}

// Hinset / Vinset are the global content gutter (§3 frame padding): every page
// composes into a region inset by Hinset cells left/right and Vinset rows
// top/bottom, with the gutter cells painted the owned canvas (§1) — the
// terminal-grid analogue of the Paper design's whole-frame container padding
// (§15.1: paddingInline 34px L/R, paddingBlock 30px T/B). At the design's ~9px
// char width / ~18px line-height that is ≈ 2 cells L/R and ≈ 1 row T/B. The inset
// is folded into the width AND height budgets at every SetSize site so the
// one-row-per-delegate pagination invariant (§3.5) stays exact — it is part of
// the budget, NOT an uncounted band.
const (
	Hinset = 2 // content gutter, cells left and right (≈ 34px design paddingInline)
	Vinset = 1 // content gutter, rows top and bottom  (≈ 30px design paddingBlock)
)

// fallbackTermWidth / fallbackTermHeight are the pre-first-WindowSizeMsg
// dimensions: when the cached terminal size is still zero/unset the canvas (and
// the content region derived from it) falls back to 80×24, matching viewLoading,
// so the frame never sizes to zero and blanks the screen.
const (
	fallbackTermWidth  = 80
	fallbackTermHeight = 24
)

// termDims returns the cached terminal dimensions with the zero/unset 80×24
// fallback applied — the full-canvas size the §1 outer fill paints.
func (m Model) termDims() (w, h int) {
	w, h = m.termWidth, m.termHeight
	if w <= 0 {
		w = fallbackTermWidth
	}
	if h <= 0 {
		h = fallbackTermHeight
	}
	return w, h
}

// insetRegion reduces a full dimension by 2× the inset, clamping the inset to 0
// when the dimension cannot hold it (region would be ≤ 0). It is the single
// content-region derivation so the budget computation and the fillCanvas
// placement agree exactly, and so a tiny terminal degrades to a 0 inset rather
// than producing a negative region or overflowing.
func insetRegion(dim, inset int) int {
	if dim <= 2*inset {
		return dim // clamp the inset to 0: keep the full dimension, no negative region
	}
	return dim - 2*inset
}

// contentWidth is the inset content-region width every page composes into:
// (termW − 2·Hinset), with the 80-col zero/unset fallback applied first and the
// inset clamped to 0 on a too-narrow terminal. It is the width fed to every
// SetSize site and content composer (header, list, footer, loading text,
// preview) so the composed view fits inside the L/R gutter.
func (m Model) contentWidth() int {
	w, _ := m.termDims()
	return insetRegion(w, Hinset)
}

// insetWindowSizeMsg rewrites a tea.WindowSizeMsg's dimensions to the inset
// content-region size (contentW × contentH) when forwarding it to a sub-model
// that composes its own framed view (the scrollback preview), so the sub-model
// sizes to sit inside the global §3 gutter. Any non-WindowSizeMsg passes through
// unchanged — only the resize carries terminal dimensions.
func insetWindowSizeMsg(msg tea.Msg, contentW, contentH int) tea.Msg {
	wsm, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		return msg
	}
	wsm.Width = contentW
	wsm.Height = contentH
	return wsm
}

// contentHeight is the inset content-region height every page composes into:
// (termH − 2·Vinset), with the 24-row zero/unset fallback applied first and the
// inset clamped to 0 on a too-short terminal. The vertical inset is subtracted IN
// ADDITION to the header/footer/band the list budget already reserves, so the
// pagination invariant holds against the reduced height.
func (m Model) contentHeight() int {
	_, h := m.termDims()
	return insetRegion(h, Vinset)
}

// fillCanvas is the SINGLE outer full-terminal canvas fill (§1) — the LAST
// layer wrapped over the already-composed per-page view (header + any notice
// band + list + footer). The composed view is sized to the INSET content region
// (contentW × contentH); fillCanvas pads every content line to contentW, fills to
// contentH with the mode-matched canvas background, then places that block inside
// the full terminal canvas at (Hinset, Vinset) — Hinset canvas gutter columns
// each side and Vinset canvas gutter rows top/bottom (§3 frame padding). The
// result is exactly termW × termH with the owned canvas filling the gutter, so no
// edge bleeds at line ends and no mid-screen row is left unpainted. It is an
// OUTER WRAP, not per-delegate painting: the list's width/height budget already
// folds in the inset (applySessionListSize), so the one-row-per-delegate
// pagination invariant (§3.5 / §4.1) is untouched — a dynamic vertical change
// (e.g. the §11.2 flash band) recomputes the list height UNDERNEATH the fill,
// which simply re-pads to contentH.
//
// It pads each line INDIVIDUALLY to the content width (rather than via
// lipgloss.Place, which pads only the gap BEYOND the single widest line — so a
// content line already at the content width would suppress padding of every
// shorter line). Each line is first stripped of trailing background-less spaces
// (the raw block-padding bubbles/list appends to its shorter lines — the title
// bar and pagination dots — which would otherwise leave a terminal-bg seam), so
// the canvas pad owns every line's trailing region. Styled trailing cells (the
// delegate's own canvas runs) end in a reset, not a bare space, and survive.
// Rows beyond the composed content are emitted as content-width canvas blanks up
// to contentH; content taller than contentH is clamped so the frame never
// overflows.
//
// Pre-first-WindowSizeMsg the cached dimensions are 0; the fill falls back to
// 80x24 exactly as viewLoading does (then insets), so it never sizes to zero and
// blanks the screen. A terminal too small to hold the inset clamps the gutter to
// 0 (contentW/H equal termW/H via insetRegion), so the frame renders flush.
func (m Model) fillCanvas(view string) string {
	w, h := m.termDims()
	contentW := m.contentWidth()
	contentH := m.contentHeight()
	// NO_COLOR carve-out (§2.5): no canvas bg and no mid-line backfill — render on
	// the terminal's native bg. Padding to width / filling to height keeps the
	// layout (the structure parity the capture verifies) but every padding/gutter
	// cell is a plain space with NO background SGR, so no canvas is painted.
	if m.colourless {
		content := fillColourless(view, contentW, contentH)
		return insetColourless(content, w, h, contentW, contentH)
	}
	canvas := lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(m.canvasMode))
	canvasBg := canvasBgParams(theme.MV.Canvas.ColorFor(m.canvasMode))
	parser := ansi.NewParser() // one instance reused across every line this frame

	lines := strings.Split(view, "\n")
	out := make([]string, 0, contentH)
	for _, line := range lines {
		if len(out) == contentH {
			break // clamp: never exceed the content-region height (no overflow)
		}
		// Backfill mid-line gaps FIRST (so every interior cell carries the
		// canvas bg without depending on the terminal default / OSC 11), then
		// pad the trailing region to the content width.
		line = backfillCanvasBackground(line, canvasBg, parser)
		out = append(out, padLineToCanvasWidth(line, contentW, canvas))
	}
	blank := canvas.Render(strings.Repeat(" ", contentW))
	for len(out) < contentH {
		out = append(out, blank)
	}
	return insetCanvasCanvas(out, w, h, contentW, canvas)
}

// gutterPadding splits the total horizontal (w−contentW) and vertical
// (h−contentH) gutter evenly across the two sides — the shared geometry both
// inset placers use so the coloured and colourless frames are byte-identical in
// layout. Each gutter is ≥ 0 (contentW/H are clamped to ≤ w/h by insetRegion), so
// it degrades to 0 cleanly when the inset was clamped at a tiny terminal.
func gutterPadding(w, h, contentW, contentH int) (leftPad, rightPad, topPad, botPad int) {
	hGutter := w - contentW
	leftPad = hGutter / 2
	rightPad = hGutter - leftPad
	vGutter := h - contentH
	topPad = vGutter / 2
	botPad = vGutter - topPad
	return leftPad, rightPad, topPad, botPad
}

// insetCanvasCanvas places the content-region rows (each already contentW wide)
// inside the full terminal canvas at (Hinset, Vinset) for the COLOURED path: it
// emits Vinset canvas gutter rows above, pads each content row with a canvas
// gutter column on each side (Hinset cells each), and emits Vinset canvas gutter
// rows below — every gutter cell painted the owned canvas. The horizontal /
// vertical gutter is derived from the actual w−contentW / h−contentH so it
// degrades to 0 cleanly when the inset was clamped (tiny terminal).
func insetCanvasCanvas(contentRows []string, w, h, contentW int, canvas lipgloss.Style) string {
	leftPad, rightPad, topPad, botPad := gutterPadding(w, h, contentW, len(contentRows))

	fullBlank := canvas.Render(strings.Repeat(" ", w))
	leftCol := ""
	if leftPad > 0 {
		leftCol = canvas.Render(strings.Repeat(" ", leftPad))
	}
	rightCol := ""
	if rightPad > 0 {
		rightCol = canvas.Render(strings.Repeat(" ", rightPad))
	}

	out := make([]string, 0, h)
	for i := 0; i < topPad; i++ {
		out = append(out, fullBlank)
	}
	for _, row := range contentRows {
		out = append(out, leftCol+row+rightCol)
	}
	for i := 0; i < botPad; i++ {
		out = append(out, fullBlank)
	}
	return strings.Join(out, "\n")
}

// fillColourless is the NO_COLOR (§2.5) variant of fillCanvas's per-line fill: it
// pads every content line to the content width and fills to the content height
// for layout parity, but emits NO background SGR — every padding cell is a plain
// space and every overflow row a plain blank line, so the frame renders on the
// terminal's native bg with no painted canvas. There is no mid-line backfill (no
// canvas to re-establish) and no canvas-styled blank; the composed line's own
// trailing block-padding is trimmed first so the plain pad owns the trailing
// region, matching fillCanvas's line geometry exactly (the structure the capture
// verifies). Content taller than the content height is clamped (no overflow).
func fillColourless(view string, w, h int) string {
	blank := strings.Repeat(" ", w)
	lines := strings.Split(view, "\n")
	out := make([]string, 0, h)
	for _, line := range lines {
		if len(out) == h {
			break // clamp: never exceed the content-region height (no overflow)
		}
		line = strings.TrimRight(line, " ")
		gap := w - lipgloss.Width(line)
		if gap > 0 {
			line += strings.Repeat(" ", gap)
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// insetColourless places the content-region block inside the full terminal frame
// at (Hinset, Vinset) for the NO_COLOR path: the gutter rows/columns are plain
// spaces (the terminal native bg) with no background SGR, mirroring
// insetCanvasCanvas's geometry exactly so the colourless frame matches the
// coloured frame's layout (the structure the capture verifies). The gutter is
// derived from w−contentW / h−contentH so it degrades to 0 when the inset was
// clamped.
func insetColourless(content string, w, h, contentW, contentH int) string {
	leftPad, rightPad, topPad, botPad := gutterPadding(w, h, contentW, contentH)

	fullBlank := strings.Repeat(" ", w)
	leftCol := strings.Repeat(" ", leftPad)
	rightCol := strings.Repeat(" ", rightPad)

	out := make([]string, 0, h)
	for i := 0; i < topPad; i++ {
		out = append(out, fullBlank)
	}
	for _, row := range strings.Split(content, "\n") {
		out = append(out, leftCol+row+rightCol)
	}
	for i := 0; i < botPad; i++ {
		out = append(out, fullBlank)
	}
	return strings.Join(out, "\n")
}

// backfillCanvasBackground rewrites a single composed line so the canvas
// background stays continuously active across EVERY interior cell — the fix for
// the mid-line bleed that mosh/Blink exposed (§1: "every cell carries the canvas
// bg"). Sub-renderers (bubbles/list's help/footer composition, the title row)
// emit bare spacer/gap cells between styled segments whose only background is the
// terminal default re-established by a `0`/empty SGR reset or an explicit `49`
// (background-default). On terminals that ignore OSC 11 (the `tea.View`
// BackgroundColor set), those cells render the terminal's own theme — the
// reported grey bleed right of the title box and through the footer inter-column
// gaps.
//
// The transform is surface-agnostic: it walks the line's SGR sequences and,
// whenever one resets the background to default (a full `0`/empty reset or a
// `49`), it appends the canvas background parameters to THAT SAME sequence —
// preserving any foreground / attribute codes the original reset also carried.
// SGRs that set an explicit background (selected-row tint, the violet title box,
// any `48;…` / `40-47` / `100-107`) are left untouched, so content backgrounds
// survive verbatim. A leading text run with no SGR at all (none is emitted by the
// current renderers, but a future one might) is prefixed with the canvas bg.
//
// Each canvas re-establishment is balanced by the reset already present at the
// run's end, so no extra terminal state leaks past the line; the SINGLE wrap
// point (fillCanvas) is preserved.
//
// The parser is passed in (not allocated here) so the per-line loop in fillCanvas
// reuses a single instance instead of allocating a 64 KiB-buffered parser per line
// per frame; this function never reads parser.Params (it re-derives params via
// sgrParamsList), so a shared instance is safe.
func backfillCanvasBackground(line, canvasBg string, parser *ansi.Parser) string {
	if canvasBg == "" {
		return line
	}
	canvasSetParams := strings.Split(canvasBg, ";")

	var b strings.Builder
	b.Grow(len(line) + len(canvasBg) + 8)

	src := []byte(line)
	state := byte(0)
	bgActive := false // an explicit background SGR is currently in effect

	// Printable content arrives grapheme-by-grapheme from DecodeSequence; buffer
	// a contiguous run so a no-background run is wrapped ONCE (not per cell),
	// keeping the rendered text searchable and the SGR overhead minimal.
	var run []byte
	flushRun := func() {
		if len(run) == 0 {
			return
		}
		if bgActive {
			b.Write(run)
		} else {
			// A bare printable run on the terminal default — open the canvas bg
			// around it and close with a reset so no state leaks past the run.
			b.WriteString("\x1b[")
			b.WriteString(canvasBg)
			b.WriteByte('m')
			b.Write(run)
			b.WriteString("\x1b[m")
		}
		run = run[:0]
	}

	for len(src) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(src, state, parser)
		if n == 0 {
			break
		}
		isSGR := ansi.HasCsiPrefix(seq) && seq[len(seq)-1] == 'm'
		switch {
		case isSGR:
			flushRun()
			params := sgrParamsList(string(seq))
			bgActive = sgrBackgroundActive(bgActive, params)
			if !bgActive {
				// This reset/49 dropped the bg to terminal-default. Re-establish
				// the canvas bg by folding its params into THIS sequence, so the
				// following cells stay on the canvas.
				b.WriteString(rewriteSGRWithCanvasBg(params, canvasSetParams))
				bgActive = true
			} else {
				b.Write(seq)
			}
		case width > 0:
			// Printable grapheme — accumulate into the current run.
			run = append(run, seq...)
		default:
			// Non-SGR control / escape — flush text, pass through verbatim.
			flushRun()
			b.Write(seq)
		}
		src = src[n:]
		state = newState
	}
	flushRun()
	return b.String()
}

// rewriteSGRWithCanvasBg renders an SGR ...m sequence built from the original
// params with the canvas background parameters appended. The original params'
// own background codes have just reset to default (that is why the caller is
// re-establishing), so the canvas bg is the only background that survives; the
// foreground / attribute codes carry through unchanged.
func rewriteSGRWithCanvasBg(originalParams, canvasParams []string) string {
	merged := make([]string, 0, len(originalParams)+len(canvasParams))
	for _, p := range originalParams {
		// A bare reset arrives as a single empty param ("\x1b[m") or an explicit
		// "0"; keep a "0" so the reset semantics are unambiguous, but drop the
		// empty placeholder so the rewritten sequence has no leading ";".
		if p == "" {
			merged = append(merged, "0")
			continue
		}
		merged = append(merged, p)
	}
	merged = append(merged, canvasParams...)
	return "\x1b[" + strings.Join(merged, ";") + "m"
}

// sgrParamsList splits the ";"-separated parameters from a CSI ...m sequence
// (e.g. "\x1b[38;2;1;2;3m" → ["38","2","1","2","3"]). A bare "\x1b[m" yields a
// single empty element so callers can treat it as a full reset.
func sgrParamsList(seq string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
	if inner == "" {
		return []string{""}
	}
	return strings.Split(inner, ";")
}

// sgrBackgroundActive folds an SGR sequence's parameters into the running
// "is an explicit background active?" flag. Mirrors the background-relevant codes
// the test scanner honours: 0/empty and 49 clear it; 48;… and the named bg codes
// (40-47, 100-107) set it; everything else (foreground, attrs) leaves it.
//
// Extended-colour runs (48;2;r;g;b / 48;5;n for the background, and 38;… for the
// foreground) are consumed WHOLE via consumeExtendedColorRun so a colour channel
// value that happens to equal a bg code (e.g. 49 or a 40-47) can never be misread
// as a background change. The foreground skip is belt-and-braces: today the
// renderers always emit fg immediately followed by bg, so the bg would decide the
// final state regardless — the skip just makes the scan correct for any ordering.
func sgrBackgroundActive(active bool, params []string) bool {
	for i := 0; i < len(params); i++ {
		switch p := params[i]; {
		case p == "" || p == "0" || p == "49":
			active = false
		case p == "48":
			active = true
			i = consumeExtendedColorRun(params, i)
		case p == "38":
			i = consumeExtendedColorRun(params, i)
		case isNamedBackground(p):
			active = true
		}
	}
	return active
}

// consumeExtendedColorRun returns the index of the last channel parameter of an
// extended-colour SGR run starting at i (a 38/48 followed by 2;r;g;b or 5;n). The
// caller's loop does i++ and re-checks the bound, so an over-run index is harmless
// (it simply ends the loop) — no clamp is needed.
func consumeExtendedColorRun(params []string, i int) int {
	if i+1 >= len(params) {
		return i
	}
	switch params[i+1] {
	case "2":
		return i + 4
	case "5":
		return i + 2
	}
	return i
}

func isNamedBackground(p string) bool {
	switch p {
	case "40", "41", "42", "43", "44", "45", "46", "47",
		"100", "101", "102", "103", "104", "105", "106", "107":
		return true
	}
	return false
}

// canvasBgParams returns the raw background-parameter form (e.g.
// "48;2;11;12;20") that the given canvas colour renders as, derived from
// lipgloss so the backfill folds the SAME bytes the leaf styles and outer fill
// paint. An empty result (colour produced no SGR) disables the backfill.
func canvasBgParams(c color.Color) string {
	probe := lipgloss.NewStyle().Background(c).Render(" ")
	idx := strings.IndexByte(probe, 'm')
	if idx <= 0 || !strings.HasPrefix(probe, "\x1b[") {
		return ""
	}
	return probe[len("\x1b["):idx]
}

// padLineToCanvasWidth right-pads a single composed line to width with
// canvas-background whitespace, after stripping any trailing background-less
// spaces (bubbles/list's raw block-padding) so the canvas pad owns the trailing
// region. A line already at/over width keeps its content (it is not truncated —
// horizontal overflow is a §2.7 degrade concern handled by name truncation, not
// here).
func padLineToCanvasWidth(line string, width int, canvas lipgloss.Style) string {
	line = strings.TrimRight(line, " ")
	gap := width - lipgloss.Width(line)
	if gap <= 0 {
		return line
	}
	return line + canvas.Render(strings.Repeat(" ", gap))
}

// viewString renders the current page to a plain string. This is the v1 View
// body verbatim; the v2 tea.View wrapping lives in View.
func (m Model) viewString() string {
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
	case pagePreview:
		return m.preview.View()
	default:
		return m.viewSessionList()
	}
}

// viewLoading renders the loading interstitial with centered text. It composes
// into the INSET content region (contentW × contentH) so fillCanvas places it
// inside the global gutter like every other page; the loading text centres within
// the inset region rather than the full terminal.
func (m Model) viewLoading() string {
	text := "Restoring sessions…"
	return lipgloss.Place(m.contentWidth(), m.contentHeight(), lipgloss.Center, lipgloss.Center, text)
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
	// Compose the §3.1 shared header block FIRST, above the list — it is the first
	// visible chrome. Its height is already folded out of the list's budget by
	// applySessionListSize (m.headerHeight), so the composed view stays within
	// termH and the one-row-per-delegate pagination invariant (§3.5) holds.
	header := m.renderHeader()
	return lipgloss.JoinVertical(lipgloss.Left, header, listView, footer)
}

// renderHeader renders the §3.1 shared header block for the model's current
// terminal width and resolved canvas mode (and the NO_COLOR carve-out). It is the
// single render entry point so the composed-view render and the height-budget
// computation (headerHeight) resolve the header against the SAME width/mode.
func (m Model) renderHeader() string {
	return renderHeaderBlock(m.contentWidth(), m.canvasMode, m.colourless)
}

// headerHeight is the rendered height of the §3.1 header block at the given laid-
// out width — the amount applySessionListSize reserves out of the list's height
// budget so the header (the first visible chrome) is part of the budget, NOT an
// uncounted band (§3.5). It resolves the header against the same width/mode the
// render uses (via the shared headerWidthOrFallback fallback), so the budget and
// the render agree exactly.
func (m Model) headerHeight(width int) int {
	return lipgloss.Height(renderHeaderBlock(width, m.canvasMode, m.colourless))
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
var byTagSignpostStyle = lipgloss.NewStyle().Foreground(theme.MV.TextStrong.Color()).Italic(true)

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
// value semantics prevent mutation bleed across renders. The inline-flash
// message uses the warning-flash message token (text.on-warning) per §2.9 /
// §11.2 — colour source only here; the band's bg.warning tint, ▌ left-bar, and
// ⚠/✓ glyph are the Phase 2 structural restyle.
var flashRowStyle = lipgloss.NewStyle().Foreground(theme.MV.TextOnWarning.Color())

// renderFlashRow returns the styled flash row for the Sessions page.
// flashText is rendered verbatim (no truncation, no transformation); the
// caller is responsible for the message wording (spec pins it as
// `session "<name>" no longer exists` at the call site).
func (m Model) renderFlashRow() string {
	return flashRowStyle.Render(m.flashText)
}
