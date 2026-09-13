package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

var resolveLogger = log.For("resolve")

var themeLogger = log.For("theme")

var openTUIFunc = openTUI

var openPathFunc = openPath

var openSessionFunc = openSession

var openDeps *OpenDeps

// OpenDeps overrides production dependencies; a nil field falls back to the
// production implementation.
type OpenDeps struct {
	SessionLister resolver.SessionLister
	AliasLookup   resolver.AliasLookup
	Zoxide        resolver.ZoxideQuerier
	DirValidator  resolver.DirValidator
	AckWriter     spawn.AckWriter
	ThemeLoader   *theme.Loader
}

type SessionConnector interface {
	Connect(name string) error
}

type SwitchClienter interface {
	SwitchClient(name string) error
}

// SwitchConnector is the connector for use inside an existing tmux session.
type SwitchConnector struct {
	client SwitchClienter
}

func (sc *SwitchConnector) Connect(name string) error {
	return sc.client.SwitchClient(name)
}

// AttachConnector is the connector for use outside tmux (bare shell); it execs,
// so Connect never returns on success. A zero execer or tmuxPath falls back to
// the production default.
type AttachConnector struct {
	execer   execer
	tmuxPath string
}

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

	// attach-session parses its -t as a window or pane target before falling
	// back to a session lookup, so the target is pinned with the trailing
	// separator: without it a period-bearing name is split into window and pane
	// and any live session whose name extends the pre-period component answers
	// instead.
	// Spent as a string because this argv is exec'd directly rather than run
	// through the client; the pinning is the constructor's, not this slice's.
	argv := []string{"tmux", "attach-session", "-t", string(tmux.CoordTargetExact(name))}

	logExecHandoff(argv)

	return ex.Exec(tmuxPath, argv, os.Environ())
}

func buildSessionConnector(client *tmux.Client) SessionConnector {
	if tmux.InsideTmux() {
		return &SwitchConnector{client: client}
	}
	return &AttachConnector{}
}

func openSession(cmd *cobra.Command, name string) error {
	return buildSessionConnector(tmuxClient(cmd)).Connect(name)
}

var openCmd = &cobra.Command{
	Use:   "open [targets…] [-- cmd args…]",
	Short: "Open portals to one or more targets, or launch the interactive picker",
	Long: `Open one or more portals, or launch the interactive session picker.

With no arguments, open launches the TUI picker so you can choose a destination.

A bare target is resolved through the precedence chain — exact session name → path
→ alias → zoxide query, first match wins. An exact session name attaches that
existing session; a path, alias, or zoxide match mints a new session there.

Domain pins skip the precedence chain and force a single domain:
  -s, --session   attach the named session or session glob (never mints)
  -p, --path      mint a new session at the given directory (must exist)
  -a, --alias     mint a new session at the given alias key or key glob
  -z, --zoxide    mint a new session at zoxide's best match

  -f, --filter    skip resolution and open the picker pre-filtered by <text>
                  (mutually exclusive with a target and every domain pin)

A command to run in a freshly minted session is scoped with -e/--exec or after a
-- separator; command scoping applies to mint outcomes only, never to an attach.

Passing two or more targets (or one glob that expands to several sessions) opens a
portal to each: this terminal becomes the first surface and the remaining N−1 open
in host-terminal windows.`,
	Args: validateOpenArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, destination, err := parseCommandArgs(cmd, args)
		if err != nil {
			return err
		}

		// Ahead of every branch that touches tmux, so a malformed --ack is a usage
		// error rather than a tmux failure.
		if ackVal, _ := cmd.Flags().GetString("ack"); ackVal != "" {
			if _, _, ok := spawn.ParseSpawnAckFlag(ackVal); !ok {
				return NewUsageError("open: --ack must be <batch>:<token>")
			}
		}

		// Ahead of the multi-target gate as much as of resolution: a term carrying
		// glob metacharacters is literal text to search for, and the gate would read
		// it as a target expandable to several sessions and burst it.
		if forms := searchFormPositionals(cmd, args); len(forms) > 0 {
			return openTUIFunc(cmd, pickerLanding{filter: resolver.SearchTerm(forms[0]), search: true}, nil, serverWasStarted(cmd))
		}

		// Ahead of resolution and the pin dispatch, so a filter combined with a pin
		// is rejected rather than resolving the pin.
		if cmd.Flags().Changed("filter") {
			filterVal, _ := cmd.Flags().GetString("filter")
			if destination != "" || anyOpenDomainPin(cmd) {
				return NewUsageError("cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)")
			}
			if filterVal == "" {
				return NewUsageError("-f/--filter value must not be empty")
			}
			return openTUIFunc(cmd, pickerLanding{filter: filterVal}, command, serverWasStarted(cmd))
		}

		// Raw argv because cobra collapses repeated same-flag values and splits
		// positionals from flags, losing the order and repeats the burst needs. Must
		// precede the single-pin blocks so a two-pin set bursts rather than hitting
		// the single -s arm.
		ordered := orderedOpenTargets(openOwnArgs())
		if isMultiTarget(ordered) {
			return dispatchOpenBurst(cmd, ordered, command)
		}

		// Must precede the no-target early-return below, so a pin with an empty
		// positional resolves the pin rather than launching the picker.
		for _, flag := range openDomainPinFlags {
			if cmd.Flags().Changed(flag) {
				return resolvePinAndOpen(cmd, flag, pinResolvers[flag], command)
			}
		}

		if destination == "" {
			return openTUIFunc(cmd, pickerLanding{}, command, serverWasStarted(cmd))
		}

		query := destination

		qr, err := buildQueryResolver(cmd)
		if err != nil {
			return err
		}

		result, err := qr.Resolve(query)
		if err != nil {
			return err
		}

		emitResolveDecision(query, result)

		if miss, ok := result.(*resolver.MissResult); ok {
			return singleMissError(miss.Target)
		}
		return openResolved(cmd, result, command)
	},
}

// openDomainPinFlags order is load-bearing: it is the precedence the RunE
// dispatch loop short-circuits in.
var openDomainPinFlags = []string{"session", "path", "alias", "zoxide"}

var pinResolvers = map[string]func(*resolver.QueryResolver, string) (resolver.QueryResult, error){
	"session": (*resolver.QueryResolver).ResolveSessionPin,
	"path":    (*resolver.QueryResolver).ResolvePathPin,
	"alias":   (*resolver.QueryResolver).ResolveAliasPin,
	"zoxide":  (*resolver.QueryResolver).ResolveZoxidePin,
}

func resolvePinAndOpen(cmd *cobra.Command, flag string, resolve func(*resolver.QueryResolver, string) (resolver.QueryResult, error), command []string) error {
	val, _ := cmd.Flags().GetString(flag)
	qr, err := buildQueryResolver(cmd)
	if err != nil {
		return err
	}
	result, err := resolve(qr, val)
	if err != nil {
		return err
	}
	return openResolved(cmd, result, command)
}

// openResolved handles no MissResult — the bare-positional path renders its own
// message and pins never miss — so any other result type is a defensive error.
// A command (-e/--) is mint-scoped: an attach target has no safe injection
// channel (send-keys corrupts a busy pane, respawn-pane -k destroys running work).
func openResolved(cmd *cobra.Command, result resolver.QueryResult, command []string) error {
	switch r := result.(type) {
	case *resolver.SessionResult:
		if len(command) > 0 {
			return NewUsageError(commandAttachOnlyMessage)
		}
		// After the command guard: a command+attach usage error must fire without
		// writing a marker.
		writeAckMarker(cmd)
		return openSessionFunc(cmd, r.Name)
	case *resolver.PathResult:
		writeAckMarker(cmd)
		return openPathFunc(cmd, r.Path, command)
	default:
		return fmt.Errorf("unexpected resolution result: %T", result)
	}
}

// writeAckMarker is best-effort: a failed write still attaches, which the
// parent's poll classifies as a failed window rather than leaving an orphan.
func writeAckMarker(cmd *cobra.Command) {
	ackVal, _ := cmd.Flags().GetString("ack")
	if ackVal == "" {
		return
	}
	batch, token, ok := spawn.ParseSpawnAckFlag(ackVal)
	if !ok {
		return // unreachable: RunE rejects a malformed --ack before dispatch
	}
	if err := buildAckWriter(cmd).Write(batch, token); err != nil {
		spawnLogger.Debug("spawn-ack marker write failed",
			"batch", batch,
			"detail", err.Error(),
		)
	}
}

func buildAckWriter(cmd *cobra.Command) spawn.AckWriter {
	if openDeps != nil && openDeps.AckWriter != nil {
		return openDeps.AckWriter
	}
	client := tmuxClient(cmd)
	return spawn.NewServerOptionAckChannel(client, client)
}

// emitResolveDecision logs one line per resolved query: globs are deterministic
// rather than guesses, so they emit none.
func emitResolveDecision(query string, result resolver.QueryResult) {
	if resolver.HasGlobMeta(query) {
		return
	}
	domain, resolvedPath := resolveDecision(result)
	resolveLogger.Info("resolved", "target", query, "domain", domain.String(), "resolved_path", resolvedPath)
}

func resolveDecision(result resolver.QueryResult) (domain resolver.Domain, resolvedPath string) {
	switch r := result.(type) {
	case *resolver.SessionResult:
		return r.Domain, r.Name
	case *resolver.PathResult:
		return r.Domain, r.Path
	case *resolver.MissResult:
		return resolver.DomainMiss, ""
	default:
		return "", ""
	}
}

// logExecHandoff must be called immediately before syscall.Exec: that call
// replaces the process image and never returns, so log.Close never fires and
// this is the terminal log line for the handoff.
func logExecHandoff(argv []string) {
	args := argv
	if len(args) > 0 {
		args = args[1:]
	}
	log.For("process").Info("exec", "target", "tmux", "args", strings.Join(args, " "))
}

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

	var dest string
	if len(args) > 0 {
		dest = args[0]
	}
	return nil, dest, nil
}

type sessionCreatorIface interface {
	CreateFromDir(dir string, command []string) (string, error)
}

type quickStarter interface {
	Run(path string, command []string) (*session.QuickStartResult, error)
}

type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

type realExecer struct{}

func (r *realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

type quickStartAdapter struct {
	qs *session.QuickStart
}

func (a *quickStartAdapter) Run(path string, command []string) (*session.QuickStartResult, error) {
	return a.qs.Run(path, command)
}

type PathOpener struct {
	insideTmux bool
	creator    sessionCreatorIface
	switcher   SwitchClienter
	qs         quickStarter
	execer     execer
	tmuxPath   string
}

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

	logExecHandoff(result.ExecArgs)

	return po.execer.Exec(po.tmuxPath, result.ExecArgs, os.Environ())
}

func openPath(cmd *cobra.Command, resolvedPath string, command []string) error {
	client := tmuxClient(cmd)
	gitResolver := &resolverAdapter{}
	projectsPath, err := projectsFilePath()
	if err != nil {
		return err
	}
	store := project.NewStore(projectsPath)
	gen := nanoid.NewGenerator()

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

type resolverAdapter struct{}

func (r *resolverAdapter) Resolve(dir string) (string, error) {
	return resolver.ResolveGitRoot(dir, &resolver.RealCommandRunner{})
}

type tuiConfig struct {
	lister           tui.SessionLister
	killer           tui.SessionKiller
	renamer          tui.SessionRenamer
	projectStore     tui.ProjectStore
	projectEditor    tui.ProjectEditor
	aliasEditor      tui.AliasEditor
	sessionCreator   tui.SessionCreator
	enumerator       tui.TmuxEnumerator
	reader           tui.ScrollbackReader
	previewAttacher  tui.PreviewAttacher
	dirReader        session.PaneCurrentPathReader
	dirRunner        resolver.CommandRunner
	initialMode      prefs.SessionListMode
	theme            theme.Nomination
	themeKeys        theme.RawKeys
	themeSource      tui.ThemeSource
	modePersister    tui.ModePersister
	themePersister   tui.ThemePersister
	detector         tui.TerminalDetector
	resolve          spawn.AdapterResolver
	sessionExists    func(string) bool
	ackChannel       spawn.AckChannelFull
	spawnExe         spawn.ExecutableResolver
	spawnGetenv      func(string) string
	spawnLogger      *slog.Logger
	cwd              string
	insideTmux       bool
	currentSession   string
	serverStarted    bool
	progressReceiver tea.Cmd
	noColor          bool
}

// noColorEnabled honours the NO_COLOR convention: the variable must be present
// and non-empty, so a set-but-empty value does not enable it.
func noColorEnabled() bool {
	v, ok := os.LookupEnv("NO_COLOR")
	return ok && v != ""
}

// newThemeLoader is per call rather than package-level: a Loader owns the event
// logger's per-process dedup state, so one loader per launch is one dedup scope.
func newThemeLoader() theme.Loader {
	return theme.NewLoader(theme.NewEventLogger(themeLogger))
}

func buildThemeLoader() theme.Loader {
	if openDeps != nil && openDeps.ThemeLoader != nil {
		return *openDeps.ThemeLoader
	}
	return newThemeLoader()
}

// themeResolution reads the keys as handed in — the post-translation in-memory
// value rather than a second disk read, so a migrated user renders their pin on
// the launch that translates it. A non-nil error means even the fallback theme
// did not load: nothing is honest to paint, so the caller must construct nothing.
func themeResolution(keys prefs.ThemeKeys, loader theme.Loader) (theme.Resolution, theme.RawKeys, error) {
	setting, raw := theme.ResolveSetting(themeRawKeys(keys))

	// A themes dir that will not resolve degrades to "" rather than blocking the
	// launch: the embedded built-ins are reachable with no path at all.
	themesDir, _ := themesDirPath()

	resolution, err := loader.ResolveNomination(setting, themesDir)
	if err != nil {
		return theme.Resolution{}, theme.RawKeys{}, err
	}
	return resolution, raw, nil
}

func buildTUIModel(cfg tuiConfig, landing pickerLanding, command []string) tui.Model {
	deps := tui.Deps{
		Lister:           cfg.lister,
		Killer:           cfg.killer,
		Renamer:          cfg.renamer,
		Creator:          cfg.sessionCreator,
		ProjectStore:     cfg.projectStore,
		ProjectEditor:    cfg.projectEditor,
		AliasEditor:      cfg.aliasEditor,
		Enumerator:       cfg.enumerator,
		Reader:           cfg.reader,
		PreviewAttacher:  cfg.previewAttacher,
		DirReader:        cfg.dirReader,
		DirRunner:        cfg.dirRunner,
		ModePersister:    cfg.modePersister,
		ThemePersister:   cfg.themePersister,
		CWD:              cfg.cwd,
		InitialMode:      cfg.initialMode,
		Theme:            cfg.theme,
		ThemeKeys:        cfg.themeKeys,
		ThemeSource:      cfg.themeSource,
		Command:          command,
		ServerStarted:    cfg.serverStarted,
		InsideTmux:       cfg.insideTmux,
		CurrentSession:   cfg.currentSession,
		NoColor:          cfg.noColor,
		ProgressReceiver: cfg.progressReceiver,
		Detector:         cfg.detector,
		Resolve:          cfg.resolve,
		SessionExists:    cfg.sessionExists,
		AckChannel:       cfg.ackChannel,
		SpawnExe:         cfg.spawnExe,
		SpawnGetenv:      cfg.spawnGetenv,
		SpawnLogger:      cfg.spawnLogger,
	}
	if landing.search {
		deps.Search = &tui.SearchForm{Term: landing.filter}
	} else {
		deps.InitialFilter = landing.filter
	}
	return tui.Build(deps)
}

func processTUIResult(model tui.Model, connector SessionConnector) error {
	if fatal := model.FatalError(); fatal != nil {
		return fatal
	}
	selected := model.Selected()
	if selected == "" {
		return nil
	}
	return connector.Connect(selected)
}

func openTUI(cmd *cobra.Command, landing pickerLanding, command []string, serverStarted bool) error {
	client := tmuxClient(cmd)
	gitResolver := &resolverAdapter{}
	gen := nanoid.NewGenerator()

	var pipe *bootstrapProgressPipe
	if deferred := deferredBootstrapFromContext(cmd); deferred != nil {
		pipe = newBootstrapProgressPipe()
		pipe.start(cmd.Context(), deferred.runner)
		// Forced because a bootstrap is in progress, not because the server was cold
		// — a warm-unlatched server reaches this route too. Its only effect is
		// parking the model on the loading page.
		serverStarted = true
	}

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

	stateDir, err := state.Dir()
	if err != nil {
		return fmt.Errorf("failed to resolve state directory: %w", err)
	}
	previewReader := tui.NewProductionScrollbackReader(stateDir)

	// A prefs path-resolution failure must not block opening the picker: degrade to
	// a nil persister, zero keys (the shipped pair) and the Flat default.
	loadedPrefs, err := loadPrefsStore()
	if err != nil {
		loadedPrefs = prefsLoad{}
	}
	prefsStore := loadedPrefs.Store
	initialMode := prefs.ModeFlat
	if prefsStore != nil {
		initialMode, _ = prefsStore.Load()
	}
	themeLoader := buildThemeLoader()
	resolution, themeKeys, err := themeResolution(loadedPrefs.Keys, themeLoader)
	if err != nil {
		return err
	}

	// Built now, connected only after the TUI shuts down: switching the tmux
	// client while portal is still event-looping leaves an orphan with no UI.
	connector := buildSessionConnector(client)

	previewAttacher := tui.NewPreviewAttachPipeline(client, previewLogger)

	spawnSeams := buildProductionSpawnSeams(client)

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
		dirReader:       client,
		dirRunner:       &resolver.RealCommandRunner{},
		initialMode:     initialMode,
		theme:           resolution.Nomination,
		themeKeys:       themeKeys,
		themeSource:     newThemeSource(themeLoader),
		cwd:             cwd,
		serverStarted:   serverStarted,
		detector:        spawnSeams.Detector,
		resolve:         spawnSeams.Resolve,
		sessionExists:   spawnSeams.Exists,
		ackChannel:      spawnSeams.Ack,
		spawnExe:        spawnSeams.Exe,
		spawnGetenv:     spawnSeams.Getenv,
		spawnLogger:     spawnSeams.Logger,
		noColor:         noColorEnabled(),
	}
	// On the concurrent route the channel owns the terminal BootstrapCompleteMsg;
	// Init does not synthesize one.
	if pipe != nil {
		cfg.progressReceiver = pipe.receiver()
	}
	// A typed-nil *prefs.Store boxed into the interface reads as non-nil, defeating
	// buildTUIModel's nil check, and the theme persister wrapping it would panic on
	// every write. Wire only when the store actually loaded.
	if prefsStore != nil {
		cfg.modePersister = prefsStore
		cfg.themePersister = newThemePersister(prefsStore)
	}

	if tmux.InsideTmux() {
		sessionName, err := client.CurrentSessionName()
		if err == nil && sessionName != "" {
			cfg.insideTmux = true
			cfg.currentSession = sessionName
		}
	}

	m := buildTUIModel(cfg, landing, command)
	// Staged rather than written: the model emits them only after the loading page
	// is dismissed, because a direct write during loading corrupts the rendered UI.
	stageBootstrapWarningsOnModel(&m)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	model, ok := finalModel.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", finalModel)
	}

	// Before the attach handoff, while the screen is still ours: terminals that
	// ignore Bubble Tea's OSC 111 reset keep the canvas colour after Portal quits.
	tui.RestoreTerminalBackground(os.Stdout, model)

	return processTUIResult(model, connector)
}

func buildQueryResolver(cmd *cobra.Command) (*resolver.QueryResolver, error) {
	if openDeps != nil {
		return resolver.NewQueryResolver(openDeps.SessionLister, openDeps.AliasLookup, openDeps.Zoxide, openDeps.DirValidator), nil
	}

	store, err := loadAliasStore()
	if err != nil {
		return nil, err
	}

	zoxide := resolver.NewZoxideResolver(&resolver.RealCommandRunner{}, exec.LookPath)
	dirValidator := &resolver.OSDirValidator{}

	return resolver.NewQueryResolver(tmuxClient(cmd), store, zoxide, dirValidator), nil
}

func init() {
	openCmd.Flags().StringP("exec", "e", "", "command to execute in the new session")
	openCmd.Flags().StringP("filter", "f", "", "open the picker pre-filtered by <text> (skips resolution)")
	openCmd.Flags().StringP("session", "s", "", "attach the named session or session glob (session-domain; never mints)")
	openCmd.Flags().StringP("path", "p", "", "mint a new session at the given directory (path-domain; dir must exist)")
	openCmd.Flags().StringP("alias", "a", "", "mint a new session at the given alias key or key glob (alias-domain)")
	openCmd.Flags().StringP("zoxide", "z", "", "mint a new session at zoxide's best match (zoxide-domain; explicit error if zoxide is not installed)")
	openCmd.Flags().String("ack", "", "internal: <batch>:<token> — write the @portal-spawn-<batch>-<token> ack marker before the attach/mint handoff")
	_ = openCmd.Flags().MarkHidden("ack")

	// -p/--path and -z/--zoxide deliberately register no completer, so cobra emits
	// ShellCompDirectiveDefault and the shell provides file/default completion.
	openCmd.ValidArgsFunction = func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeSessionNames(toComplete)
	}
	_ = openCmd.RegisterFlagCompletionFunc("session", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeSessionNames(toComplete)
	})
	_ = openCmd.RegisterFlagCompletionFunc("alias", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeAliasKeys(toComplete)
	})

	rootCmd.AddCommand(openCmd)
}
