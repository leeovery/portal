package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// openTUIFunc is the function used to launch the TUI. Tests override it via
// t.Cleanup-restored assignment to capture arguments without launching the
// real Bubble Tea program.
var openTUIFunc = openTUI

// openPathFunc is the function used to open a session at a resolved path.
// Tests override it via t.Cleanup-restored assignment to capture the resolved
// path without performing real tmux create / exec hand-off (which would
// require a live attached tmux client and replace the test process via
// syscall.Exec).
var openPathFunc = openPath

// openDeps holds injectable dependencies for the open command.
// When nil, real implementations are used.
var openDeps *OpenDeps

// OpenDeps allows injecting dependencies for testing.
type OpenDeps struct {
	AliasLookup  resolver.AliasLookup
	Zoxide       resolver.ZoxideQuerier
	DirValidator resolver.DirValidator
}

// SessionConnector connects the user to a tmux session.
// The implementation differs based on whether Portal is inside or outside tmux.
type SessionConnector interface {
	Connect(name string) error
}

// SwitchClienter defines the interface for switching tmux clients.
type SwitchClienter interface {
	SwitchClient(name string) error
}

// SwitchConnector connects to a session by issuing tmux switch-client.
// Used when Portal is running inside an existing tmux session.
type SwitchConnector struct {
	client SwitchClienter
}

// Connect switches the current tmux client to the named session.
func (sc *SwitchConnector) Connect(name string) error {
	return sc.client.SwitchClient(name)
}

// AttachConnector connects to a session by exec-ing tmux attach-session.
// Used when Portal is running outside tmux (bare shell).
//
// The execer and tmuxPath fields are optional injection seams for tests —
// they are exclusively for unit-test substitution to avoid the real
// syscall.Exec replacing the test process. When either is unset, Connect
// falls back to production defaults (realExecer + exec.LookPath("tmux")).
type AttachConnector struct {
	execer   execer
	tmuxPath string
}

// Connect replaces the current process with tmux attach-session.
//
// The exec'd argv is `tmux attach-session -t =<name>`:
//   - `=` prefixes the target so tmux uses exact-match resolution rather
//     than prefix match — uniform with HasSession / SelectWindow /
//     SelectPane / SwitchClient. See spec § Pre-select + attach sequence
//     > Exact-match target syntax.
func (ac *AttachConnector) Connect(name string) error {
	tmuxPath := ac.tmuxPath
	if tmuxPath == "" {
		p, err := exec.LookPath("tmux")
		if err != nil {
			return fmt.Errorf("tmux not found: %w", err)
		}
		tmuxPath = p
	}
	ex := ac.execer
	if ex == nil {
		ex = &realExecer{}
	}

	// argv[0] is the "tmux" program name; argv[1:] is the tmux subcommand+flags.
	// We log argv[1:] so args renders "attach-session -t =<name>" (target already
	// names tmux) and pass the full argv to Exec.
	argv := []string{"tmux", "attach-session", "-t", "=" + name}

	// Exec-handoff marker (spec § Defensive invariants — exec-handoff markers).
	// syscall.Exec replaces the process image and never returns, so Close never
	// fires and no process: exit line is emitted — this exec line is the terminal
	// marker for the bare-shell attach handoff. marker emitted pre-exec; the log
	// writer is unbuffered (Task 2-7) so the bytes are in the kernel before
	// syscall.Exec replaces the image — no Sync needed.
	log.For("process").Info("exec", "target", "tmux", "args", strings.Join(argv[1:], " "))

	return ex.Exec(tmuxPath, argv, os.Environ())
}

// buildSessionConnector returns the appropriate SessionConnector based on
// whether Portal is running inside an existing tmux session.
func buildSessionConnector(client *tmux.Client) SessionConnector {
	if tmux.InsideTmux() {
		return &SwitchConnector{client: client}
	}
	return &AttachConnector{}
}

var openCmd = &cobra.Command{
	Use:   "open [-e cmd] [destination] [-- cmd args...]",
	Short: "Open the interactive session picker or start a session at a path",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, destination, err := parseCommandArgs(cmd, args)
		if err != nil {
			return err
		}

		if destination == "" {
			return openTUIFunc(cmd, "", command, serverWasStarted(cmd))
		}

		query := destination

		qr, err := buildQueryResolver()
		if err != nil {
			return err
		}

		result, err := qr.Resolve(query)
		if err != nil {
			return err
		}

		switch r := result.(type) {
		case *resolver.PathResult:
			return openPathFunc(cmd, r.Path, command)
		case *resolver.FallbackResult:
			return openTUIFunc(cmd, r.Query, command, false)
		default:
			return fmt.Errorf("unexpected resolution result: %T", result)
		}
	},
}

// parseCommandArgs extracts the command slice and destination from cobra args and flags.
// It validates mutual exclusivity of -e/--exec and --, and rejects empty commands.
func parseCommandArgs(cmd *cobra.Command, args []string) ([]string, string, error) {
	execFlag, _ := cmd.Flags().GetString("exec")
	dashIdx := cmd.ArgsLenAtDash()

	hasExec := cmd.Flags().Changed("exec")
	hasDash := dashIdx >= 0

	if hasExec && hasDash {
		return nil, "", NewUsageError("cannot use both -e/--exec and -- to specify a command")
	}

	if hasExec {
		if execFlag == "" {
			return nil, "", NewUsageError("-e/--exec value must not be empty")
		}
		var dest string
		if len(args) > 0 {
			dest = args[0]
		}
		return []string{execFlag}, dest, nil
	}

	if hasDash {
		dashArgs := args[dashIdx:]
		if len(dashArgs) == 0 {
			return nil, "", NewUsageError("no command specified after --")
		}
		var dest string
		if dashIdx > 0 {
			dest = args[0]
		}
		return dashArgs, dest, nil
	}

	// No command specified
	var dest string
	if len(args) > 0 {
		dest = args[0]
	}
	return nil, dest, nil
}

// sessionCreatorIface creates a tmux session from a directory and returns the session name.
type sessionCreatorIface interface {
	CreateFromDir(dir string, command []string) (string, error)
}

// quickStarter runs the quick-start pipeline and returns exec args.
type quickStarter interface {
	Run(path string, command []string) (*session.QuickStartResult, error)
}

// execer abstracts process replacement for testability.
type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

// realExecer replaces the current process via syscall.Exec.
type realExecer struct{}

// Exec replaces the current process.
func (r *realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

// quickStartAdapter adapts session.QuickStart to the quickStarter interface.
type quickStartAdapter struct {
	qs *session.QuickStart
}

// Run delegates to the underlying QuickStart pipeline.
func (a *quickStartAdapter) Run(path string, command []string) (*session.QuickStartResult, error) {
	return a.qs.Run(path, command)
}

// PathOpener handles creating a new tmux session from a resolved path.
// It branches on insideTmux: inside tmux creates detached then switches;
// outside tmux uses exec handoff with -A flag.
type PathOpener struct {
	insideTmux bool
	creator    sessionCreatorIface
	switcher   SwitchClienter
	qs         quickStarter
	execer     execer
	tmuxPath   string
}

// Open creates a session at the given path and connects to it.
// When command is non-nil, it is passed through to session creation
// for execution as a tmux shell-command.
func (po *PathOpener) Open(resolvedPath string, command []string) error {
	if po.insideTmux {
		sessionName, err := po.creator.CreateFromDir(resolvedPath, command)
		if err != nil {
			return err
		}
		return po.switcher.SwitchClient(sessionName)
	}

	result, err := po.qs.Run(resolvedPath, command)
	if err != nil {
		return err
	}

	// session.QuickStart builds ExecArgs as the chained create-stamp-attach
	// invocation {"tmux", "new-session", "-d", …, ";", "set-option", …, ";",
	// "attach-session", …}. ExecArgs[0] is always the "tmux" program name, so
	// ExecArgs[1:] is the tmux subcommand chain. Drop the program name; never
	// index [1:] on a <1-len slice (ExecArgs is always populated, but be defensive).
	logArgs := result.ExecArgs
	if len(logArgs) > 0 {
		logArgs = logArgs[1:]
	}

	// Exec-handoff marker (spec § Defensive invariants — exec-handoff markers).
	// syscall.Exec replaces the process image and never returns, so Close never
	// fires — this exec line is the terminal marker for the bare-shell create-or-
	// attach handoff. marker emitted pre-exec; the log writer is unbuffered
	// (Task 2-7) so the bytes are in the kernel before syscall.Exec replaces the
	// image — no Sync needed.
	log.For("process").Info("exec", "target", "tmux", "args", strings.Join(logArgs, " "))

	return po.execer.Exec(po.tmuxPath, result.ExecArgs, os.Environ())
}

// openPath creates a new tmux session at the given resolved directory path.
// When inside tmux, it creates the session detached and switches to it.
// When outside tmux, it execs into tmux with the -A flag for atomic create-or-attach.
func openPath(cmd *cobra.Command, resolvedPath string, command []string) error {
	client := tmuxClient(cmd)
	gitResolver := &resolverAdapter{}
	projectsPath, err := projectsFilePath()
	if err != nil {
		return err
	}
	store := project.NewStore(projectsPath)
	gen := session.NewNanoIDGenerator()

	insideTmux := tmux.InsideTmux()

	opener := &PathOpener{
		insideTmux: insideTmux,
		creator:    session.NewSessionCreator(gitResolver, store, client, gen),
		switcher:   client,
		qs:         &quickStartAdapter{qs: session.NewQuickStart(gitResolver, store, client, gen)},
		execer:     &realExecer{},
	}

	if !insideTmux {
		tmuxPath, err := exec.LookPath("tmux")
		if err != nil {
			return fmt.Errorf("tmux not found: %w", err)
		}
		opener.tmuxPath = tmuxPath
	}

	return opener.Open(resolvedPath, command)
}

// resolverAdapter adapts resolver.ResolveGitRoot to the session.GitResolver interface.
type resolverAdapter struct{}

// Resolve resolves a directory to its git repository root.
func (r *resolverAdapter) Resolve(dir string) (string, error) {
	return resolver.ResolveGitRoot(dir, &resolver.RealCommandRunner{})
}

// tuiConfig holds injectable dependencies for building the TUI model.
type tuiConfig struct {
	lister          tui.SessionLister
	killer          tui.SessionKiller
	renamer         tui.SessionRenamer
	projectStore    tui.ProjectStore
	projectEditor   tui.ProjectEditor
	aliasEditor     tui.AliasEditor
	sessionCreator  tui.SessionCreator
	enumerator      tui.TmuxEnumerator
	reader          tui.ScrollbackReader
	previewAttacher tui.PreviewAttacher
	dirReader       session.PaneCurrentPathReader
	dirRunner       resolver.CommandRunner
	initialMode     prefs.SessionListMode
	appearance      prefs.Appearance
	modePersister   tui.ModePersister
	cwd             string
	insideTmux      bool
	currentSession  string
	serverStarted   bool
	// noColor is the NO_COLOR carve-out decision (§2.5), read ONCE here in the cmd
	// layer (os.Getenv) so internal/tui stays env-free. It is the single inheritable
	// colourless flag passed into tui.Deps.NoColor.
	noColor bool
}

// noColorEnabled reports whether the NO_COLOR carve-out (§2.5) is active, per the
// no-color.org convention: the NO_COLOR env var must be PRESENT and NON-EMPTY. A
// set-but-empty NO_COLOR ("") does NOT enable it (an empty value is treated as
// unset by the convention). This is the SINGLE place NO_COLOR is read in the
// cmd/open path; the decision flows as a boolean into tui.Deps so internal/tui
// stays env-free and every canvas-dependent surface inherits one flag.
func noColorEnabled() bool {
	v, ok := os.LookupEnv("NO_COLOR")
	return ok && v != ""
}

// buildTUIModel constructs a tui.Model from the given config and parameters by
// mapping the cmd-local tuiConfig onto the shared tui.Deps seam set and
// delegating to tui.Build — the single model-construction chokepoint shared with
// the offline capture harness (cmd/capturetool). The mapping is field-for-field
// (no nil-guards or option assembly here — Build owns that), so production and
// the harness assemble the identical model.
func buildTUIModel(cfg tuiConfig, initialFilter string, command []string) tui.Model {
	return tui.Build(tui.Deps{
		Lister:          cfg.lister,
		Killer:          cfg.killer,
		Renamer:         cfg.renamer,
		Creator:         cfg.sessionCreator,
		ProjectStore:    cfg.projectStore,
		ProjectEditor:   cfg.projectEditor,
		AliasEditor:     cfg.aliasEditor,
		Enumerator:      cfg.enumerator,
		Reader:          cfg.reader,
		PreviewAttacher: cfg.previewAttacher,
		DirReader:       cfg.dirReader,
		DirRunner:       cfg.dirRunner,
		ModePersister:   cfg.modePersister,
		CWD:             cfg.cwd,
		InitialMode:     cfg.initialMode,
		Appearance:      cfg.appearance,
		InitialFilter:   initialFilter,
		Command:         command,
		ServerStarted:   cfg.serverStarted,
		InsideTmux:      cfg.insideTmux,
		CurrentSession:  cfg.currentSession,
		NoColor:         cfg.noColor,
	})
}

// processTUIResult handles the result of a TUI run.
// If the user selected a session, it connects via the given connector.
// If the user quit without selecting, it returns nil.
func processTUIResult(model tui.Model, connector SessionConnector) error {
	selected := model.Selected()
	if selected == "" {
		return nil
	}
	return connector.Connect(selected)
}

// openTUI launches the interactive session picker with an optional initial filter.
func openTUI(cmd *cobra.Command, initialFilter string, command []string, serverStarted bool) error {
	client := tmuxClient(cmd)
	gitResolver := &resolverAdapter{}
	gen := session.NewNanoIDGenerator()

	store, err := loadProjectStore()
	if err != nil {
		return err
	}

	aliasStore, err := loadAliasStore()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}

	// Resolve stateDir once per Portal process — captured into the
	// scrollback reader adapter at TUI construction so the preview page
	// reads from the same directory the daemon and bootstrap orchestrator
	// write to. state.Dir() is the single source of truth for state-path
	// policy; preview never resolves stateDir on its own.
	stateDir, err := state.Dir()
	if err != nil {
		return fmt.Errorf("failed to resolve state directory: %w", err)
	}
	previewReader := tui.NewProductionScrollbackReader(stateDir)

	// Load the prefs store once at TUI construction; the same *prefs.Store
	// instance serves the initial-mode read here and per-toggle writes via the
	// tui.ModePersister seam. A prefs path-resolution failure must NOT block
	// opening the TUI: swallow it and proceed with a nil persister + the Flat
	// default. prefs.json is deliberately outside the closed state-mutation
	// audit-trail (see internal/prefs), so there is no breadcrumb component to
	// log under here.
	prefsStore, err := loadPrefsStore()
	if err != nil {
		prefsStore = nil
	}
	// Read the persisted initial mode tolerantly. Store.Load collapses every
	// degenerate case (missing / empty / corrupt / unrecognised value) to
	// ModeFlat, so the discarded error is acceptable: a read failure can only
	// yield Flat, which is the first-launch default anyway.
	initialMode := prefs.ModeFlat
	if prefsStore != nil {
		initialMode, _ = prefsStore.Load()
	}
	// Read the persisted appearance preference from the SAME prefsStore instance.
	// LoadAppearance is tolerant — every degenerate case collapses to AppearanceAuto
	// — so the discarded error is acceptable: a read failure can only yield Auto,
	// which is the default detection behaviour anyway. The model only stores it for
	// now; honouring it (skip detection + first-paint wait) is a later task.
	appearance := prefs.AppearanceAuto
	if prefsStore != nil {
		appearance, _ = prefsStore.LoadAppearance()
	}

	// Resolve the connector once. It is used post-TUI by processTUIResult
	// for both Sessions-page Enter and Preview-page Enter. Both
	// *AttachConnector and *SwitchConnector are safe to reuse across
	// calls — neither holds per-attach state — so a single instance per
	// openTUI invocation is sufficient. The preview-page pipeline no
	// longer holds a reference to the connector: it emits a
	// previewAttachSelectedMsg, the model records the selected session +
	// tea.Quit, and the connector handoff happens in processTUIResult
	// after the TUI program shuts down. This prevents the inside-tmux
	// orphan-portal-process regression where switch-client would move
	// the surrounding tmux client while portal kept event-looping with
	// no UI.
	connector := buildSessionConnector(client)

	// The pre-select pipeline WARNs on select-window / select-pane failures
	// (spec § Pre-select + attach sequence > step 2/3) under the preview
	// component. The handler is configured once by main -> log.Init; there is
	// no per-process log open here.
	previewAttacher := tui.NewPreviewAttachPipeline(client, previewLogger)

	cfg := tuiConfig{
		lister:          client,
		killer:          client,
		renamer:         client,
		projectStore:    store,
		projectEditor:   store,
		aliasEditor:     aliasStore,
		sessionCreator:  session.NewSessionCreator(gitResolver, store, client, gen),
		enumerator:      client,
		reader:          previewReader,
		previewAttacher: previewAttacher,
		// Render-layer lazy directory-resolution fallback: client
		// (*tmux.Client) satisfies session.PaneCurrentPathReader via
		// ActivePaneCurrentPath; RealCommandRunner resolves the active pane's
		// cwd to a git-root. The result is cached in-memory only, never stamped.
		dirReader:     client,
		dirRunner:     &resolver.RealCommandRunner{},
		initialMode:   initialMode,
		appearance:    appearance,
		cwd:           cwd,
		serverStarted: serverStarted,
		// NO_COLOR carve-out (§2.5): read the env ONCE here (cmd layer) so
		// internal/tui stays env-free. The single colourless flag flows through
		// tui.Deps.NoColor and is inherited by every canvas-dependent surface.
		noColor: noColorEnabled(),
	}
	// Guard the persister assignment: a typed-nil *prefs.Store boxed into the
	// tui.ModePersister interface would be non-nil, defeating buildTUIModel's nil
	// check. Only wire the persister when the store actually loaded.
	if prefsStore != nil {
		cfg.modePersister = prefsStore
	}

	if tmux.InsideTmux() {
		sessionName, err := client.CurrentSessionName()
		if err == nil && sessionName != "" {
			cfg.insideTmux = true
			cfg.currentSession = sessionName
		}
	}

	m := buildTUIModel(cfg, initialFilter, command)
	// Drain any soft bootstrap warnings accumulated during PersistentPreRunE
	// and stage them on the model. Init folds them into BootstrapCompleteMsg
	// so they ride the loading-page dismissal gate; the model emits them to
	// stderr (with alt-screen toggle) only after the loading page has been
	// dismissed — direct writes during loading would corrupt the rendered UI.
	stageBootstrapWarningsOnModel(&m)
	// Bootstrap-before-TUI ordering: PersistentPreRunE has already run the
	// orchestrator synchronously by the time openTUI is reached. The TUI's
	// Init emits BootstrapCompleteMsg from its first event-loop tick, paired
	// with a 1.2s LoadingMinElapsedMsg tea.Tick. Loading-page dismissal is
	// gated on receipt of both — see internal/tui/model.go transitionFromLoading.
	// Bubble Tea v2 removed the tea.WithAltScreen() program option — the
	// alternate screen is now declared via the tea.View.AltScreen field, set
	// in tui.Model.View(). The launch is otherwise unchanged.
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", finalModel)
	}

	// Restore the terminal's original background on exit (§ background restore-
	// on-exit), BEFORE any session attach/exec handoff. The owned canvas paint
	// sets the terminal background via OSC 11 so it extends into the gutter;
	// terminals that ignore the OSC 111 reset (mosh/Blink) keep the canvas
	// colour after Portal quits, so SET the captured original back. It MUST run
	// before processTUIResult: on the quit-to-shell path (no selection) this is
	// the restore the user sees; on the attach-handoff path it is harmless (tmux
	// takes over the screen). No-op when no OSC 11 response was captured (best-
	// effort fallback to Bubble Tea's own OSC 111 reset). Shared helper with
	// cmd/capturetool so both restore identically; writes to os.Stdout (the
	// program's output).
	tui.RestoreTerminalBackground(os.Stdout, model)

	return processTUIResult(model, connector)
}

// buildQueryResolver creates a QueryResolver with appropriate dependencies.
func buildQueryResolver() (*resolver.QueryResolver, error) {
	if openDeps != nil {
		return resolver.NewQueryResolver(openDeps.AliasLookup, openDeps.Zoxide, openDeps.DirValidator), nil
	}

	store, err := loadAliasStore()
	if err != nil {
		return nil, err
	}

	zoxide := resolver.NewZoxideResolver(&resolver.RealCommandRunner{}, exec.LookPath)
	dirValidator := &resolver.OSDirValidator{}

	return resolver.NewQueryResolver(store, zoxide, dirValidator), nil
}

func init() {
	openCmd.Flags().StringP("exec", "e", "", "command to execute in the new session")
	rootCmd.AddCommand(openCmd)
}
