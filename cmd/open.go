package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
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
// The exec'd argv is `tmux attach-session -A -t =<name>`:
//   - `-A` enables tmux's atomic create-or-attach semantics (the session
//     is created if absent, attached otherwise). This is also the residual
//     fallback for the TOCTOU window between has-session and connector
//     handoff described in spec § Session-killed-externally bail path.
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
	return ex.Exec(tmuxPath, []string{"tmux", "attach-session", "-A", "-t", "=" + name}, os.Environ())
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

// osDirLister adapts browser.ListDirectories to the tui.DirLister interface.
type osDirLister struct{}

// ListDirectories lists directory entries at the given path.
func (o *osDirLister) ListDirectories(path string, showHidden bool) ([]browser.DirEntry, error) {
	return browser.ListDirectories(path, showHidden)
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
	dirLister       tui.DirLister
	enumerator      tui.TmuxEnumerator
	reader          tui.ScrollbackReader
	previewAttacher tui.PreviewAttacher
	cwd             string
	insideTmux      bool
	currentSession  string
	serverStarted   bool
}

// buildTUIModel constructs a tui.Model from the given config and parameters.
func buildTUIModel(cfg tuiConfig, initialFilter string, command []string) tui.Model {
	opts := []tui.Option{
		tui.WithKiller(cfg.killer),
		tui.WithRenamer(cfg.renamer),
		tui.WithProjectStore(cfg.projectStore),
		tui.WithSessionCreator(cfg.sessionCreator),
		tui.WithDirLister(cfg.dirLister, cfg.cwd),
		tui.WithCWD(cfg.cwd),
	}
	if cfg.serverStarted {
		opts = append(opts, tui.WithServerStarted(true))
	}
	if cfg.projectEditor != nil {
		opts = append(opts, tui.WithProjectEditor(cfg.projectEditor))
	}
	if cfg.aliasEditor != nil {
		opts = append(opts, tui.WithAliasEditor(cfg.aliasEditor))
	}
	if cfg.enumerator != nil {
		opts = append(opts, tui.WithEnumerator(cfg.enumerator))
	}
	if cfg.reader != nil {
		opts = append(opts, tui.WithScrollbackReader(cfg.reader))
	}
	if cfg.previewAttacher != nil {
		opts = append(opts, tui.WithPreviewAttachPipeline(cfg.previewAttacher))
	}
	m := tui.New(cfg.lister, opts...)
	if len(command) > 0 {
		m = m.WithCommand(command)
	}
	if initialFilter != "" {
		m = m.WithInitialFilter(initialFilter)
	}
	if cfg.insideTmux && cfg.currentSession != "" {
		m = m.WithInsideTmux(cfg.currentSession)
	}
	return m
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

	// Resolve the connector once and share it with both the preview-page
	// Enter pipeline and the post-TUI Sessions-page handoff. Both
	// *AttachConnector and *SwitchConnector are safe to reuse across
	// calls — neither holds per-attach state — so a single instance per
	// openTUI invocation is sufficient.
	connector := buildSessionConnector(client)

	// Open a best-effort logger so the pre-select pipeline can WARN on
	// select-window / select-pane failures (spec § Pre-select + attach
	// sequence > step 2/3). Log opening is non-fatal: the pipeline
	// honours *state.Logger's nil-receiver no-op contract, so a failed
	// open passes nil through and the rest of openTUI proceeds.
	previewLogger, err := state.OpenLogger(state.PortalLog(stateDir), false)
	if err != nil {
		previewLogger = nil
	}
	previewAttacher := tui.NewPreviewAttachPipeline(client, connector, previewLogger)

	cfg := tuiConfig{
		lister:          client,
		killer:          client,
		renamer:         client,
		projectStore:    store,
		projectEditor:   store,
		aliasEditor:     aliasStore,
		sessionCreator:  session.NewSessionCreator(gitResolver, store, client, gen),
		dirLister:       &osDirLister{},
		enumerator:      client,
		reader:          previewReader,
		previewAttacher: previewAttacher,
		cwd:             cwd,
		serverStarted:   serverStarted,
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
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", finalModel)
	}

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
