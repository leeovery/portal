// Package tui provides the Bubble Tea TUI for Portal.
package tui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"sort"
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
	"github.com/leeovery/portal/internal/spawn"
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

// paginationDotGlyph is the bubbles/list built-in paginator dot glyph (•). The
// §3.5 restyle recolours it (active accent.violet / inactive text.faint) and
// centres the row without changing the glyph or the engine's page count.
const paginationDotGlyph = "•"

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

// editMode is the §8.2 two-mode state of the edit-project modal. Navigate is the
// default: Tab/Shift+Tab move between fields and ←/→ move across a chip field's
// chips + trailing + add slot; x deletes a focused chip; Esc closes. Edit puts
// ONE element live (Name or a single chip): typing edits the value, ←/→ move the
// text cursor, Enter commits & persists, Esc discards the in-progress edit.
type editMode int

const (
	editModeNavigate editMode = iota
	editModeEdit
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

// BootstrapProgressMsg is a single live per-step progress event streamed from
// the bootstrap orchestrator over the §10.2 progress channel on the concurrent
// cold-boot route. It is NON-terminal: its Update arm re-issues the progress
// receiver so the next channel event is pulled, and never drives the
// loading-page transition (only the terminal BootstrapCompleteMsg does).
//
// Index is the 1-based canonical bootstrap step number (1..10) — the stable key
// the consumer maps to a §10.4 friendly label. The mapping lives in exactly one
// place (loading_progress.go's stepLabelTable / LabelForStep), so the wire
// message deliberately carries NO friendly label and NO raw StepName: a copy of
// either here would be a second, drift-prone encoding of the same §10.4 mapping.
// RestoreN / RestoreM are the restore per-session counter (current / total);
// both zero means "no per-item counter".
type BootstrapProgressMsg struct {
	Index    int
	RestoreN int // restore per-session counter (current)
	RestoreM int // restore per-session counter (total)
}

// BootstrapFatalMsg is the §10.5 terminal FATAL event streamed over the §10.2
// progress channel on the concurrent cold/TUI route when a fatal bootstrap step
// (EnsureServer, RegisterPortalHooks, SetRestoring, ClearRestoring) aborts the
// boot. It is terminal — like BootstrapCompleteMsg it is NOT re-issued — but
// instead of dismissing the loading page it drives the in-TUI error state: the
// failed step's row gets a state.red ✗ marker, the one-line Message renders
// beneath the step-list, and the model awaits a q/Esc quit (never transitioning
// to the picker).
//
// FailedStep is the 1-based canonical index of the aborting step (1, 2, 3, or 8).
// Message is the FatalError.UserMessage (the single user-facing line). Err carries
// the underlying *bootstrap.FatalError as an error interface so internal/tui stays
// decoupled from cmd/bootstrap; openTUI extracts it post-program (via errors.As)
// and returns it so main.classify yields the code-1 exit with no double-print.
type BootstrapFatalMsg struct {
	FailedStep int
	Message    string
	Err        error
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
	colourless     bool
	selected       string
	sessionLister  SessionLister
	sessionKiller  SessionKiller
	sessionRenamer SessionRenamer
	projectStore   ProjectStore
	projectEditor  ProjectEditor
	aliasEditor    AliasEditor
	sessionCreator SessionCreator
	cwd            string
	activePage     page
	projectList    list.Model
	initialFilter  string
	// initialCursor is the capture-only cursor anchor (§5 visual gate): the name of
	// the session row the cursor should land on once the list loads. It is applied
	// (and cleared) in evaluateDefaultPage after items ingest, mirroring how
	// initialFilter is applied there. Empty is a no-op — production never sets it
	// (WithInitialCursor is wired only by the offline capture harness).
	initialCursor      string
	insideTmux         bool
	currentSession     string
	modal              modalState
	pendingKillName    string
	pendingKillWindows int
	renameInput        textinput.Model
	renameTarget       string
	pendingDeletePath  string
	pendingDeleteName  string
	command            []string
	commandPending     bool

	// Bootstrap loading state
	serverStarted     bool
	minElapsed        bool
	bootstrapComplete bool

	// §10.5 fatal cold-boot error state. fatalActive flips true on a
	// BootstrapFatalMsg; once set the model stays on the loading page rendering
	// the error frame (failed step ✗ in state.red + the one-line fatalMessage),
	// stops gating on BootstrapCompleteMsg (it will never arrive), and binds
	// q/Esc → tea.Quit. fatalStep is the 1-based aborting step index (drives the
	// failed-label overlay), and fatalErr is the underlying *bootstrap.FatalError
	// (carried as an error so openTUI can errors.As it post-program and return it
	// for the non-zero exit classification).
	fatalActive  bool
	fatalStep    int
	fatalMessage string
	fatalErr     error

	// progressReceiver is the §10.2 concurrent cold-boot route's channel-receive
	// tea.Cmd: it blocks on a single channel receive and is re-issued by the
	// BootstrapProgressMsg Update arm (the standard Bubble Tea external-channel
	// pattern — a single blocking receive re-issued preserves exact event order
	// even under command batching). When set, Init streams live per-step progress
	// from the channel and does NOT synthesize the terminal BootstrapCompleteMsg —
	// the channel owns it. When nil (the synchronous warm/CLI route), Init keeps
	// synthesizing BootstrapCompleteMsg from its first tick exactly as today.
	progressReceiver tea.Cmd

	// loadingProgress is the task-5-4 §10.4 accumulator: every BootstrapProgressMsg
	// is folded into it (in the BootstrapProgressMsg Update arm), and viewLoading
	// reads loadingProgress.View() for the §10.3 bar fraction + the ordered five
	// friendly labels with their done/active/pending states + counters. The zero
	// value is ready to use (bar at 0, every label pending), so an un-streamed
	// model renders the all-pending loading screen.
	loadingProgress LoadingProgress

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
	// flashKind selects the §11.2 MV styling of the active inline flash (spec §11.2
	// inline flash → warning / success variants). flashWarning is the zero value, so
	// the externally-killed bail (which calls setFlash) stays a warning band without
	// the caller passing a kind; setSuccessFlash flips it to the green ✓ success
	// variant. The arbiter (activeNoticeBand) maps it to the band role so the shared
	// notice-band primitive paints the matching bar colour + glyph. It is reset to
	// flashWarning by every setFlash and is irrelevant once flashText is empty.
	flashKind flashKind

	// byTagSignpost is the persistent "No tags yet" signpost flag (§11.3 / §5.3 —
	// By Tag with zero tags anywhere). It is set true by rebuildSessionList whenever
	// ModeByTag is active AND no project carries any tag — the zero-tags-anywhere
	// gate.
	// In that state the list is built with the plain flat builder (degrade
	// to flat) while viewSessionList renders a dimmed signpost row. Unlike
	// flashText (transient, cleared on the next actionable key), this is a
	// derived persistent flag recomputed on every rebuild — it clears the
	// moment the mode leaves ByTag or any tag appears.
	byTagSignpost bool

	// §5 Multi-select mode state. multiSelectMode is true while the Sessions page
	// is in multi-select mode (entered by `m` from the normal list, exited by Esc
	// or the task-5.7 N=0 commit). selectedSessions is the marked set keyed on
	// Session.Name — the SAME identity the attach / selectedSessionItem path uses —
	// so a multi-tag session that spans several By-Tag rows is marked exactly once.
	// The map is LAZILY initialised (on mode entry / first insert), so a zero-value
	// model needs no constructor change; exitMultiSelect nils it back out.
	multiSelectMode  bool
	selectedSessions map[string]struct{}

	// Async host-terminal detection lifecycle (restore-host-terminal-windows §6).
	// detector runs the process-tree/client-walk identity detection off the Update
	// path (a tea.Cmd on the command goroutine); resolve is the SAME config-aware
	// identity→adapter/resolution seam the spawn CLI uses (loaded once from
	// terminals.json at construction). Both are injected together (nil in the
	// capture harness). Reaching PageSessions dispatches Detect() exactly once
	// (detectDispatched latch); the terminalDetectedMsg arm caches the identity and
	// its resolution (detectIdentity/detectResolution, detectResolved=true). Caching
	// the Resolution — not just IsNull() — is load-bearing: a recognised-but-undriven
	// terminal is non-NULL yet resolves unsupported. A later burst REUSES this cache;
	// no rebuild re-detects or re-resolves.
	detector         TerminalDetector
	resolve          func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
	detectIdentity   spawn.Identity
	detectResolution spawn.Resolution
	detectResolved   bool
	detectDispatched bool

	// §6-3 N≥2 picker-burst seams + lifecycle (restore-host-terminal-windows). The
	// seams mirror the spawn CLI's SpawnDeps: sessionExists is the pre-flight
	// has-session probe, ackChannel is the token-ack Collect+Clean channel, spawnExe
	// resolves the picker's own binary, and spawnGetenv reads PATH — all injected
	// together (nil in the capture harness / unit tests that never drive a burst).
	// The RESOLVE seam is REUSED from the detection block above (m.resolve), never
	// re-injected.
	sessionExists func(string) bool
	ackChannel    spawn.AckChannelFull
	spawnExe      spawn.ExecutableResolver
	spawnGetenv   func(string) string

	// Burst lifecycle state. burstPending is true from dispatch until the terminal
	// spawnCompleteMsg/spawnAbortMsg lands; burstPipe/burstCancel own the goroutine's
	// channel + cancel (task 6-8 drives the cancel). burstTrigger/burstExternal are
	// the net-N split (trigger = self-attach target, external = the N-1 opened
	// windows); burstTotal is N (incl. the trigger). burstDone/burstBatch/burstResults
	// accumulate the streamed outcome (self-attach + selection mutation land in
	// 6-4/6-6). burstIdentity/burstResolution snapshot the resolved terminal.
	burstPending    bool
	burstPipe       *burstProgressPipe
	burstCancel     context.CancelFunc
	burstTrigger    string
	burstExternal   []string
	burstTotal      int
	burstDone       int
	burstBatch      string
	burstResults    []spawn.WindowResult
	burstIdentity   spawn.Identity
	burstResolution spawn.Resolution

	// pendingBurstEnter + pendingBurstOrdered stash a deferred N≥2 Enter pressed
	// while detection is still in flight (detectResolved false). The
	// terminalDetectedMsg arm re-runs the branch decision from the snapshot once
	// detection resolves, so the burst is never lost and never fires before the
	// host terminal is known.
	pendingBurstEnter   bool
	pendingBurstOrdered []string

	// Data loading tracking
	sessionsLoaded       bool
	projectsLoaded       bool
	defaultPageEvaluated bool

	// Edit project modal state — the §8.2 two-mode (navigate/edit),
	// immediate-persist state machine. There is NO batch buffer and NO dirty
	// state: every element persists on exit-edit (Enter) or, for chip deletes,
	// immediately on x; the modal only tracks the live in-progress edit and the
	// focused element.
	editProject project.Project
	editMode    editMode  // navigate (default) vs edit (one element live)
	editFocus   editField // which field (Name / Aliases / Tags) is focused
	editName    string    // the (persisted) project name; also the displayed Name value
	editAliases []string  // alias chips for this project (persisted on each edit)
	editTags    []string  // tag chips for this project (persisted on each edit)
	// editAliasCursor / editTagCursor are the focused-ELEMENT index within a chip
	// field: chips occupy [0, len-1] and the trailing + add slot is at index len.
	// Only the currently focused field's index is meaningful.
	editAliasCursor int
	editTagCursor   int
	// editChanged records that AT LEAST ONE field persisted live this session, so
	// closing on Esc refreshes the cached project records + grouping index. It is
	// NOT a dirty/unsaved flag — everything is already on disk — only a
	// refresh-needed signal (extended to Name + Aliases + Tags, not just tags).
	editChanged bool

	// Live-edit buffer (only meaningful while editMode == editModeEdit).
	editBuffer    string // the in-progress text of the live element
	editCursor    int    // text-cursor position (rune index) within editBuffer
	editIsNewChip bool   // the live chip is brand-new (vanishes on Esc-discard)
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

// MultiSelectActive reports whether the Sessions page is in §5 multi-select mode, for testing.
func (m Model) MultiSelectActive() bool {
	return m.multiSelectMode
}

// IsSessionSelected reports whether the named session is in the multi-select set, for testing.
func (m Model) IsSessionSelected(name string) bool {
	_, ok := m.selectedSessions[name]
	return ok
}

// SelectedSessionCount returns the number of sessions marked in multi-select mode, for testing.
func (m Model) SelectedSessionCount() int {
	return len(m.selectedSessions)
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

// FatalError returns the §10.5 fatal cold-boot error carried after a
// BootstrapFatalMsg, or nil when no fatal occurred. openTUI reads it after the
// Bubble Tea program returns and returns it so Execute/main.classify map the
// underlying *bootstrap.FatalError to the code-1 exit (single stderr line, no
// double-print). The error is the underlying cause (an interface so internal/tui
// stays decoupled from cmd/bootstrap); callers recover the concrete type via
// errors.As.
func (m Model) FatalError() error {
	return m.fatalErr
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
// the session list is skipped and the projects page is shown directly,
// the §11.4 command-pending banner renders over the full Projects chrome,
// and the Projects footer swaps to the §11.4 copy (renderCommandPendingFooter,
// sourced from the commandPendingKeymap descriptor).
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

// WithProgressReceiver wires the §10.2 concurrent cold-boot route's
// channel-receive tea.Cmd. When set, Init streams live per-step bootstrap
// progress from the channel (a BootstrapProgressMsg per step, the terminal
// BootstrapCompleteMsg over the same channel) and does NOT synthesize
// BootstrapCompleteMsg from its first tick — the channel owns the terminal
// event. Production (cmd/open.go) wires this only on the cold + TUI path; the
// synchronous warm/CLI route omits it, leaving the receiver nil and Init's
// today-behaviour intact. A nil receiver is a no-op so omitting the option keeps
// the synchronous path byte-for-byte unchanged.
func WithProgressReceiver(receiver tea.Cmd) Option {
	return func(m *Model) {
		m.progressReceiver = receiver
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

// WithInitialFlash seeds the §11.2 inline WARNING flash on the model at
// construction — the orange left-bar + ⚠ + message on the bg.warning tint. It is
// the capture-harness entry point for the otherwise-transient flash (production's
// only flash source is the preview-bail path). It sets the state fields directly
// (no flashGen bump, no layout resync — dimensions are still zero at construction;
// the first WindowSizeMsg resyncs the layout, which reserves the band's row). An
// empty string is a no-op so omitting the option leaves no flash. Only the warning
// variant is seedable — the success variant is not separately captured.
func WithInitialFlash(text string) Option {
	return func(m *Model) {
		if text == "" {
			return
		}
		m.flashText = text
		m.flashKind = flashWarning
	}
}

// WithInitialMultiSelect seeds the §5 multi-select mode at construction with the
// named sessions pre-marked — the capture-harness entry point for the otherwise
// user-driven mode (production enters via the `m` key, never this option). It
// mirrors handleMultiSelectToggle's enter step: set multiSelectMode, seed the
// marked set keyed on Session.Name, and refresh the delegate so the ● column arms
// from the first frame (the list is constructed with a default MultiSelect==false
// delegate). A nil/empty slice is a no-op so omitting the option leaves the model
// in normal mode. The names need not resolve to a loaded session yet — the marked
// set is keyed by name and the delegate matches on name as rows ingest.
func WithInitialMultiSelect(names []string) Option {
	return func(m *Model) {
		if len(names) == 0 {
			return
		}
		m.multiSelectMode = true
		m.selectedSessions = make(map[string]struct{}, len(names))
		for _, n := range names {
			m.selectedSessions[n] = struct{}{}
		}
		m.refreshSessionDelegate()
	}
}

// WithInitialCursor seeds the §5 capture-only cursor anchor: the name of the
// session row the cursor should land on once the list loads (evaluateDefaultPage
// re-anchors by name after items ingest, mirroring the initial-filter apply). It
// only STORES the name here — positioning happens after ingestion so it survives
// the SetItems that would otherwise reset the cursor to index 0. An empty name is
// a no-op; production never sets it (the live picker leaves the cursor at index 0).
func WithInitialCursor(name string) Option {
	return func(m *Model) {
		m.initialCursor = name
	}
}

// canvasHelpStyles backgrounds the bubbles/list HelpStyle wrapper through
// Background(canvas) for the resolved mode, so the footer (a leaf surface of the
// foundation Sessions screen, §1) renders on the owned canvas. The footer glyphs
// and labels themselves are rendered by the descriptor-driven renderCondensedFooter
// path (see footer.go), which paints its own canvas; this only ensures the
// HelpStyle wrapper around the footer area carries the canvas background too.
func canvasHelpStyles(l *list.Model, mode theme.Mode) {
	canvas := theme.MV.Canvas.ColorFor(mode)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Background(canvas)
}

// colourlessHelpStyles strips the bubbles/list HelpStyle wrapper background for
// the NO_COLOR carve-out (§2.5): no canvas background, so the footer area renders
// on the terminal's native bg. The footer glyphs/labels are rendered by the
// descriptor-driven renderCondensedFooter path, which drops its own hue under
// NO_COLOR; this only drops the canvas background the wrapper would otherwise emit.
func colourlessHelpStyles(l *list.Model) {
	l.Styles.HelpStyle = l.Styles.HelpStyle.UnsetBackground()
}

// canvasPaginationDots re-points the §3.5 height-driven paginator's dot glyph
// styles onto the §2.9 role tokens AND paints each (plus the dot row's wrapper)
// through Background(canvas) for the resolved mode: the active page dot in
// accent.violet, the inactive dots in text.faint. The bubbles/list paginator is
// kept as the engine (§14.1) — only the dot glyph styling and the centred,
// canvas-painted placement change; the page count / per-page / paging keys are
// untouched (parity, §3.6).
//
// list.New reads styles.ActivePaginationDot.String() / InactivePaginationDot
// .String() into Paginator.ActiveDot / InactiveDot ONCE at construction, so after
// restyling the styles the rendered dots are re-fed into the live paginator here.
// The dots keep the engine's bullet glyph (SetString) — only the colour changes.
func canvasPaginationDots(l *list.Model, mode theme.Mode) {
	canvas := theme.MV.Canvas.ColorFor(mode)
	l.Styles.ActivePaginationDot = lipgloss.NewStyle().
		Foreground(theme.MV.AccentViolet.ColorFor(mode)).
		Background(canvas).
		SetString(paginationDotGlyph)
	l.Styles.InactivePaginationDot = lipgloss.NewStyle().
		Foreground(theme.MV.TextFaint.ColorFor(mode)).
		Background(canvas).
		SetString(paginationDotGlyph)
	l.Paginator.ActiveDot = l.Styles.ActivePaginationDot.String()
	l.Paginator.InactiveDot = l.Styles.InactivePaginationDot.String()
	centrePaginationRow(l, lipgloss.NewStyle().Background(canvas))
}

// colourlessPaginationDots strips the paginator dot styles to bare styles for the
// NO_COLOR carve-out (§2.5): no canvas background and no foreground hue, so the
// dots render on the terminal's native fg/bg with the bullet glyph intact (§2.2 —
// the dots stay glyph-legible; foreground hue would be stripped by the writer
// layer anyway, and setting bare styles drops the canvas background lipgloss would
// otherwise still emit). The centred placement is preserved without a canvas fill.
func colourlessPaginationDots(l *list.Model) {
	l.Styles.ActivePaginationDot = lipgloss.NewStyle().SetString(paginationDotGlyph)
	l.Styles.InactivePaginationDot = lipgloss.NewStyle().SetString(paginationDotGlyph)
	l.Paginator.ActiveDot = l.Styles.ActivePaginationDot.String()
	l.Paginator.InactiveDot = l.Styles.InactivePaginationDot.String()
	centrePaginationRow(l, lipgloss.NewStyle())
}

// centrePaginationRow re-points the §3.5 paginator wrapper (Styles.PaginationStyle,
// applied by bubbles/list's paginationView to the assembled dot string) so the dot
// row renders CENTRED across the list width, on the supplied base style (canvas-
// painted for the coloured path, bare under NO_COLOR). It replaces the engine
// default's PaddingLeft(2) left alignment. The width is the list's current width
// (SetSize-driven), so the centring tracks resizes; applySessionListSize re-runs
// this after each SetSize so the width stays current. Centring is across the LIST
// width (not the terminal width) per §3.5.
func centrePaginationRow(l *list.Model, base lipgloss.Style) {
	l.Styles.PaginationStyle = base.Width(l.Width()).Align(lipgloss.Center)
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
// because the §3.4 condensed keymap footer is rendered manually by
// renderCondensedFooter over the §12.1 keymapEntry descriptors (see footer.go),
// which sources its own §2.9 role tokens — nothing in the render path consumes
// the list's own help.Model, so l.Help.Styles.* is never populated.
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

// newProjectList creates and configures a bubbles/list.Model for projects.
// See newSessionList for the rationale behind SetShowHelp(false): the keymap
// footer is rendered manually by renderCondensedFooter over the keymapEntry
// descriptors, not via the list's own help.Model.
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
	pinArrowOnlyNav(&l.KeyMap)
	l.InfiniteScrolling = true
	return l
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
// sessionDelegate constructs the SessionDelegate for the current model state: the
// resolved canvas Mode + NO_COLOR carve-out, plus the §5 multi-select fields
// (MultiSelect gate + the live selectedSessions set). It is the single source the
// delegate is built from, so applyCanvasMode and refreshSessionDelegate cannot
// drift on which fields the delegate carries.
func (m *Model) sessionDelegate() SessionDelegate {
	return SessionDelegate{
		Mode:        m.canvasMode,
		Colourless:  m.colourless,
		MultiSelect: m.multiSelectMode,
		Selected:    m.selectedSessions,
	}
}

// refreshSessionDelegate re-sets ONLY the session list's delegate from the current
// model state, without re-touching the footer/pagination/title styles. It is the
// narrow update path the §5 multi-select handlers call after mutating the marked
// set (enter / toggle / Esc exit) so the ● tracks the set on the next frame — the
// list is constructed with a default MultiSelect==false delegate, so a mutation is
// invisible until the delegate is re-pointed at the live set.
func (m *Model) refreshSessionDelegate() {
	m.sessionList.SetDelegate(m.sessionDelegate())
}

func (m *Model) applyCanvasMode() {
	// NO_COLOR carve-out (§2.5): paint no canvas at all. The delegate drops its
	// Background(canvas) leaf paint (Colourless), the footer help styles drop their
	// canvas background and foreground hue, and the title bar carries no canvas
	// background — every cell renders on the terminal's native bg. Foreground hue
	// is stripped FREE by the writer layer (colorprofile honours NO_COLOR), so
	// state stays glyph-distinct (§2.2).
	m.styleFilterInput()
	if m.colourless {
		m.sessionList.SetDelegate(m.sessionDelegate())
		colourlessHelpStyles(&m.sessionList)
		colourlessPaginationDots(&m.sessionList)
		// PaddingBottom(1) is the §3.2 section-header BOTTOM gap (Sessions → first
		// session row). It makes the title bar 2 lines (line 0 = "Sessions…", line 1
		// = the blank gap row); bubbles/list measures the rendered title-bar height
		// and reserves it from the item area, so this is auto-budgeted (no manual
		// reserve). DEPENDENCY: applySectionHeader's string surgery replaces ONLY line
		// 0 (the title) and preserves line 1 — the gap is line 1, the surgery touches
		// line 0. Under NO_COLOR the padding row is native-bg (UnsetBackground).
		// PaddingLeft(0) drops the bubbles/list default TitleBar left pad (=2): the
		// section-header surgery and the delegate rows already render flush at col 0
		// (the outer gutter insets the whole frame), so the ONLY line the left pad
		// affected was the live filter input. Flush-aligning it keeps the input-active
		// `/ query` at the SAME column as the wordmark / section header / list rows /
		// the list-active locked query (the hint-to-query swap reads cleanly).
		m.sessionList.Styles.TitleBar = m.sessionList.Styles.TitleBar.UnsetBackground().PaddingLeft(0).PaddingBottom(1)
		// Strip the bubbles/list default Title box colours (its violet 48;5;62
		// background + bright foreground) so "Sessions" renders on the terminal's
		// native fg/bg — the title is a leaf canvas-dependent surface too. The
		// coloured path leaves this default box untouched (the wordmark/header chrome
		// restyle is Phase 2); under NO_COLOR it must carry no background SGR.
		m.sessionList.Styles.Title = m.sessionList.Styles.Title.UnsetBackground().UnsetForeground()
		m.applyProjectCanvasMode()
		return
	}
	m.sessionList.SetDelegate(m.sessionDelegate())
	canvasHelpStyles(&m.sessionList, m.canvasMode)
	canvasPaginationDots(&m.sessionList, m.canvasMode)
	// Background the title bar so its leading left-pad cells (bubbles/list's
	// TitleBar PaddingLeft) are canvas rather than the terminal background. The
	// "Sessions" title box keeps its own colour (the wordmark/header chrome
	// restyle is Phase 2); this only paints the padding around it.
	canvas := theme.MV.Canvas.ColorFor(m.canvasMode)
	// PaddingBottom(1) is the §3.2 section-header BOTTOM gap (Sessions → first
	// session row). It makes the title bar 2 lines (line 0 = "Sessions…", line 1 =
	// the blank gap row, which inherits the canvas Background so it is canvas-painted
	// with no terminal-bg island); bubbles/list measures the rendered title-bar
	// height and reserves it from the item area, so this is auto-budgeted (no manual
	// reserve). DEPENDENCY: applySectionHeader's string surgery replaces ONLY line 0
	// (the title) and preserves line 1 — the gap is line 1, the surgery touches line 0.
	// PaddingLeft(0): see the colourless branch above — drops the default left pad so
	// the live filter input aligns flush with every other content line.
	m.sessionList.Styles.TitleBar = m.sessionList.Styles.TitleBar.Background(canvas).PaddingLeft(0).PaddingBottom(1)
	m.applyProjectCanvasMode()
}

// applyProjectCanvasMode re-points the §6 Projects screen's leaf styles at the
// model's resolved canvasMode (or the NO_COLOR carve-out), mirroring the Sessions
// branch of applyCanvasMode for the project list: the two-line ProjectDelegate
// paints every run through the resolved Mode (or drops hue under Colourless), the
// help/pagination styles carry the canvas, and the bubbles/list TitleBar default
// left-pad + Title box colours are stripped (PaddingLeft(0) + PaddingBottom(1) so
// the §6 section-header surgery in viewProjectList replaces line 0 and preserves the
// blank gap row on line 1, the SAME contract applySectionHeader relies on).
func (m *Model) applyProjectCanvasMode() {
	if m.colourless {
		m.projectList.SetDelegate(ProjectDelegate{Mode: m.canvasMode, Colourless: true})
		colourlessHelpStyles(&m.projectList)
		colourlessPaginationDots(&m.projectList)
		m.projectList.Styles.TitleBar = m.projectList.Styles.TitleBar.UnsetBackground().PaddingLeft(0).PaddingBottom(1)
		m.projectList.Styles.Title = m.projectList.Styles.Title.UnsetBackground().UnsetForeground()
		return
	}
	m.projectList.SetDelegate(ProjectDelegate{Mode: m.canvasMode})
	canvasHelpStyles(&m.projectList, m.canvasMode)
	canvasPaginationDots(&m.projectList, m.canvasMode)
	canvas := theme.MV.Canvas.ColorFor(m.canvasMode)
	m.projectList.Styles.TitleBar = m.projectList.Styles.TitleBar.Background(canvas).PaddingLeft(0).PaddingBottom(1)
	m.projectList.Styles.Title = m.projectList.Styles.Title.UnsetBackground().UnsetForeground()
}

// styleFilterInput restyles the bubbles/list FilterInput to the §7 MV treatment:
// an accent.orange `/ ` prompt, the live query text in accent.orange, and an
// accent.orange block cursor (input-active). It is the input-active counterpart of
// renderFilterQueryHeader (the list-active locked-query render): the same
// accent.orange `/ query` reads in both modes, the only difference being the
// cursor, which the live bubbles/list FilterInput owns while Filtering.
//
// The leaf .Background(canvas) is deliberately NOT applied here: the filter input
// carries NO background tint (§7.1) — it renders over the canvas the surrounding
// title bar already paints, with no per-run band of its own. Under the NO_COLOR
// carve-out every hue drops; the `/ ` prompt and the query render on the
// terminal's native fg, structurally distinct from the rest of the chrome.
//
// Cursor blink is disabled so the captured frame is deterministic (the cursor is
// always the solid orange block the §7.1 reference shows, never a blinked-off gap).
func (m *Model) styleFilterInput() {
	m.sessionList.FilterInput.Prompt = filterPromptPrefix
	styles := m.sessionList.FilterInput.Styles()
	if m.colourless {
		// No canvas, no hue: the `/ ` prompt + query render on the terminal's native
		// fg, and the cursor falls back to a bare (non-coloured) block. The colour is
		// stripped FREE by the writer layer under NO_COLOR, but pin a hue-free style
		// here too so no accent.orange SGR is emitted at all.
		styles.Focused.Prompt = lipgloss.NewStyle()
		styles.Focused.Text = lipgloss.NewStyle()
		styles.Cursor.Color = lipgloss.NoColor{}
		styles.Cursor.Blink = false
		m.sessionList.FilterInput.SetStyles(styles)
		return
	}
	orange := theme.MV.AccentOrange.ColorFor(m.canvasMode)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(orange)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(orange)
	styles.Cursor.Color = orange
	styles.Cursor.Blink = false
	m.sessionList.FilterInput.SetStyles(styles)
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

// applyListSize is the shared sizing core for the per-page wrappers
// (applySessionListSize / applyProjectListSize): it sizes the given list to
// width × (height − reserved) and re-centres the §3.5 paginator dot row across
// the new list width (the centred PaginationStyle pins an explicit Width, so it
// must track every resize), painted on the owned canvas or bare under the
// NO_COLOR carve-out. The wrappers own the reserved-height computation (header +
// footer + the §11 notice band) so call sites cannot drift; this core owns the
// SetSize + paginator-recentre pairing both share. Callers invoke a wrapper, not
// this core directly.
func (m *Model) applyListSize(l *list.Model, width, height, reserved int) {
	l.SetSize(width, height-reserved)
	if m.colourless {
		centrePaginationRow(l, lipgloss.NewStyle())
		return
	}
	centrePaginationRow(l, lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(m.canvasMode)))
}

// applySessionListSize is the per-page wrapper that owns the §3.4 Sessions
// height budget: it reserves the header block, the condensed footer, and any
// active notice band before sizing m.sessionList, so call sites cannot drift on
// what the Sessions list reserves.
//
// It also subtracts the height of any active notice band (the §11.2 flash row
// and/or the By-Tag "No tags yet" signpost) inserted beneath the title by
// viewSessionList, so the composed Sessions view stays within termH. This is the
// "list height recompute underneath the fill" of §1: when a band appears or
// clears the list reserves one fewer / one more row and the outer canvas fill
// simply re-pads to termH — the one-row-per-delegate pagination invariant (§3.5
// / §4.1) holds and the frame never overflows the viewport.
func (m *Model) applySessionListSize(width, height int) {
	// Fold the §3.1 header block height AND the §3.4 condensed footer height out of
	// the budget (in addition to the §11 notice band) so bubbles/list paginates
	// against the reduced height: both the header and the footer are part of the
	// height budget, NOT uncounted bands (§3.5). Each is resolved against the SAME
	// width this size-apply was called with, so the budget and the viewSessionList
	// render agree at every call site (WindowSizeMsg, rebuildSessionList, the 80x24
	// construction seed). The Sessions footer no longer routes through the former
	// manual three-column path; the header, condensed-footer, and §11
	// single-slot notice-band heights are reserved directly here and handed to the
	// shared applyListSize core. sessionBandHeight is the §11.2 F10 hook: it is one
	// row while a band owns the slot, zero when it clears, so the list reserves /
	// releases exactly one row as the band appears / clears.
	reserved := m.sessionBandHeight() + m.headerHeight(width) + m.sessionFooterHeight(width)
	m.applyListSize(&m.sessionList, width, height, reserved)
}

// sessionFooterHeight is the rendered height of the §3.4 condensed Sessions footer
// at the given laid-out width — the amount applySessionListSize reserves out of the
// list's height budget so the single-row footer (plus its 1px top rule) is part of
// the budget, NOT an uncounted band (§3.5). It resolves the footer against the same
// width/mode the render uses (via the shared headerWidthOrFallback fallback), so
// the budget and the viewSessionList render agree exactly. With the §2-2 header
// height already reserved, this keeps the one-row-per-delegate pagination invariant
// exact.
func (m Model) sessionFooterHeight(width int) int {
	return lipgloss.Height(renderSessionsFooter(width, m.canvasMode, m.colourless))
}

// sessionBandHeight returns the rendered height of the SINGLE arbitrated notice
// SLOT viewSessionList inserts beneath the title (§11 single-slot rule) — the
// amount applySessionListSize reserves out of the list's height budget so the slot
// never pushes the composed view past termH. The slot is the active band PLUS the
// canvas-painted blank row beneath it (the band→section-header breathing gap), so
// the reserve is two rows when a band owns the slot — released to zero when the
// slot empties. This is the §11.2 F10 recompute: a band appearing reserves two
// fewer list rows (band + blank), clearing releases both, and the outer canvas
// fill re-pads to termH so the one-row-per-delegate pagination invariant holds.
//
// It is measured off renderSessionBandSlot — the SAME block viewSessionList
// composes — so the reserved row count is, by construction, exactly what is
// inserted and the two can never drift (a future multi-line band stays correct
// automatically).
func (m *Model) sessionBandHeight() int {
	slot := m.renderSessionBandSlot()
	if slot == "" {
		return 0
	}
	return lipgloss.Height(slot)
}

// applyProjectListSize is the per-page wrapper that owns the §6 Projects height
// budget: it reserves the §3.1 PORTAL header block and the §6.3 condensed footer
// before sizing m.projectList, so bubbles/list paginates against the reduced height
// and the composed view (header + section header + rows + footer) stays within
// termH — the one-row-per-delegate (here, two-line) pagination invariant holds.
// Both reserves resolve against the SAME width this size-apply was called with, so
// the budget and the viewProjectList render agree at every call site. The Projects
// footer no longer routes through the former manual three-column path;
// the header, condensed-footer, and command-pending band heights are reserved
// directly here.
func (m *Model) applyProjectListSize(width, height int) {
	reserved := m.headerHeight(width) + m.projectFooterHeight(width) + m.projectBandHeight()
	m.applyListSize(&m.projectList, width, height, reserved)
}

// projectFooterHeight is the rendered height of the §6.3 condensed Projects footer
// at the given laid-out width — the amount applyProjectListSize reserves out of the
// list's height budget so the single-row footer (plus its 1px top rule) is part of
// the budget, NOT an uncounted band (§3.5). It resolves the footer against the same
// width/mode the render uses so the budget and the viewProjectList render agree
// exactly.
func (m Model) projectFooterHeight(width int) int {
	return lipgloss.Height(renderProjectsFooter(width, m.canvasMode, m.colourless))
}

// projectBandHeight returns the rendered height of the §11.4 command-pending notice
// SLOT viewProjectList inserts beneath the title separator — the banner PLUS the
// canvas-painted blank breathing row beneath it (two rows when a command is pending,
// more if the banner ever wraps). It is reserved out of the list's height budget so
// the slot never pushes the composed view past termH (mirrors sessionBandHeight).
//
// It is measured off renderProjectBandSlot — the SAME block viewProjectList composes
// — so the reserved row count is, by construction, exactly what is inserted and the
// two can never drift.
func (m Model) projectBandHeight() int {
	slot := m.renderProjectBandSlot()
	if slot == "" {
		return 0
	}
	return lipgloss.Height(slot)
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
	// §5 sticky selection: drop any marked session that vanished from the
	// refreshed list (e.g. externally killed during the Space preview) BEFORE
	// the delegate re-render below, so the pruned-and-refreshed set feeds the ●.
	m.pruneSelectionToLiveSessions()
	return m.rebuildSessionList()
}

// pruneSelectionToLiveSessions drops from the §5 multi-select set any marked
// session name no longer present in m.sessions — the "prune what's gone" rule
// shared with the pre-flight gate (spec § Multi-Select Mode → Sticky selection;
// § Burst & Partial-Failure Contract → Pre-flight). Called from the applySessions
// refresh chokepoint (notably the post-preview-dismiss refresh) so a session
// externally killed during the Space preview leaves the selection while every
// survivor is kept. Marks are keyed on Session.Name, so this is a pure
// set-difference against the refreshed slice; deleting the aliased set's keys
// re-renders the ● off the pruned rows on the next frame (the delegate references
// the same set). A nil/empty set short-circuits, so the non-multi-select refresh
// paths (the SessionsMsg / kill / rename refreshes) pay nothing. Kept a reusable
// method so Task 5.7 / the Phase-6 pre-flight prune reuse the identical rule.
func (m *Model) pruneSelectionToLiveSessions() {
	if len(m.selectedSessions) == 0 {
		return
	}
	live := make(map[string]struct{}, len(m.sessions))
	for i := range m.sessions {
		live[m.sessions[i].Name] = struct{}{}
	}
	for name := range m.selectedSessions {
		if _, ok := live[name]; !ok {
			delete(m.selectedSessions, name)
		}
	}
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

	m.applyInitialFilter()
	m.applyInitialCursor()
}

// applyInitialFilter commits the deferred initial filter (if any) onto the active
// page's list once the default page is settled, then clears it so it applies at
// most once. Empty is a no-op.
func (m *Model) applyInitialFilter() {
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

// applyInitialCursor re-anchors the session-list cursor onto the seeded row name
// (the §5 capture-only WithInitialCursor anchor) once items have ingested, then
// clears it so it applies at most once. It runs AFTER applyInitialFilter so the
// re-anchor operates on the post-filter visible set. Only the Sessions page has a
// cursor anchor; empty is a no-op (the production default), so the live picker
// keeps the default index-0 selection.
func (m *Model) applyInitialCursor() {
	if m.initialCursor == "" {
		return
	}
	if m.activePage == PageSessions {
		m.reanchorSessionCursor(m.initialCursor)
	}
	m.initialCursor = ""
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
	// Default kind: warning (§11.2). The externally-killed bail calls setFlash, so
	// the unparameterised path stays the orange ⚠ warning band; setSuccessFlash is
	// the explicit opt-in to the green ✓ success variant. Resetting here means a
	// success flash followed by a plain setFlash reverts to warning.
	m.flashKind = flashWarning
	m.resyncSessionLayout()
}

// setSuccessFlash records an inline flash styled as the §11.2 SUCCESS variant —
// a state.green left-bar + ✓ glyph (glyph-distinct from the warning ⚠, never
// colour-only, §2.2). It shares the warning flash's lifecycle exactly: it bumps
// flashGen, assigns flashText, and re-syncs the layout — the only difference is
// the kind, so the auto-clear tick + generation guard + actionable-key clear all
// apply unchanged. The verbatim message is the caller's (no wording owned here).
func (m *Model) setSuccessFlash(text string) {
	m.flashGen++
	m.flashText = text
	m.flashKind = flashSuccess
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

// fetchSessionsCmd returns the tea.Cmd that enumerates live tmux sessions and
// wraps the result in a SessionsMsg. It is the single source of the session
// enumeration so the Init frame-one fetch and the §10.2 post-restore re-fetch
// (see refetchSessionsAfterRestore) issue byte-identical commands. A pure read —
// it never mutates tmux or state.
func (m Model) fetchSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.sessionLister.ListSessions()
		return SessionsMsg{Sessions: sessions, Err: err}
	}
}

// refetchSessionsAfterRestore is the §10.2 Part-B carry-forward fix. On the
// concurrent cold-boot route (progressReceiver != nil) the orchestrator runs in
// a goroutine, so Init's frame-one fetchSessions enumerated tmux BEFORE Restore
// (bootstrap step 6) created the saved skeleton sessions — that Init snapshot is
// stale/empty. When the loading page dismisses (both gates closed) the picker
// would otherwise render that stale snapshot — the prior-incident
// "empty-previews / slow-open" surface. So on this route ONLY, re-enumerate
// sessions at the transition so the Sessions page reflects post-restore tmux
// state. The resulting SessionsMsg lands on PageSessions and re-renders the list
// via applySessions/rebuildSessionList.
//
// The warm/synchronous route (progressReceiver == nil) returns nil: there
// PersistentPreRunE ran the orchestrator synchronously BEFORE the model was
// built, so Init's snapshot is already post-restore — a re-fetch would be a
// wasted enumeration and a behaviour change. Keeping the warm path's enumeration
// unchanged is the §10.1 zero-new-risk contract.
func (m Model) refetchSessionsAfterRestore() tea.Cmd {
	if m.progressReceiver == nil {
		return nil
	}
	return m.fetchSessionsCmd()
}

// transitionFromLoading moves from the loading page to the normal sessions page.
//
// On the warm/CLI route (nil progressReceiver) the Init snapshot is already
// post-restore (PersistentPreRunE ran the orchestrator synchronously before the
// model was built), so it marks sessions loaded and lets evaluateDefaultPage
// make the landing decision immediately.
//
// On the cold concurrent route (non-nil progressReceiver) the Init snapshot is
// stale/empty — Restore (bootstrap step 6) ran in a goroutine AFTER frame-one
// enumeration, so the restored sessions did not exist yet when Init enumerated.
// Deciding the landing page here would latch defaultPageEvaluated against that
// stale interim list (→ Projects) and the post-restore refetch could never
// re-decide. So on the cold route this only moves off PageLoading onto a valid
// interim PageSessions and leaves sessionsLoaded false, so every
// evaluateDefaultPage caller early-returns until the post-restore refetch's
// SessionsMsg lands and makes the one true decision against the repaired list.
// The refetch is dispatched in the SAME handler return (see the
// LoadingMinElapsedMsg / BootstrapCompleteMsg arms).
func (m *Model) transitionFromLoading() {
	m.activePage = PageSessions
	if m.progressReceiver != nil {
		return
	}
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
	// fetchSessionsCmd is the single session-enumeration command; the §10.2
	// post-restore re-fetch (refetchSessionsAfterRestore) issues the identical
	// command at loading-page dismissal on the concurrent route.
	fetchSessions := m.fetchSessionsCmd()
	loadProjects := m.loadProjects()

	if m.activePage == PageLoading {
		loadingPadTick := tea.Tick(LoadingMinDuration, func(time.Time) tea.Msg {
			return LoadingMinElapsedMsg{}
		})
		cmds := []tea.Cmd{requestBg, detectTimeout, fetchSessions, loadingPadTick}
		if m.progressReceiver != nil {
			// §10.2 concurrent cold-boot route: the orchestrator runs in a
			// goroutine and streams progress over a channel. Wire the receiver so
			// channel events flow into Update as BootstrapProgressMsg / the
			// terminal BootstrapCompleteMsg. Do NOT synthesize BootstrapCompleteMsg
			// here — the channel owns the terminal event, so synthesizing it would
			// dismiss the loading page before the orchestrator finished.
			cmds = append(cmds, m.progressReceiver)
		} else {
			// Synchronous warm/CLI route: PersistentPreRunE has already run the
			// orchestrator before this Init, so emit BootstrapCompleteMsg from the
			// first event-loop tick to satisfy the bootstrapComplete gate, carrying
			// any pending warnings drained from the package-level sink by openTUI.
			// Loading dismissal still requires the LoadingMinElapsedMsg tick (1.2s
			// minimum-display floor); warnings are flushed to stderr at that moment
			// via flushBufferedWarningsCmd.
			pending := m.pendingBootstrapWarnings
			cmds = append(cmds, func() tea.Msg { return BootstrapCompleteMsg{Warnings: pending} })
		}
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
		// §6 warm direct Sessions entry: once evaluateDefaultPage has landed
		// PageSessions, dispatch the async host-terminal detection exactly once (the
		// detectDispatched latch + the PageSessions guard inside
		// maybeDispatchDetectionCmd make this a no-op on the loading/refetch
		// re-entries and on a Projects landing). On the cold route this arm
		// early-returned above while PageLoading, so detection is dispatched by the
		// loading→Sessions transition arms instead.
		return m, tea.Batch(cmd, m.maybeDispatchDetectionCmd())
	case LoadingMinElapsedMsg:
		m.minElapsed = true
		// §10.5: once a fatal aborted the boot the model is parked in the error
		// state — never dismiss the loading page into the picker, even if the
		// min-elapsed gate fires after the fatal.
		if m.fatalActive {
			return m, nil
		}
		if m.bootstrapComplete && m.activePage == PageLoading {
			m.transitionFromLoading()
			// §10.2 Part-B: re-enumerate sessions on the concurrent cold-boot
			// route so the picker reflects post-restore tmux state, not the
			// stale Init snapshot (no-op on the warm route). Batched with the
			// warnings-surface cmd so neither is dropped. §6: also dispatch the
			// async host-terminal detection here (the cold/warm loading→Sessions
			// transition), guarded once via the detectDispatched latch.
			return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore(), m.maybeDispatchDetectionCmd())
		}
		return m, nil
	case BootstrapProgressMsg:
		// §10.2 concurrent cold-boot route: a live per-step progress event from
		// the channel. Non-terminal — record it for the loading-screen render
		// (task 5-5) and re-issue the receiver so the NEXT channel event is
		// pulled. A single blocking receive re-issued preserves exact event
		// order even though Bubble Tea batches commands. Never drives the
		// transition: only the terminal BootstrapCompleteMsg does, which keeps
		// the TUI inert during loading (§10.2 race containment). A nil receiver
		// (defensive: a stray progress msg with no receiver wired) stops the
		// loop rather than re-issuing nil.
		//
		// Fold the event into the §10.4 accumulator so viewLoading (§10.3) renders
		// the live bar fraction + tick-list states + counter from it. Apply returns
		// a new value (no receiver mutation), so the accumulator stays the single
		// source of the loading-screen render.
		m.loadingProgress = m.loadingProgress.Apply(msg)
		return m, m.progressReceiver
	case BootstrapCompleteMsg:
		// §10.5: a fatal already parked the model in the error state. The
		// concurrent route never sends BOTH a fatal and a complete (the pipe maps
		// the terminal channel event to one or the other), but guard defensively so
		// a stray complete can never dismiss the error frame into a picker.
		if m.fatalActive {
			return m, nil
		}
		m.bootstrapComplete = true
		// Only buffer warnings when still on the loading page; warnings
		// that arrive after dismissal (orphaned BootstrapCompleteMsg) are
		// dropped so they cannot accumulate dead state on the model.
		if m.activePage == PageLoading {
			m.bufferedWarnings = msg.Warnings
		}
		if m.minElapsed && m.activePage == PageLoading {
			m.transitionFromLoading()
			// §10.2 Part-B: see the LoadingMinElapsedMsg arm — re-fetch sessions
			// on the concurrent route so the post-transition picker reflects
			// post-restore tmux state (no-op on the warm route). §6: also dispatch
			// the async host-terminal detection here, guarded once via the
			// detectDispatched latch (the LoadingMinElapsedMsg arm is a no-op when
			// this one fired first, and vice versa).
			return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore(), m.maybeDispatchDetectionCmd())
		}
		return m, nil
	case BootstrapFatalMsg:
		// §10.5 fatal cold-boot abort: a fatal step returned over the §10.2
		// channel. Enter the in-TUI error state — record the failed step + the
		// one-line message + the underlying error, and STOP gating on
		// BootstrapCompleteMsg (it will never arrive on this route). The model
		// stays on PageLoading: it must NEVER flip to PageSessions on a fatal (no
		// half-restored picker, ever). viewLoading renders the error frame and the
		// PageLoading key arm binds q/Esc → tea.Quit from here. Do NOT re-issue the
		// receiver — this is a terminal event.
		m.fatalActive = true
		m.fatalStep = msg.FailedStep
		m.fatalMessage = msg.Message
		m.fatalErr = msg.Err
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
	case terminalDetectedMsg:
		// §6 async host-terminal detection resolved on the command goroutine. Cache
		// the identity AND its resolution via the injected config-aware resolve seam.
		// Caching the Resolution (not just IsNull()) is load-bearing: a
		// recognised-but-undriven terminal is non-NULL yet resolves unsupported. A
		// later picker burst reuses this cache; no rebuild re-detects or re-resolves.
		m.detectIdentity = msg.identity
		_, m.detectResolution = m.resolve(msg.identity)
		m.detectResolved = true
		// §6-3: a deferred N≥2 Enter (pressed while detection was in flight) resolves
		// its branch decision now that the terminal is known — supported → dispatch the
		// burst, unsupported → the atomic no-op. Without this the deferred Enter would
		// wait forever.
		if m.pendingBurstEnter {
			return m.decideBurst(m.pendingBurstOrdered)
		}
		return m, nil
	case spawnProgressMsg:
		// §6-3 non-terminal burst progress: record the per-window counter and re-issue
		// the receiver so the next channel event is pulled (mirroring
		// BootstrapProgressMsg). A nil pipe (defensive: a stray progress msg) stops the
		// loop rather than re-issuing nil. burstTotal is NOT touched here: it is N
		// (incl. the trigger), set once at dispatch, whereas msg.Total is the external
		// count (N-1) — overwriting would wrongly shrink BurstTotal() mid-burst.
		m.burstDone = msg.Done
		if m.burstPipe == nil {
			return m, nil
		}
		return m, m.burstPipe.receiver()
	case spawnCompleteMsg:
		// §6-3 terminal burst outcome: record the batch + per-window results and clear
		// burst-pending. The self-attach to the trigger + the selection mutation are
		// tasks 6-4/6-6; this task lands the streaming + outcome capture only.
		m.burstResults = msg.Results
		m.burstBatch = msg.Batch
		m.burstPending = false
		return m, nil
	case spawnAbortMsg:
		// §6-3 pre-flight abort: a selected session vanished between marking and Enter,
		// so nothing spawned. Clear burst-pending; the abort UI is task 6-7.
		m.burstPending = false
		return m, nil
	case burstChannelClosedMsg:
		// The burst channel drained and closed — the post-terminal sentinel. No-op:
		// the terminal spawnCompleteMsg/spawnAbortMsg already cleared pending.
		return m, nil
	}

	// Delegate to the active view
	switch m.activePage {
	case PageLoading:
		keyMsg, isKey := msg.(tea.KeyPressMsg)
		if isKey && keyIsCtrlC(keyMsg) {
			return m, tea.Quit
		}
		// §10.5 error state: q AND Esc both quit (non-zero — openTUI returns the
		// carried *bootstrap.FatalError). Outside the error state the loading page
		// stays inert (animation only, §10.2 race containment), so these keys only
		// quit once the fatal is active.
		if m.fatalActive && isKey && (isRuneKey(keyMsg, "q") || keyIsCode(keyMsg, tea.KeyEscape)) {
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
		if m.projectList.SettingFilter() {
			break
		}
		// ? opens OUR per-page help modal (§8.5). This REPLACES the prior swallow —
		// the swallow existed so bubbles/list never toggled its own help; opening
		// our modal still consumes the key, so the list help never fires either.
		if isRuneKey(msg, "?") {
			m.modal = modalHelp
			return m, nil
		}
		switch {
		case keyIsCode(msg, tea.KeyEscape):
			if m.projectList.FilterState() == list.FilterApplied {
				break
			}
			return m, tea.Quit
		case isRuneKey(msg, "q"):
			return m, tea.Quit
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
	case keyIsCode(keyMsg, tea.KeyEscape):
		// §8.1/§8.6: cancel is Esc only — `n` is dropped (now ignored). Clear BOTH
		// pending fields so no stale state leaks past dismissal.
		m.modal = modalNone
		m.pendingDeletePath = ""
		m.pendingDeleteName = ""
		return m, nil
	}
	// Ignore all other keys while modal is active (including `n`).
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
	m.editMode = editModeNavigate
	m.editFocus = editFieldName
	m.editChanged = false
	m.editAliasCursor = 0
	m.editTagCursor = 0
	m.resetEditBuffer()

	// Seed the tag buffer from a copy of the project's tags so mutating the
	// buffer never aliases the stored project slice. slices.Clone(nil)
	// returns nil, so a back-compat record with no tags seeds an empty buffer.
	m.editTags = slices.Clone(pi.Project.Tags)

	// Load aliases matching this project's directory. The matches are sorted into a
	// stable (alphabetical) order: Load() returns a map, so the raw iteration order
	// is non-deterministic — sorting gives the alias chips a fixed render order
	// (matching the deterministic capture fixture) and a predictable left-to-right
	// chip layout for the user. Order of an unordered set is cosmetic only.
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
		sort.Strings(matching)
		m.editAliases = matching
	}

	return m, nil
}

// resetEditBuffer clears the live-edit buffer back to its navigate-mode resting
// state (no in-progress text, cursor at 0, not a brand-new chip).
func (m *Model) resetEditBuffer() {
	m.editBuffer = ""
	m.editCursor = 0
	m.editIsNewChip = false
}

func (m Model) updateEditProjectModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.editMode == editModeEdit {
		return m.updateEditModeKey(keyMsg)
	}
	return m.updateNavigateModeKey(keyMsg)
}

// updateNavigateModeKey handles a key in §8.2 navigate mode: Tab/Shift+Tab (and
// ↓/↑, their aliases) move between fields, ←/→ move across a chip field's chips +
// trailing + add slot, Enter/e/+ enter edit mode, x deletes a focused chip
// immediately, and Esc closes the modal (refreshing the cached projects if
// anything persisted).
func (m Model) updateNavigateModeKey(keyMsg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Code {
	case tea.KeyEscape:
		return m.closeEditModal()

	case tea.KeyTab:
		if keyMsg.Mod&tea.ModShift != 0 {
			m.focusField(m.prevField())
		} else {
			m.focusField(m.nextField())
		}
		return m, nil

	case tea.KeyDown:
		// ↓ is an alias for Tab (next field), mirroring nextField including
		// wrap-around and landing a chip field on its trailing + add slot.
		m.focusField(m.nextField())
		return m, nil

	case tea.KeyUp:
		// ↑ is an alias for Shift+Tab (previous field).
		m.focusField(m.prevField())
		return m, nil

	case tea.KeyLeft:
		m.moveElement(-1)
		return m, nil

	case tea.KeyRight:
		m.moveElement(+1)
		return m, nil

	case tea.KeyEnter:
		return m.enterEditFromNavigate()

	default:
		// Printable-rune key press: a single character lands in Text with no
		// modifiers. x deletes a focused chip; e/Enter edit it; + on the add
		// slot spawns a new chip.
		if keyMsg.Mod != 0 || keyMsg.Text == "" {
			return m, nil
		}
		switch keyMsg.Text {
		case "x":
			return m.deleteFocusedChip()
		case "e":
			// `e` enters edit mode on the NAME field (mirroring Enter) and on a
			// focused chip — matching the `⏎/e edit` footer hint. The + add slot
			// uses `+`/Enter, not `e`. In edit mode `e` is a literal char (handled
			// by updateEditModeKey, never reaching here).
			if m.editFocus == editFieldName || m.focusedOnChip() {
				return m.enterEditFromNavigate()
			}
			return m, nil
		case "+":
			if m.focusedOnAddSlot() {
				return m.enterEditFromNavigate()
			}
			return m, nil
		}
		return m, nil
	}
}

// updateEditModeKey handles a key in §8.2 edit mode (one element live): typing
// edits the value at the text cursor, ←/→ move the text cursor, Enter commits &
// persists (returning to navigate), and Esc discards the in-progress edit (a
// brand-new chip vanishes), also returning to navigate.
func (m Model) updateEditModeKey(keyMsg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Code {
	case tea.KeyEscape:
		return m.discardEdit()

	case tea.KeyEnter:
		return m.commitEdit()

	case tea.KeyLeft:
		if m.editCursor > 0 {
			m.editCursor--
		}
		return m, nil

	case tea.KeyRight:
		if m.editCursor < len([]rune(m.editBuffer)) {
			m.editCursor++
		}
		return m, nil

	case tea.KeyBackspace:
		m.backspaceEditBuffer()
		return m, nil

	default:
		// Printable-rune key press: insert the literal character at the cursor.
		// In edit mode x is a literal char, not a delete.
		if keyMsg.Mod != 0 || keyMsg.Text == "" {
			return m, nil
		}
		m.insertEditRunes(keyMsg.Text)
		return m, nil
	}
}

// --- Navigate-mode helpers -------------------------------------------------

func (m Model) nextField() editField {
	switch m.editFocus {
	case editFieldName:
		return editFieldAliases
	case editFieldAliases:
		return editFieldTags
	default:
		return editFieldName
	}
}

func (m Model) prevField() editField {
	switch m.editFocus {
	case editFieldName:
		return editFieldTags
	case editFieldTags:
		return editFieldAliases
	default:
		return editFieldName
	}
}

// focusField moves focus to field, landing a chip field on its trailing + add
// slot (the common-action default — ← then reaches the existing chips).
func (m *Model) focusField(field editField) {
	m.editFocus = field
	switch field {
	case editFieldAliases:
		m.editAliasCursor = len(m.editAliases)
	case editFieldTags:
		m.editTagCursor = len(m.editTags)
	}
}

// moveElement shifts the focused chip field's element index by delta, bounded to
// [0, len] (len being the + add slot). Name has no elements, so it is a no-op.
func (m *Model) moveElement(delta int) {
	switch m.editFocus {
	case editFieldAliases:
		m.editAliasCursor = clampInt(m.editAliasCursor+delta, 0, len(m.editAliases))
	case editFieldTags:
		m.editTagCursor = clampInt(m.editTagCursor+delta, 0, len(m.editTags))
	}
}

// focusedChips returns the chip slice and element index for the focused chip
// field, plus ok==true only when a CHIP field is focused.
func (m Model) focusedChips() (chips []string, idx int, ok bool) {
	switch m.editFocus {
	case editFieldAliases:
		return m.editAliases, m.editAliasCursor, true
	case editFieldTags:
		return m.editTags, m.editTagCursor, true
	default:
		return nil, 0, false
	}
}

// focusedOnChip reports whether the focus is on an existing chip (not the + add
// slot, and not the Name field).
func (m Model) focusedOnChip() bool {
	chips, idx, ok := m.focusedChips()
	return ok && idx < len(chips)
}

// focusedOnAddSlot reports whether the focus is on a chip field's trailing
// + add slot.
func (m Model) focusedOnAddSlot() bool {
	chips, idx, ok := m.focusedChips()
	return ok && idx == len(chips)
}

// deleteFocusedChip removes the focused chip immediately, persisting the delete
// (alias via DeleteAndSave, tag via RemoveTag). x on the + add slot or Name is a
// no-op.
func (m Model) deleteFocusedChip() (tea.Model, tea.Cmd) {
	if !m.focusedOnChip() {
		return m, nil
	}
	switch m.editFocus {
	case editFieldAliases:
		removed := m.editAliases[m.editAliasCursor]
		if m.aliasEditor != nil {
			if _, err := m.aliasEditor.DeleteAndSave(removed, "cli"); err != nil {
				return m, nil
			}
		}
		m.editAliases = slices.Delete(m.editAliases, m.editAliasCursor, m.editAliasCursor+1)
		m.editAliasCursor = clampInt(m.editAliasCursor, 0, len(m.editAliases))
	case editFieldTags:
		removed := m.editTags[m.editTagCursor]
		if m.projectEditor != nil {
			if err := m.projectEditor.RemoveTag(m.editProject.Path, removed); err != nil {
				return m, nil
			}
		}
		m.editTags = slices.Delete(m.editTags, m.editTagCursor, m.editTagCursor+1)
		m.editTagCursor = clampInt(m.editTagCursor, 0, len(m.editTags))
	}
	m.editChanged = true
	return m, nil
}

// enterEditFromNavigate transitions to edit mode for the focused element. On a
// chip the buffer seeds from the chip's value; on the + add slot a brand-new
// empty chip is spawned (editIsNewChip); on Name the buffer seeds from the
// current name.
func (m Model) enterEditFromNavigate() (tea.Model, tea.Cmd) {
	m.editMode = editModeEdit
	switch m.editFocus {
	case editFieldName:
		m.editBuffer = m.editName
		m.editIsNewChip = false
	default:
		chips, idx, _ := m.focusedChips()
		if idx < len(chips) {
			m.editBuffer = chips[idx]
			m.editIsNewChip = false
		} else {
			m.editBuffer = ""
			m.editIsNewChip = true
		}
	}
	m.editCursor = len([]rune(m.editBuffer))
	return m, nil
}

// closeEditModal closes the modal (navigate-mode Esc). All work is already
// persisted, so this never discards; it only refreshes the cached project
// records + grouping index when something changed this session.
func (m Model) closeEditModal() (tea.Model, tea.Cmd) {
	m.modal = modalNone
	if m.editChanged {
		m.editChanged = false
		return m, m.loadProjects()
	}
	return m, nil
}

// --- Edit-mode helpers -----------------------------------------------------

func (m *Model) insertEditRunes(text string) {
	runes := []rune(m.editBuffer)
	pos := clampInt(m.editCursor, 0, len(runes))
	inserted := []rune(text)
	out := make([]rune, 0, len(runes)+len(inserted))
	out = append(out, runes[:pos]...)
	out = append(out, inserted...)
	out = append(out, runes[pos:]...)
	m.editBuffer = string(out)
	m.editCursor = pos + len(inserted)
}

func (m *Model) backspaceEditBuffer() {
	if m.editCursor == 0 {
		return
	}
	runes := []rune(m.editBuffer)
	pos := clampInt(m.editCursor, 0, len(runes))
	m.editBuffer = string(runes[:pos-1]) + string(runes[pos:])
	m.editCursor = pos - 1
}

// discardEdit backs out the in-progress edit (Esc in edit mode): a brand-new
// chip vanishes, an existing chip/Name keeps its prior value. Nothing persists.
func (m Model) discardEdit() (tea.Model, tea.Cmd) {
	m.editMode = editModeNavigate
	m.resetEditBuffer()
	// An existing chip or Name simply retains its prior value (the buffer was
	// never written through). A brand-new chip was never added to the slice, so
	// it also needs no slice mutation — it just vanishes. Re-clamp the element
	// index so the focus stays in bounds (the add slot for a vanished chip).
	switch m.editFocus {
	case editFieldAliases:
		m.editAliasCursor = clampInt(m.editAliasCursor, 0, len(m.editAliases))
	case editFieldTags:
		m.editTagCursor = clampInt(m.editTagCursor, 0, len(m.editTags))
	}
	return m, nil
}

// commitEdit persists the live element and returns to navigate mode, applying
// the §8.2 falling-out rules (empty-on-commit = delete; empty Name reverts;
// duplicate-on-commit = silent dedupe; cross-project alias collision = silent
// revert). It always exits edit mode — there is no blocking error modal.
func (m Model) commitEdit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.editBuffer)
	field := m.editFocus
	m.editMode = editModeNavigate

	switch field {
	case editFieldName:
		m.commitName(value)
	case editFieldAliases:
		m.commitAlias(value)
	case editFieldTags:
		m.commitTag(value)
	}

	m.resetEditBuffer()
	return m, nil
}

// commitName persists a Name change via Rename. An empty Name can't persist and
// reverts to the prior value (no Rename call, no error).
func (m *Model) commitName(value string) {
	if value == "" {
		// Empty Name reverts to prior — editName is untouched.
		return
	}
	if value == m.editName {
		return
	}
	if m.projectEditor != nil {
		if err := m.projectEditor.Rename(m.editProject.Path, value, "cli"); err != nil {
			return
		}
	}
	m.editName = value
	m.editChanged = true
}

// commitAlias persists an alias chip edit. Empty-on-commit deletes the chip;
// a duplicate (already present in this field) dedupes silently; a cross-project
// collision (the alias maps to a DIFFERENT project) is a silent revert.
func (m *Model) commitAlias(value string) {
	idx := m.editAliasCursor
	existing := idx < len(m.editAliases)

	if value == "" {
		m.deleteAliasAt(idx)
		return
	}

	// Duplicate within this field: silent no-op (the existing chip remains).
	for i, a := range m.editAliases {
		if a == value && i != idx {
			if existing {
				// The user renamed an existing chip onto a duplicate: drop the
				// edited chip, leave the original.
				m.deleteAliasAt(idx)
			}
			return
		}
	}

	// Cross-project collision pre-check: reject if the alias maps to a different
	// project path. Silent revert (no error modal) — consistent with the
	// duplicate rule (see Edge Cases in the task / §8.2).
	if m.aliasEditor != nil {
		if all, err := m.aliasEditor.Load(); err == nil {
			if path, ok := all[value]; ok && path != m.editProject.Path {
				if !existing {
					m.dropNewChip()
				}
				return
			}
		}
		if err := m.aliasEditor.SetAndSave(value, m.editProject.Path, "cli"); err != nil {
			if !existing {
				m.dropNewChip()
			}
			return
		}
	}

	if existing {
		// An existing chip was renamed: delete the old alias name and set the new.
		old := m.editAliases[idx]
		if old != value && m.aliasEditor != nil {
			_, _ = m.aliasEditor.DeleteAndSave(old, "cli")
		}
		m.editAliases[idx] = value
	} else {
		m.editAliases = append(m.editAliases, value)
		m.editAliasCursor = len(m.editAliases)
	}
	m.editChanged = true
}

// commitTag persists a tag chip edit. Empty-on-commit deletes; a duplicate
// (case-sensitive via NormaliseTag) dedupes silently.
func (m *Model) commitTag(value string) {
	idx := m.editTagCursor
	existing := idx < len(m.editTags)

	if value == "" {
		m.deleteTagAt(idx)
		return
	}

	tag, ok := project.NormaliseTag(value)
	if !ok {
		if existing {
			m.deleteTagAt(idx)
		} else {
			m.dropNewChip()
		}
		return
	}

	// Duplicate within this field (case-sensitive): silent no-op.
	for i, existingTag := range m.editTags {
		if existingTag == tag && i != idx {
			if existing {
				m.deleteTagAt(idx)
			} else {
				m.dropNewChip()
			}
			return
		}
	}

	if existing {
		old := m.editTags[idx]
		if old != tag && m.projectEditor != nil {
			_ = m.projectEditor.RemoveTag(m.editProject.Path, old)
		}
		if m.projectEditor != nil {
			if err := m.projectEditor.AddTag(m.editProject.Path, tag); err != nil {
				return
			}
		}
		m.editTags[idx] = tag
	} else {
		if m.projectEditor != nil {
			if err := m.projectEditor.AddTag(m.editProject.Path, tag); err != nil {
				m.dropNewChip()
				return
			}
		}
		m.editTags = append(m.editTags, tag)
		m.editTagCursor = len(m.editTags)
	}
	m.editChanged = true
}

// deleteAliasAt removes the alias chip at idx (persisting the delete) when idx
// addresses an existing chip; a non-existent index (the just-spawned new chip)
// simply vanishes. The element index is re-clamped to the + add slot.
func (m *Model) deleteAliasAt(idx int) {
	if idx < len(m.editAliases) {
		removed := m.editAliases[idx]
		if m.aliasEditor != nil {
			_, _ = m.aliasEditor.DeleteAndSave(removed, "cli")
		}
		m.editAliases = slices.Delete(m.editAliases, idx, idx+1)
		m.editChanged = true
	}
	m.editAliasCursor = len(m.editAliases)
}

func (m *Model) deleteTagAt(idx int) {
	if idx < len(m.editTags) {
		removed := m.editTags[idx]
		if m.projectEditor != nil {
			_ = m.projectEditor.RemoveTag(m.editProject.Path, removed)
		}
		m.editTags = slices.Delete(m.editTags, idx, idx+1)
		m.editChanged = true
	}
	m.editTagCursor = len(m.editTags)
}

// dropNewChip re-clamps the element index to the + add slot after a brand-new
// chip fails to commit (collision, blank, persist error) and so never enters
// the slice.
func (m *Model) dropNewChip() {
	switch m.editFocus {
	case editFieldAliases:
		m.editAliasCursor = len(m.editAliases)
	case editFieldTags:
		m.editTagCursor = len(m.editTags)
	}
}

// clampInt returns v bounded to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
		// When the list is actively filtering, let it handle all key input
		// (so ? is a literal filter character, never the help toggle).
		if m.sessionList.SettingFilter() {
			break
		}
		// ? opens OUR per-page help modal (§8.5). This REPLACES the prior swallow —
		// the swallow existed so bubbles/list never toggled its own help; opening
		// our modal still consumes the key, so the list help never fires either.
		if isRuneKey(msg, "?") {
			m.modal = modalHelp
			return m, nil
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
			// Propagate the resolved canvas mode + NO_COLOR carve-out so the
			// §9.1 cyan peek-mode chrome resolves the right token variant (and
			// drops hue under NO_COLOR, §9.2).
			pmodel.mode = m.canvasMode
			pmodel.colourless = m.colourless
			m.preview = pmodel
			m.activePage = pagePreview
			return m, nil
		}
		switch {
		case keyIsCode(msg, tea.KeyEscape):
			// §5 Multi-select: Esc exits the mode and clears the whole set. This
			// LEADING branch is reachable only when the / filter is not focused
			// (the SettingFilter guard above already routed a focused-filter Esc to
			// the list), so it is checked BEFORE the progressive-back filter check.
			if m.multiSelectMode {
				return m.exitMultiSelect(), nil
			}
			// Progressive back: if filter is active, let the list clear it;
			// otherwise quit.
			if m.sessionList.FilterState() == list.FilterApplied {
				break
			}
			return m, tea.Quit
		case isRuneKey(msg, "q"):
			return m, tea.Quit
		case isRuneKey(msg, "k"):
			// §5 Multi-select suppresses the row-action keys: k (kill) does not
			// compose with a marked set, so it is a no-op in the mode. The arm stays
			// PRESENT (gated, not deleted) so keymap_dispatch_guard_test.go's
			// default-mode probe still sees k honoured.
			if m.multiSelectMode {
				return m, nil
			}
			return m.handleKillKey()
		case isRuneKey(msg, "r"):
			// §5 Multi-select suppresses r (rename) — a row action that does not
			// compose with a marked set — as a no-op in the mode (arm kept for the
			// default-mode dispatch-parity probe).
			if m.multiSelectMode {
				return m, nil
			}
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
		// m enters / toggles §5 multi-select mode. Like s, this case MUST stay
		// inside this rune switch, below the `if m.sessionList.SettingFilter()
		// { break }` guard above — that guard makes m a literal filter character
		// while the / filter input is focused. Do NOT hoist above that guard.
		case isRuneKey(msg, "m"):
			return m.handleMultiSelectToggle()
		// x is the sole Sessions↔Projects toggle (§12.2). The former p alias
		// (Sessions → Projects) is dropped so each key has a single meaning.
		case isRuneKey(msg, "x"):
			// §5 Multi-select suppresses x (page-toggle) — a row-adjacent action that
			// does not compose with a marked set — as a no-op in the mode (arm kept
			// for the default-mode dispatch-parity probe).
			if m.multiSelectMode {
				return m, nil
			}
			m.activePage = PageProjects
			return m, nil
		case keyIsCode(msg, tea.KeyEnter):
			// §5 Multi-select owns Enter while in the mode: it commits the marked set
			// via handleMultiSelectEnter (the task-5.7 N=0/N=1/N≥2 boundary). This arm
			// is reached only when the / filter is NOT focused — a focused-filter Enter
			// is delegated to the list (commit-to-browse) by the SettingFilter break
			// above — so multi-select open never fires while filtering.
			if m.multiSelectMode {
				return m.handleMultiSelectEnter()
			}
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

// handleMultiSelectToggle drives the §5 multi-select `m` key. The first press
// from the normal list ENTERS the mode with an empty selection (enter-only, no
// implicit mark). Every subsequent press toggles the highlighted session row's
// Session.Name in the marked set — inserting if absent, deleting if present. A
// press while the highlighted row is a non-selectable HeaderItem (or the list is
// empty / has no highlighted row) is a no-op that leaves the set untouched. It
// returns (tea.Model, tea.Cmd) to match the updateSessionList dispatch arms.
func (m Model) handleMultiSelectToggle() (tea.Model, tea.Cmd) {
	if !m.multiSelectMode {
		m.multiSelectMode = true
		m.selectedSessions = map[string]struct{}{}
		// Re-point the delegate at the (now non-nil) set and MultiSelect==true so the
		// ● column arms from the next frame — the list was built with a default
		// MultiSelect==false delegate.
		(&m).refreshSessionDelegate()
		return m, nil
	}
	si, ok := m.selectedSessionItem()
	if !ok {
		return m, nil
	}
	if m.selectedSessions == nil {
		m.selectedSessions = map[string]struct{}{}
	}
	name := si.Session.Name
	if _, marked := m.selectedSessions[name]; marked {
		delete(m.selectedSessions, name)
	} else {
		m.selectedSessions[name] = struct{}{}
	}
	// Refresh so the delegate reflects the mutated set on the next frame (the map is
	// aliased, but re-pointing keeps the wiring robust to a reallocated set).
	(&m).refreshSessionDelegate()
	return m, nil
}

// exitMultiSelect leaves §5 multi-select mode and clears the whole marked set. It
// is the shared exit path — Esc invokes it here and the task-5.7 N=0 commit
// reuses it — so it is a reusable method returning the updated model rather than
// an inline reset.
func (m Model) exitMultiSelect() Model {
	m.multiSelectMode = false
	m.selectedSessions = nil
	// Re-point the delegate at MultiSelect==false so the ● column disarms and no row
	// renders a marker after the exit.
	(&m).refreshSessionDelegate()
	return m
}

// handleMultiSelectEnter commits the §5 multi-select marked set when Enter is
// pressed in the mode (the / filter not focused — that Enter is delegated to the
// list to commit-to-browse). It is dispatched from the updateSessionList Enter
// mode-branch. The cursor/highlight is irrelevant — only the marked set commits.
//
// The N-count boundary (spec Multi-Select Mode → N=0 / N=1 boundary):
//   - N=0: no-op EXIT — reuse exitMultiSelect so Portal stays open with nothing
//     opened (same effect as Esc; NOT a quit).
//   - N=1: degenerates to a plain single attach — set m.selected to the one
//     marked name and quit, byte-identical in effect to handleSessionListEnter,
//     so the cmd layer's existing self-attach connector opens it in the current
//     window (no special-casing, no adapter).
//   - N≥2: the §6-3 spawn-burst boundary — build the list-ordered marked set and
//     hand it to beginBurst, which gates on the async terminal detection and
//     dispatches the async host-window burst (or defers until detection resolves).
func (m Model) handleMultiSelectEnter() (tea.Model, tea.Cmd) {
	switch len(m.selectedSessions) {
	case 0:
		m = m.exitMultiSelect()
		return m, nil
	case 1:
		var name string
		for name = range m.selectedSessions {
		}
		m.selected = name
		return m, tea.Quit
	default:
		return m.beginBurst(m.orderedMarkedSessions())
	}
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
	m.pendingKillWindows = si.Session.Windows
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
	case modalHelp:
		return m.updateHelpModal(msg)
	default:
		return m, nil
	}
}

// updateHelpModal handles the §8.5 per-page help modal. It is key-exclusive
// (§8.1): ? toggles it closed and Esc dismisses it (modalNone), and EVERY other
// key is consumed — so Esc dismisses the help and does NOT fall through to the
// page's clear-filter / quit (the edge case: help open over an applied filter
// must dismiss the help only, leaving the filter intact). Non-key messages are
// swallowed too while the modal owns the screen.
func (m Model) updateHelpModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if isRuneKey(keyMsg, "?") || keyIsCode(keyMsg, tea.KeyEscape) {
		m.modal = modalNone
		return m, nil
	}
	// Key-exclusive: consume all other keys (no fall-through to page binds).
	return m, nil
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
		m.pendingKillWindows = 0
		return m, m.killAndRefresh(name)
	case keyIsCode(keyMsg, tea.KeyEscape):
		// §8.3 drops `n` — cancel is Esc only (the §8.1 modal anatomy). `n` falls
		// through to the "ignore all other keys" tail below (no cancel, no confirm).
		m.modal = modalNone
		m.pendingKillName = ""
		m.pendingKillWindows = 0
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
	// No inline prompt: the §8.4 reskin renders the field as a `NEW NAME` label over
	// an orange-outlined input box (renderRenameModalContent), so the textinput shows
	// the value alone — the former "New name: " prompt would double up inside the box.
	ti.Prompt = ""
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
	// Scope: the gate holds EVERY owned-canvas surface, including the §10 cold-path
	// loading page (§10.2 canvas-flip avoidance) — it paints the resolved (or
	// dark-fallback) canvas from frame one, never a paint-then-flip. Init arms the
	// detect-or-timeout window on the loading path too (the loadingPadTick batch
	// includes detectTimeout), so the gate resolves under the same race; the
	// tens-of-ms wait is invisible against the multi-hundred-ms bootstrap.
	if !m.modeResolved() {
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
	for range topPad {
		out = append(out, fullBlank)
	}
	for _, row := range contentRows {
		out = append(out, leftCol+row+rightCol)
	}
	for range botPad {
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
	for range topPad {
		out = append(out, fullBlank)
	}
	for row := range strings.SplitSeq(content, "\n") {
		out = append(out, leftCol+row+rightCol)
	}
	for range botPad {
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
		return m.viewProjectList()
	case pagePreview:
		return m.preview.View()
	default:
		return m.viewSessionList()
	}
}

// viewLoading renders the §10.3 honest loading interstitial: the locked block
// PORTAL wordmark + violet caret bar, a thick violet progress bar on the bg.track
// track, and a real 5-row tick-list, all driven live from the §10.4
// loadingProgress accumulator (folded per BootstrapProgressMsg). It composes into
// the INSET content region (contentW × contentH) so View()→fillCanvas paints the
// owned canvas around it, and centres the block via lipgloss.Place exactly like
// every other page. The render degrades on a narrow/short terminal (§2.7) and
// drops the canvas + hues under the NO_COLOR carve-out (§2.5).
func (m Model) viewLoading() string {
	view := m.loadingProgress.View()
	if m.fatalActive {
		// §10.5 error frame: the failed step's row flips to a state.red ✗, the bar
		// freezes at the fraction reached at fatal time, and the one-line message +
		// quit hint render beneath the step-list (renderLoadingScreen folds Message
		// in). No page transition — the model stays on PageLoading awaiting q/Esc.
		view = m.loadingProgress.FailedView(m.fatalStep, m.fatalMessage)
	}
	return renderLoadingScreen(
		view,
		m.contentWidth(),
		m.contentHeight(),
		m.canvasMode,
		m.colourless,
	)
}

// viewProjectList renders the §6 Modern Vivid Projects page: the §3.1 shared PORTAL
// header block, the §6 Projects section header (state.green label + text.detail
// count + right-aligned `/ to filter` hint) swapped in place of the plain
// bubbles/list title, the two-line MV rows, and the §6.3 condensed footer — all
// composed exactly the way viewSessionList composes the Sessions page. When a modal
// is open the page is instead CLEARED to the owned canvas (§8.1/13.5 blank-screen
// modal layer) and only the centred panel is returned — the list/header/footer
// chrome is not composed; see the in-body comment below.
func (m Model) viewProjectList() string {
	// §8.1/13.5 blank-screen modal layer (shared — Projects inherits the same
	// change as Sessions): when a modal is open the project list behind it is
	// CLEARED to the owned canvas — return ONLY the centred panel sized to the
	// inset content region and let the View()→fillCanvas outer wrap paint the
	// cleared backdrop (NO_COLOR suppression + the 80×24 fallback inherited from
	// that Phase 1 path). The chrome is NOT composed while a modal is up.
	// §14.6 ADAPT decision recorded on placeModalOnClearedCanvas.
	switch m.modal {
	case modalDeleteProject:
		// §8.6 delete-project confirm modal: the MV hand-drawn single-tone joined panel
		// (the SAME frame the kill modal uses) — ▲ Delete project? / <name> + <path> +
		// record-only consequence / y delete · esc cancel. The confirm/cancel LOGIC is
		// unchanged (updateDeleteProjectModal); only the rendering is reskinned.
		return renderDeleteModalOnClearedCanvas(m.pendingDeleteName, m.pendingDeletePath, m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	case modalEditProject:
		// §8.2/§13.1: the MV two-mode edit-project modal is its OWN hand-drawn
		// single-tone joined panel (renderEditProjectContent), placed directly on the
		// cleared canvas via renderEditModalOnClearedCanvas — the already-framed panel
		// is placed without any lipgloss auto-border wrap that would add a redundant
		// second border.
		return renderEditModalOnClearedCanvas(m, m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	case modalHelp:
		// §8.5 per-page help: the Projects keymap descriptor, descriptor-driven, in
		// the help modal's own zero-h-padding panel (FIX 4).
		return renderHelpModalOnClearedCanvas(projectsKeymap(), m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	}
	listView := m.projectList.View()
	// §6 / §3.2: replace the plain bubbles/list title line with the restyled
	// Projects section header (state.green `Projects` label + text.detail count +
	// right-aligned `/ to filter` hint). Like the Sessions page, swapping the title
	// row's CONTENT (not adding a row) keeps the one-row-per-delegate pagination
	// invariant exact. While the filter input is active the title row IS that input —
	// leave it untouched so the user sees what they type.
	listView = m.applyProjectsSectionHeader(listView)
	// §11.1 empty-projects state: the project list is GENUINELY empty (zero items AND
	// no active filter — projectListEmpty REQUIRES the Unfiltered state). Replace the
	// (empty) list body BELOW the title row with the centred block glyph `▌ ▌ ▌` +
	// `No projects yet` + the open-a-directory hint, mirroring the empty-sessions
	// state. Suppressed while a command is pending — the §11.4 command-pending banner
	// + footer own that case (the user is being asked to pick a project to run), so
	// the empty-state guidance would be wrong there.
	if m.projectListEmpty() && !m.commandPending {
		listView = m.replaceListBodyWithEmptyState(listView, m.projectList.Height(), emptyProjectsGlyph, emptyProjectsMessage, emptyProjectsHint)
	}
	// §6.3 condensed footer: the §6.3 Projects keymap copy over the shared 1px
	// border.footer rule (or the §11.4 command-pending footer while a command is
	// pending — renderProjectsFooterForFilterState arbitrates). Its height is reserved
	// out of the list's budget by applyProjectListSize (resolved against the SAME
	// contentWidth) so the composed view stays within termH.
	footer := m.renderProjectsFooterForFilterState()
	// Compose the §3.1 shared PORTAL header block FIRST, above the list — the first
	// visible chrome. Its height is folded out of the list's budget by
	// applyProjectListSize (m.headerHeight), so the composed view stays within termH.
	header := m.renderHeader()
	// §11.4 command-pending banner: the violet `▌` left-bar + `▸` caret + `Pick a
	// project to run` + the joined command in an accent.orange chip, on a subtle
	// tinted band. Like the Sessions notice band (§11) it sits DIRECTLY under the
	// title separator, ABOVE the section header (line 0 of listView), full-width —
	// the slot (renderProjectBandSlot) is the band PLUS one canvas-painted blank
	// breathing row beneath it, so the section header + list shift down by TWO rows.
	// The slot's height is reserved out of the list's budget by applyProjectListSize
	// (projectBandHeight, measured off the SAME slot), so the composed view re-pads to
	// termH and the one-row-per-delegate pagination invariant holds. Suppressed while
	// a modal is up (handled above by the cleared-canvas return).
	if slot := m.renderProjectBandSlot(); slot != "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, slot, listView, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, listView, footer)
}

// renderProjectCommandBand renders the §11.4 command-pending banner for the model's
// current width / resolved canvas mode (and the NO_COLOR carve-out), or the empty
// string when no command is pending. It is the single render entry point the band
// SLOT (renderProjectBandSlot) composes beneath the title separator, and from which
// the slot's height is measured, so the budget and the render agree.
func (m Model) renderProjectCommandBand() string {
	if !m.commandPending {
		return ""
	}
	return renderCommandBand(m.command, m.contentWidth(), m.canvasMode, m.colourless)
}

// renderProjectBandSlot renders the FULL §11.4 Projects notice slot for the model's
// current width / canvas mode — the command-pending banner PLUS one canvas-painted
// full-width blank row BENEATH it (the band→section-header breathing gap), or the
// empty string when no command is pending. Mirrors renderSessionBandSlot: the band
// stays flush under the title separator, the blank separates it from the section
// header (line 0 of listView), so the slot composes as band → blank → listView.
//
// This is the SINGLE source of truth for what the slot inserts: both viewProjectList
// (composition) and projectBandHeight (the F10 height reserve) consume it, so the
// reserved row count is, by construction, exactly the rendered height of what is
// composed — the two can never drift.
func (m Model) renderProjectBandSlot() string {
	band := m.renderProjectCommandBand()
	if band == "" {
		return ""
	}
	blank := blankCanvasRow(m.contentWidth(), m.canvasMode, m.colourless)
	return lipgloss.JoinVertical(lipgloss.Left, band, blank)
}

// applyProjectsSectionHeader swaps the §6 restyled Projects section header in place
// of the plain bubbles/list title line (the FIRST line of listView), mirroring
// applySectionHeader for the Sessions page. Replacing the title row's content
// (rather than inserting a row) keeps the one-row-per-delegate pagination invariant
// exact. While the filter input is active (FilterState == Filtering) the first line
// is the live filter input — leave it untouched. In the list-active mode
// (FilterState == FilterApplied) swap in the locked accent.orange `/ query` header
// the same way the Sessions page does, so the filtered Projects list reads identically.
func (m Model) applyProjectsSectionHeader(listView string) string {
	if m.projectList.FilterState() == list.Filtering {
		return listView
	}
	if m.projectList.FilterState() == list.FilterApplied {
		header := renderFilterQueryHeader(
			m.projectList.FilterValue(),
			m.contentWidth(),
			m.canvasMode,
			m.colourless,
		)
		idx := strings.IndexByte(listView, '\n')
		if idx < 0 {
			return header
		}
		return header + listView[idx:]
	}
	header := renderProjectsSectionHeader(
		m.visibleProjectRowCount(),
		m.contentWidth(),
		m.canvasMode,
		m.colourless,
	)
	idx := strings.IndexByte(listView, '\n')
	if idx < 0 {
		return header
	}
	return header + listView[idx:]
}

// visibleProjectRowCount is the §6 / §3.2 count source for the Projects section
// header: the number of VISIBLE project rows — the SAME count the rendered list
// shows, so an applied filter (VisibleItems is the filtered set) is reflected.
func (m Model) visibleProjectRowCount() int {
	return len(m.projectList.VisibleItems())
}

// renderProjectsFooterForFilterState resolves the §6.3 condensed Projects footer to
// the correct variant for the current filter mode (§7.1), mirroring the Sessions
// page: while the filter input is active the input-active footer renders, once
// committed the list-active footer renders, otherwise the standard condensed
// Projects footer renders. All three are two rows over the SAME border.footer rule,
// so the swap is height-neutral against the reserved budget.
func (m Model) renderProjectsFooterForFilterState() string {
	switch m.projectList.FilterState() {
	case list.Filtering:
		return renderFilteringFooter(m.contentWidth(), m.canvasMode, m.colourless)
	case list.FilterApplied:
		// Projects-specific list-active footer: Enter on Projects is "new session",
		// not "attach" — do not leak the Sessions filterAppliedFooter copy here.
		return renderProjectsFilterAppliedFooter(m.contentWidth(), m.canvasMode, m.colourless)
	default:
		// §11.4: the command-pending Projects footer (`⏎ run here · n run in cwd ·
		// esc cancel`) replaces the standard §6.3 condensed footer while a command is
		// pending — but only outside an active filter mode (the contextual filter
		// footers above still own the filter states).
		if m.commandPending {
			return renderCommandPendingFooter(m.contentWidth(), m.canvasMode, m.colourless)
		}
		// §11.1 empty-projects state: the standard footer is FULLY REPLACED by the
		// projects-relevant keys (`n new in cwd · x sessions · / filter · ? help`),
		// drawn from the Projects keymap descriptor. Two-row footer over the SAME
		// border.footer rule, so the swap is height-neutral against the reserved
		// budget. Gated AFTER command-pending so the command-pending footer wins.
		if m.projectListEmpty() {
			return renderEmptyProjectsFooter(m.contentWidth(), m.canvasMode, m.colourless)
		}
		return renderProjectsFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
}

// viewSessionList renders the session list using bubbles/list, composes the
// optional inline flash row between the list's title/filter row and the
// rest, then composes the condensed keymap footer beneath the resulting block.
// When the flash is inactive the list sits directly under the title/filter as
// before (spec § Inline flash — feature-local infrastructure > Render); the footer
// sits below the composed list+flash block. When a modal is open the page is
// instead CLEARED to the owned canvas (§8.1/13.5) and only the centred panel is
// returned — see the in-body comment below.
func (m Model) viewSessionList() string {
	// §8.1/13.5 blank-screen modal layer: when a modal is open the page behind it
	// is CLEARED to the owned canvas — return ONLY the centred panel sized to the
	// inset content region and let the View()→fillCanvas outer wrap paint the
	// cleared owned-canvas backdrop (NO_COLOR suppression and the 80×24 zero-dims
	// fallback are inherited from that same Phase 1 path — no double-branch here).
	// The list/header/footer chrome below is NOT composed while a modal is up, so
	// no list rows (and no §11.2 flash band) leak into the cleared view; on dismissal
	// the list renders normally again, leaving the pagination invariant untouched.
	// §14.6 ADAPT decision recorded on placeModalOnClearedCanvas.
	switch m.modal {
	case modalKillConfirm:
		// §8.3 kill-confirm modal: the MV hand-drawn single-tone joined panel (the
		// SAME frame the help modal uses) — ▲ Kill session? / <name> · N window(s) +
		// consequence / y kill · esc cancel. The confirm/cancel LOGIC is unchanged
		// (updateKillConfirmModal); only the rendering is reskinned.
		return renderKillModalOnClearedCanvas(m.pendingKillName, m.pendingKillWindows, m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	case modalRename:
		// §8.4 rename modal: the MV hand-drawn single-tone joined panel (the SAME
		// frame the help/kill modals use) — Rename session header / NEW NAME label +
		// violet-outlined input box + was: <old name> / ⏎ rename · esc cancel footer.
		// The rename flow LOGIC is unchanged (updateRenameModal / renameAndRefresh);
		// only the rendering is reskinned.
		return renderRenameModalOnClearedCanvas(m.renameInput, m.renameTarget, m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	case modalHelp:
		// §8.5 per-page help: the Sessions keymap descriptor, descriptor-driven, in
		// the help modal's own zero-h-padding panel (FIX 4).
		return renderHelpModalOnClearedCanvas(sessionsKeymap(), m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless)
	}
	listView := m.sessionList.View()
	// §3.2 / §4.2: replace the plain bubbles/list title line with the restyled
	// section header (Sessions label + state.green count + mode suffix + the
	// right-aligned `/ to filter` hint). The title row already costs exactly one
	// line in the list's height budget, so swapping its CONTENT (not adding a row)
	// keeps the one-row-per-delegate pagination invariant (§3.5) exact. While the
	// filter input is active the title row IS that input — leave it untouched so
	// the user sees what they type.
	listView = m.applySectionHeader(listView)
	// §7.3 over-filtered no-matches state: an ACTIVE non-empty filter query with
	// zero visible items. Replace the (empty) list body BELOW the title/filter row
	// with the centred null-set glyph + `No sessions match "<query>"` message +
	// widen/clear hint. This is DISTINCT from the §11.1 empty-sessions state (no
	// sessions exist at all, no active query) — sessionListNoMatches REQUIRES an
	// active non-empty query, so the two paths are NOT merged. The replacement keeps
	// the SAME body height (Height()−1 rows), so the composed view stays within termH
	// and the one-row-per-delegate pagination invariant (§3.5) is unaffected.
	if m.sessionListNoMatches() {
		listView = m.replaceListBodyWithNoMatches(listView)
	}
	// §11.1 empty-sessions state: the session list is GENUINELY empty (zero items
	// AND no active filter — sessionListEmpty REQUIRES the Unfiltered state). Replace
	// the (empty) list body BELOW the title row with the centred block glyph
	// `▌ ▌ ▌` + `No sessions yet` + the hint. This is DISTINCT from the §7.3
	// no-matches state above (items exist, an active query filters to zero) — the two
	// predicates are mutually exclusive (no-matches needs an active query, empty needs
	// Unfiltered), so they never both fire. The replacement keeps the SAME body height
	// (Height()−1 rows), so the §3.5 one-row-per-delegate pagination invariant holds.
	if m.sessionListEmpty() {
		listView = m.replaceListBodyWithEmptyState(listView, m.sessionList.Height(), emptySessionsGlyph, emptySessionsMessage, emptySessionsHint)
	}
	// §3.4 condensed footer: a single row of the Core keymap keys (sourced from the
	// task 2-1 descriptor) over a 1px border.footer rule, replacing the manual
	// three-column footer for Sessions. Its height is folded out of the list's budget
	// by applySessionListSize (m.sessionFooterHeight) — resolved against the SAME
	// contentWidth — so the composed view stays within termH and the
	// one-row-per-delegate pagination invariant (§3.5) holds.
	//
	// §7.1: while a filter mode is active the standard footer is REPLACED by one of
	// the two contextual filter footers (input-active vs list-active). All three
	// footers are exactly two rows (the shared border.footer rule + one entry row),
	// so the swap is height-neutral — the budget reserved by sessionFooterHeight
	// holds regardless of filter mode.
	footer := m.renderSessionsFooterForFilterState()
	// Compose the §3.1 shared header block FIRST, above the list — it is the first
	// visible chrome. Its height is already folded out of the list's budget by
	// applySessionListSize (m.headerHeight), so the composed view stays within
	// termH and the one-row-per-delegate pagination invariant (§3.5) holds.
	header := m.renderHeader()
	// §11 single-slot notice band: the slot holds AT MOST ONE band, resolved by the
	// arbiter (renderActiveNoticeBand) — the transient flash wins over the
	// persistent By-Tag signpost while shown, the signpost returns once the flash
	// clears, so the two never render at once. The band sits DIRECTLY under the
	// title separator (the header block's bottom rule), ABOVE the section header
	// (line 0 of listView), full-width. The slot (renderSessionBandSlot) is the band
	// PLUS one canvas-painted blank row beneath it (a breathing gap so the band does
	// not render flush against the section header) — so the slot composes as
	// band → blank → listView and the section header + list shift down by TWO rows.
	// The slot's two-row height is reserved out of the list's budget by
	// applySessionListSize (sessionBandHeight, measured off the SAME slot), so the
	// composed view re-pads to termH and the §3.5 / §4.1 one-row-per-delegate
	// pagination invariant (the §11.2 F10 recompute) holds. Composing it between the
	// header and the list (not inside the list, where the former in-list inserts
	// placed it) is what lands it ABOVE the section header per the §11 placement
	// rule. The blank is below ONLY — the band stays flush under the title separator.
	if slot := m.renderSessionBandSlot(); slot != "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, slot, listView, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, listView, footer)
}

// renderSessionsFooterForFilterState resolves the §3.4 condensed footer to the
// correct variant for the current filter mode (§7.1). While the filter input is
// active (FilterState == Filtering) the input-active footer renders
// (`type to filter · ↵/↓ browse results · esc clear`); once committed
// (FilterState == FilterApplied) the list-active footer renders
// (`↵ attach · ↑↓ navigate · esc clear filter`). Otherwise the standard condensed
// footer renders. All three are two rows over the SAME border.footer rule, so the
// swap is height-neutral against the reserved sessionFooterHeight budget.
func (m Model) renderSessionsFooterForFilterState() string {
	// §7.3 over-filtered no-matches state keeps the footer in the input-active form,
	// REDUCED: `type to filter · esc clear` WITHOUT the `browse results` entry (there
	// are no results to browse). It takes precedence over the plain Filtering footer
	// so the reduced entry set renders whenever the active query matches zero.
	if m.sessionListNoMatches() {
		return renderNoMatchesFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
	// §11.1 empty-sessions state: the standard footer is FULLY REPLACED by the keys
	// relevant with no sessions (`n new in cwd · x projects · / filter · ? help`),
	// drawn from the Sessions keymap descriptor. It is a two-row footer over the SAME
	// border.footer rule, so the swap is height-neutral against the reserved
	// sessionFooterHeight budget. The empty state is Unfiltered, so this guard sits
	// before the filter-state switch (which only handles Filtering/FilterApplied).
	if m.sessionListEmpty() {
		return renderEmptySessionsFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
	// §5 multi-select mode owns the footer (takes over the standard footer) but yields
	// to the focused filter footer: while the `/` input is focused (FilterState ==
	// Filtering) the input-active filter footer renders and the mode footer steps
	// aside (the filter is an inner sub-state). Otherwise — unfiltered OR
	// FilterApplied-in-mode — the multi-select footer renders. So Filtering is checked
	// FIRST, then the mode, then the plain FilterApplied/standard footers.
	if m.sessionList.FilterState() == list.Filtering {
		return renderFilteringFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
	if m.multiSelectMode {
		return renderMultiSelectFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
	switch m.sessionList.FilterState() {
	case list.FilterApplied:
		return renderFilterAppliedFooter(m.contentWidth(), m.canvasMode, m.colourless)
	default:
		return renderSessionsFooter(m.contentWidth(), m.canvasMode, m.colourless)
	}
}

// renderHeader renders the §3.1 shared header block for the model's current
// terminal width and resolved canvas mode (and the NO_COLOR carve-out). It is the
// single render entry point so the composed-view render and the height-budget
// computation (headerHeight) resolve the header against the SAME width/mode.
func (m Model) renderHeader() string {
	return renderHeaderBlock(m.contentWidth(), m.canvasMode, m.colourless)
}

// unsupportedBannerActive reports whether the §6.2 proactive unsupported-terminal
// banner owns the Sessions section-header row: detection has resolved to an
// unsupported resolution (a NULL remote/mosh identity OR a non-NULL undriven
// identity like com.apple.Terminal — the resolution-based DetectUnsupported test,
// NOT IsNull) AND multi-select mode is not active (the §5 `N selected` banner
// outranks it). It is the SINGLE predicate both applySectionHeader (which swaps in
// the banner) and activeNoticeBand (which suppresses the By-Tag signpost while the
// banner owns the row) read, so the two can never drift.
func (m Model) unsupportedBannerActive() bool {
	return m.DetectUnsupported() && !m.multiSelectMode
}

// applySectionHeader swaps the §3.2 / §4.2 restyled section header in place of the
// plain bubbles/list title line (the FIRST line of listView). Because the title
// row already occupies exactly one line in the list's height budget, replacing its
// content — rather than inserting a new row — keeps the one-row-per-delegate
// pagination invariant (§3.5) exact with no budget recompute.
//
// While the filter input is active (FilterState == Filtering) the first line is
// the live filter input, NOT the title — leave it untouched so the user sees what
// they type (the section header is suppressed for that frame, matching the
// pre-reskin behaviour where the input replaced the title). The bubbles/list
// FilterInput is restyled (accent.orange `/ ` prompt + query + cursor) in
// applyCanvasMode, so the live input already reads as the §7 MV filter input.
//
// In the §7.1 list-active mode (FilterState == FilterApplied) bubbles/list renders
// the TITLE (not the input) on the first line; the committed query lives only in
// the (un-rendered) FilterInput value. Swap in the LOCKED accent.orange `/ query`
// header (no cursor, no bg tint) so the section-header position shows the live
// query the same way it did while typing — the cursor-less locked query is what
// signals the list is filtered.
func (m Model) applySectionHeader(listView string) string {
	if m.sessionList.FilterState() == list.Filtering {
		return listView
	}
	// §5 multi-select mode owns the section-header row (a filter-line analogue): swap
	// in the `N selected` / `esc cancel` banner in place of the standard `Sessions`
	// header. This PRECEDES the FilterApplied branch, so a committed/applied filter
	// WHILE in the mode shows the banner (the mode affordance), not the locked query
	// header — the live filter INPUT (Filtering, above) still steps the banner aside.
	if m.multiSelectMode {
		header := renderMultiSelectHeader(
			len(m.selectedSessions),
			m.contentWidth(),
			m.canvasMode,
			m.colourless,
		)
		idx := strings.IndexByte(listView, '\n')
		if idx < 0 {
			return header
		}
		return header + listView[idx:]
	}
	// §6.2 proactive unsupported/NULL banner: once detection has resolved to an
	// unsupported resolution, swap in the `⚠ unsupported terminal — <name> ·
	// <bundleID>` (named) / `⚠ no host-local terminal` (NULL) banner. This PRECEDES
	// the FilterApplied branch (a committed filter WHILE unsupported still shows the
	// banner) but FOLLOWS multi-select (the `N selected` banner outranks it — the
	// gate is unsupportedBannerActive, false in the mode). An in-flight detection
	// (not yet resolved) leaves DetectUnsupported false, so the standard header shows.
	if m.unsupportedBannerActive() {
		header := renderUnsupportedHeader(
			m.detectIdentity.Name,
			m.detectIdentity.BundleID,
			m.contentWidth(),
			m.canvasMode,
			m.colourless,
		)
		idx := strings.IndexByte(listView, '\n')
		if idx < 0 {
			return header
		}
		return header + listView[idx:]
	}
	if m.sessionList.FilterState() == list.FilterApplied {
		header := renderFilterQueryHeader(
			m.sessionList.FilterValue(),
			m.contentWidth(),
			m.canvasMode,
			m.colourless,
		)
		idx := strings.IndexByte(listView, '\n')
		if idx < 0 {
			return header
		}
		return header + listView[idx:]
	}
	header := renderSectionHeader(
		m.sessionListMode,
		m.insideTmux,
		m.currentSession,
		m.visibleSessionRowCount(),
		m.contentWidth(),
		m.canvasMode,
		m.colourless,
	)
	idx := strings.IndexByte(listView, '\n')
	if idx < 0 {
		return header
	}
	return header + listView[idx:]
}

// visibleSessionRowCount is the §3.2 count source: the number of VISIBLE session
// rows in the list — the SAME count the rendered list shows. It counts
// SessionItem rows (HeaderItem group separators are excluded) among the list's
// visible items, so it tracks the inside-tmux exclusion (filteredSessions feeds
// the items), an applied filter (VisibleItems is the filtered set), and the By-Tag
// per-tag row repeats (Pattern B materialises one SessionItem per (session, tag)
// pair, so the count reflects the rows actually drawn).
func (m Model) visibleSessionRowCount() int {
	count := 0
	for _, it := range m.sessionList.VisibleItems() {
		if _, ok := it.(SessionItem); ok {
			count++
		}
	}
	return count
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

// replaceListBodyWithNoMatches swaps the list BODY (every row below the
// title/filter row) for the §7.3 centred no-matches empty state, preserving the
// title/filter row (the first line) byte-for-byte. The body block is rendered at the
// SAME height the bubbles/list body occupies (Height()−1, the list height minus the
// title row), so the composed view height is unchanged and the §3.5 one-row-per-
// delegate pagination invariant is unaffected — the body is empty here anyway, this
// just paints guidance into the rows the empty list would otherwise leave blank.
func (m Model) replaceListBodyWithNoMatches(listView string) string {
	bodyHeight := max(
		// minus the title/filter row
		m.sessionList.Height()-1, 1)
	body := renderNoMatchesBody(m.sessionList.FilterValue(), m.contentWidth(), bodyHeight, m.canvasMode, m.colourless)
	idx := strings.IndexByte(listView, '\n')
	if idx < 0 {
		// Degenerate single-line listView (no body to replace): append the body.
		return listView + "\n" + body
	}
	return listView[:idx+1] + body
}

// byTagSignpostText is the exact, persistent signpost wording rendered in By
// Tag mode when no project carries any tag (spec §11.3 / §5.3 → By Tag with
// zero tags). It states the empty condition ("No tags yet") and points the user
// at where to add tags (the per-project editor: press x for projects, then e to
// edit). Placement: the §11 single-slot info notice band inserted directly
// beneath the title separator, above the section header (the §11 arbiter funnels
// it through renderNoticeBand as the persistent accent.violet info band). The
// per-band tint / on-band text token / left-bar colour are owned by the band
// primitive, NOT a per-row ad-hoc style. The string is verbatim from spec §11.3.
const byTagSignpostText = "No tags yet — add tags in a project's editor: press x for projects, then e to edit"
