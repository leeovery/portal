package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

var resolveLogger = log.For("resolve")

var themeLogger = log.For("theme")

var openTUIFunc = openTUI

var openPathFunc = openPath

var openSessionFunc = openSession

var runProgramFunc = runProgram

// runProgram runs the picker on the terminal until it quits.
func runProgram(m tui.Model) (tea.Model, error) {
	return tea.NewProgram(m).Run()
}

var openDeps *OpenDeps

// OpenDeps overrides production dependencies. Once it is staged, the
// resolver takes its alias, zoxide and directory seams as they stand; every
// other nil field falls back to the production implementation.
type OpenDeps struct {
	// SessionLister, when nil, lists the backend's user sessions.
	SessionLister resolver.SessionLister
	// Kept, when nil, judges a saved session kept by sessions.json and the
	// backend's sessions.
	Kept         resolver.KeptLookup
	AliasLookup  resolver.AliasLookup
	Zoxide       resolver.ZoxideQuerier
	DirValidator resolver.DirValidator
	ThemeLoader  *theme.Loader
	// Execer replaces the exec that hands the terminal to the backend's
	// attach command.
	Execer execer
	// Runner replaces runner.Run, which attaches the terminal to a session.
	Runner func(context.Context, runner.Config) (runner.Outcome, error)
	// Runners replaces the runner registry under the state directory.
	Runners runnerRegistry
	// AckWriter replaces the spawn-ack channel under the state directory.
	AckWriter spawn.AckWriter
}

// SessionConnector connects the caller's terminal to a session.
type SessionConnector interface {
	Connect(name string) error
}

// connectorFunc is a SessionConnector that is a function.
type connectorFunc func(name string) error

func (f connectorFunc) Connect(name string) error { return f(name) }

func openSession(cmd *cobra.Command, name string) error {
	b, err := attachingBackend()
	if err != nil {
		return err
	}
	return newSessionConnector(cmd, b).Connect(name)
}

var openCmd = &cobra.Command{
	Use:   "open [targets…] [-- cmd args…]",
	Short: "Open portals to one or more targets, or launch the interactive picker",
	Long: `Open one or more portals, or launch the interactive session picker.

With no arguments, open launches the TUI picker so you can choose a destination.

A bare target is resolved through the precedence chain — exact session name → kept
saved session → path → alias → zoxide query, first match wins. An exact session
name attaches that existing session; the name of a saved session that isn't
running, which the next restore would bring back, brings it back and attaches it;
a path, alias, or zoxide match mints a new session there.

Domain pins skip the precedence chain and force a single domain:
  -s, --session   attach the named session or session glob (never mints); an
                  exact name that is a kept saved session is brought back first
  -p, --path      mint a new session at the given directory (must exist)
  -a, --alias     mint a new session at the given alias key or key glob
  -z, --zoxide    mint a new session at zoxide's best match

  -f, --filter    skip resolution and open the picker pre-filtered by <text>
                  (mutually exclusive with a target and every domain pin)

A positional beginning with / and containing no further / searches your live
sessions instead of resolving:
  open /term      search live sessions for "term"
  open /          open the picker with the filter empty and ready to type

The term is matched case-folded, as a contiguous run, against each live session's
label, name and directory. Exactly one match attaches that session outright;
no match or several open the picker pre-filtered by the term — never an error.
A single-segment absolute directory is read as a search, not a path: mint there
with -p /tmp.

A search composes with nothing — another target, a command, -f, a domain pin or a
second search on the same line is a usage error.

-f always opens the picker, which makes it the form for a script or a keybinding;
/term takes you straight to the session when only one matches, which makes it the
interactive form.

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

		// Ahead of resolution, so a malformed --ack is a usage error rather than
		// an open that acks nothing.
		if _, _, ok := spawnAck(cmd); cmd.Flags().Changed("ack") && !ok {
			return NewUsageError("open: --ack must be <batch>:<token>")
		}

		// Ahead of the multi-target gate as much as of resolution: a term carrying
		// glob metacharacters is literal text to search for, and the gate would read
		// it as a target expandable to several sessions and burst it.
		if forms := searchFormPositionals(cmd, args); len(forms) > 0 {
			return runSearchForm(cmd, resolver.SearchTerm(forms[0]))
		}

		// Ahead of resolution and the pin dispatch: -f always opens the picker, and
		// the Args validator has already refused every line where it would not.
		if cmd.Flags().Changed("filter") {
			filterVal, _ := cmd.Flags().GetString("filter")
			return openTUIFunc(cmd, pickerLanding{filter: filterVal}, command)
		}

		// Raw argv because cobra collapses repeated same-flag values and splits
		// positionals from flags, losing the order and repeats the burst needs. Must
		// precede the single-pin blocks so a two-pin set bursts rather than hitting
		// the single -s arm. A window a burst spawned names its one surface
		// exactly, so it never bursts again: a name with glob characters would
		// expand, opening more windows than were asked for. No burst spawns a
		// window naming more than one, which would ack one and drop the rest.
		ordered := orderedOpenTargets(openOwnArgs())
		acked := cmd.Flags().Changed("ack")
		if acked && len(ordered) > 1 {
			return NewUsageError("open: cannot use --ack with more than one target")
		}
		if !acked && isMultiTarget(ordered) {
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
			return openTUIFunc(cmd, pickerLanding{}, command)
		}

		qr, err := buildQueryResolver(cmd)
		if err != nil {
			return err
		}
		return openBare(cmd, qr, destination, command)
	},
}

// openBare opens what qr resolves the bare positional query to, logging the
// decision, a kept session brought back first.
func openBare(cmd *cobra.Command, qr *resolver.QueryResolver, query string, command []string) error {
	result, err := qr.Resolve(query)
	if err != nil {
		return err
	}
	emitResolveDecision(query, result)
	if miss, ok := result.(*resolver.MissResult); ok {
		return singleMissError(miss.Target)
	}
	return openOr(cmd, result, command, func() error {
		return openBare(cmd, qr.WithKept(nil), query, command)
	})
}

// validateFilterFlag refuses the lines -f cannot open a picker for: it is the
// whole-picker form, so it composes with neither a target nor a domain pin, and
// it needs text to filter by. destination is the positional target the line
// carries, empty when it carries none.
func validateFilterFlag(cmd *cobra.Command, destination string) error {
	if !cmd.Flags().Changed("filter") {
		return nil
	}
	if destination != "" || anyOpenDomainPin(cmd) {
		return NewUsageError("cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)")
	}
	if filterVal, _ := cmd.Flags().GetString("filter"); filterVal == "" {
		return NewUsageError("-f/--filter value must not be empty")
	}
	return nil
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
	return openPinned(cmd, resolve, val, command)
}

// openPinned opens what resolve, a pin's resolver, finds for val, a kept
// session brought back first.
func openPinned(cmd *cobra.Command, resolve func(*resolver.QueryResolver, string) (resolver.QueryResult, error), val string, command []string) error {
	qr, err := buildQueryResolver(cmd)
	if err != nil {
		return err
	}
	return openPinnedBy(cmd, qr, resolve, val, command)
}

// openPinnedBy is openPinned over qr.
func openPinnedBy(cmd *cobra.Command, qr *resolver.QueryResolver, resolve func(*resolver.QueryResolver, string) (resolver.QueryResult, error), val string, command []string) error {
	result, err := resolve(qr, val)
	if err != nil {
		return err
	}
	return openOr(cmd, result, command, func() error {
		return openPinnedBy(cmd, qr.WithKept(nil), resolve, val, command)
	})
}

// openResolved handles no MissResult — the bare-positional path renders its own
// message and pins never miss — so any other result type is a defensive error.
// A command (-e/--) is mint-scoped: an attach target is already running its
// own program, so there is nothing safe to inject it into.
func openResolved(cmd *cobra.Command, result resolver.QueryResult, command []string) error {
	switch r := result.(type) {
	case *resolver.SessionResult:
		if len(command) > 0 {
			return NewUsageError(commandAttachOnlyMessage)
		}
		return openSessionFunc(cmd, r.Name)
	case *resolver.KeptResult:
		return openKept(cmd, r.Name, command)
	case *resolver.PathResult:
		return openPathFunc(cmd, r.Path, command)
	default:
		return fmt.Errorf("unexpected resolution result: %T", result)
	}
}

// emitResolveDecision logs one line per resolution of a query, so a kept
// target that is kept no longer by its bring-back logs a second, for its
// fall-through. Globs are deterministic rather than guesses, so they emit
// none.
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
	case *resolver.KeptResult:
		return resolver.DomainKept, r.Name
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
	log.For("process").Info("exec", "target", "zmx", "args", shellquote.Readable(args))
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

type execer interface {
	Exec(argv0 string, argv []string, envv []string) error
}

type realExecer struct{}

func (realExecer) Exec(argv0 string, argv []string, envv []string) error {
	return syscall.Exec(argv0, argv, envv)
}

func openPath(cmd *cobra.Command, resolvedPath string, command []string) error {
	b, err := attachingBackend()
	if err != nil {
		return err
	}
	projectsPath, err := projectsFilePath()
	if err != nil {
		return err
	}
	creator := newIndexingCreator(cmd.Context(), project.NewStore(projectsPath), b, "cli")
	name, err := creator.CreateFromDir(resolvedPath, command)
	if err != nil {
		return err
	}
	return newSessionConnector(cmd, b).Connect(name)
}

type resolverAdapter struct{}

func (r *resolverAdapter) Resolve(dir string) (string, error) {
	return resolver.ResolveGitRoot(dir, &resolver.RealCommandRunner{})
}

type tuiConfig struct {
	lister           tui.SessionLister
	killer           tui.SessionKiller
	askedKiller      tui.SessionKiller
	labeller         tui.SessionLabeller
	projectStore     tui.ProjectStore
	projectEditor    tui.ProjectEditor
	aliasEditor      tui.AliasEditor
	sessionCreator   tui.SessionCreator
	history          tui.HistoryReader
	transcripts      tui.TranscriptReader
	dirRunner        resolver.CommandRunner
	pendingReader    tui.PendingResumeReader
	hydrating        tui.HydratingReader
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
	spawnEnv         spawn.WindowEnv
	spawnLogger      *slog.Logger
	cwd              string
	currentSession   string
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

// wireStateReads wires the picker's reads of the state dir: the sessions
// waiting on their resume panel, the settle after a restore, and a waiting
// session's transcript, which its preview shows.
func (cfg *tuiConfig) wireStateReads() {
	cfg.pendingReader, cfg.hydrating = pickerFlagReaders()
	cfg.transcripts = pickerTranscripts()
}

func buildTUIModel(cfg tuiConfig, landing pickerLanding, command []string) tui.Model {
	deps := tui.Deps{
		Lister:           cfg.lister,
		Killer:           cfg.killer,
		AskedKiller:      cfg.askedKiller,
		Labeller:         cfg.labeller,
		Creator:          cfg.sessionCreator,
		CreateFatal:      createFatal,
		ProjectStore:     cfg.projectStore,
		ProjectEditor:    cfg.projectEditor,
		AliasEditor:      cfg.aliasEditor,
		History:          cfg.history,
		Transcripts:      cfg.transcripts,
		DirRunner:        cfg.dirRunner,
		PendingReader:    cfg.pendingReader,
		Hydrating:        cfg.hydrating,
		ModePersister:    cfg.modePersister,
		ThemePersister:   cfg.themePersister,
		CWD:              cfg.cwd,
		InitialMode:      cfg.initialMode,
		Theme:            cfg.theme,
		ThemeKeys:        cfg.themeKeys,
		ThemeSource:      cfg.themeSource,
		Command:          command,
		CurrentSession:   cfg.currentSession,
		NoColor:          cfg.noColor,
		ProgressReceiver: cfg.progressReceiver,
		Detector:         cfg.detector,
		Resolve:          cfg.resolve,
		SessionExists:    cfg.sessionExists,
		AckChannel:       cfg.ackChannel,
		SpawnExe:         cfg.spawnExe,
		SpawnEnv:         cfg.spawnEnv,
		SpawnLogger:      cfg.spawnLogger,
		Flash:            landing.flash,
		KillAsk:          landing.kill,
	}
	deps.PendingSettleBound = pendingSettleBound
	if landing.search != nil {
		deps.Search = landing.search
	} else {
		deps.InitialFilter = landing.filter
	}
	return tui.Build(deps)
}

func processTUIResult(model tui.Model, connector SessionConnector) error {
	if fatal := model.FatalError(); fatal != nil {
		return fatal
	}
	// A session list that could not be read has searched or shown nothing, so
	// it is an ordinary error rather than a bootstrap fatal: the backend's own
	// words on stderr and a non-zero exit, with no error frame and nothing
	// attached.
	if searchErr := model.SearchError(); searchErr != nil {
		return searchErr
	}
	if listErr := model.ListError(); listErr != nil {
		return listErr
	}
	selected := model.Selected()
	if selected == "" {
		return nil
	}
	return connector.Connect(selected)
}

// finishTUI runs the teardown tail in its load-bearing order.
func finishTUI(model tui.Model, connector SessionConnector, canvas, warnings io.Writer) error {
	// Before the attach handoff, while the screen is still ours: terminals that
	// ignore Bubble Tea's OSC 111 reset keep the canvas colour after Portal quits.
	tui.RestoreTerminalBackground(canvas, model)
	// Before the connect: an attach Portal cannot run itself execs and never
	// returns.
	tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())

	return processTUIResult(model, connector)
}

func openTUI(cmd *cobra.Command, landing pickerLanding, command []string) error {
	b, err := attachingBackend()
	if err != nil {
		return err
	}
	launch := pickerLaunch{
		landing:   landing,
		command:   command,
		bootstrap: deferredBootstrapFromContext(cmd),
	}
	return runPicker(cmd, b, launch, connectorFunc(newSessionConnector(cmd, b).ConnectPick))
}

// pickerLaunch is what one run of the picker opens with.
type pickerLaunch struct {
	landing pickerLanding
	command []string
	// bootstrap is the deferred bootstrap the picker runs behind its loading
	// page; nil when none is owed.
	bootstrap *deferredBootstrap
}

// runPicker runs the picker, tears it down and connects its pick. The
// connector is handed in, since the attach takes over the terminal the
// picker holds and so runs only once the picker has shut down. A picker with
// a bootstrap to run opens on its loading page while it runs.
func runPicker(cmd *cobra.Command, b backend.Backend, launch pickerLaunch, connector SessionConnector) error {
	var pipe *bootstrapProgressPipe
	if launch.bootstrap != nil {
		pipe = newBootstrapProgressPipe()
		pipe.start(cmd.Context(), launch.bootstrap.runner)
		defer pipe.unwind()
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

	spawnSeams := buildProductionSpawnSeams(cmd.Context(), b, stateRunners{})

	cfg := tuiConfig{
		lister:         pickerLister{ctx: cmd.Context(), b: b},
		killer:         pickerKiller{ctx: cmd.Context(), b: b},
		askedKiller:    askedKiller(cmd.Context(), b),
		labeller:       pickerLabeller{ctx: cmd.Context(), b: b},
		projectStore:   store,
		projectEditor:  store,
		aliasEditor:    aliasStore,
		sessionCreator: newIndexingCreator(cmd.Context(), store, b, "tui"),
		history:        b,
		dirRunner:      &resolver.RealCommandRunner{},
		initialMode:    initialMode,
		theme:          resolution.Nomination,
		themeKeys:      themeKeys,
		themeSource:    newThemeSource(themeLoader),
		cwd:            cwd,
		detector:       spawnSeams.Detector,
		resolve:        spawnSeams.Resolve,
		sessionExists:  spawnSeams.Exists,
		ackChannel:     spawnSeams.Ack,
		spawnExe:       spawnSeams.Exe,
		spawnEnv:       spawnSeams.Env,
		spawnLogger:    spawnSeams.Logger,
		noColor:        noColorEnabled(),
	}
	// The concurrent route: the picker opens on its loading page, and the
	// channel owns the terminal BootstrapCompleteMsg.
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

	cfg.currentSession = b.Current()
	cfg.wireStateReads()

	m := buildTUIModel(cfg, launch.landing, launch.command)
	// Staged rather than written: a direct write while the TUI holds the screen
	// corrupts the rendered frame.
	stageBootstrapWarningsOnModel(&m)

	finalModel, err := runProgramFunc(m)
	if err != nil {
		return err
	}

	model, ok := finalModel.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected model type: %T", finalModel)
	}

	return finishTUI(model, connector, os.Stdout, cmd.ErrOrStderr())
}

func buildQueryResolver(cmd *cobra.Command) (*resolver.QueryResolver, error) {
	sessions := resolverSessions(cmd)
	if openDeps != nil {
		return resolver.NewQueryResolver(sessions, openDeps.AliasLookup, openDeps.Zoxide, openDeps.DirValidator).WithKept(resolverKept(cmd)), nil
	}

	store, err := loadAliasStore()
	if err != nil {
		return nil, err
	}

	zoxide := resolver.NewZoxideResolver(&resolver.RealCommandRunner{}, exec.LookPath)
	dirValidator := &resolver.OSDirValidator{}

	return resolver.NewQueryResolver(sessions, store, zoxide, dirValidator).WithKept(resolverKept(cmd)), nil
}

func resolverSessions(cmd *cobra.Command) resolver.SessionLister {
	if openDeps != nil && openDeps.SessionLister != nil {
		return openDeps.SessionLister
	}
	return backendSessionNames{ctx: cmd.Context()}
}

func resolverKept(cmd *cobra.Command) resolver.KeptLookup {
	if openDeps != nil && openDeps.Kept != nil {
		return openDeps.Kept
	}
	return keptSessions{ctx: cmd.Context()}
}

// keptSessions judges a saved session kept by restore's own rule. A name
// zmx's Create refuses, its length aside, is never kept, so it costs nothing.
// Any other is looked up in sessions.json, read without its lock, and the
// sessions are listed only for a name the index holds, so a target no session
// was saved under costs one file read. A read that fails is logged here; the
// resolver then keeps nothing, as a listing that fails lists no session.
type keptSessions struct {
	ctx context.Context
}

func (k keptSessions) IsKept(name string) (bool, error) {
	if zmx.CheckName(name) != nil {
		return false, nil
	}
	b, err := sessionBackend()
	if err != nil {
		restoreLogger.Warn("list sessions failed", "session", name, "error", err)
		return false, err
	}
	dir, e, saved, err := savedEntry(name)
	if err != nil {
		restoreLogger.Warn("sessions.json unreadable", "session", name, "error", err)
		return false, err
	}
	if !saved {
		return false, nil
	}
	live, err := b.List(k.ctx)
	if err != nil {
		restoreLogger.Warn("list sessions failed", "session", name, "error", err)
		return false, err
	}
	kept := restore.Kept(k.ctx, []sessionindex.Entry{e}, live, restore.Keeping{Rule: claimRule(dir, b.ValidateName), Names: b})
	return len(kept) == 1, nil
}

// savedEntry is the state directory and the entry sessions.json there holds
// for name, and whether it holds one.
func savedEntry(name string) (dir string, e sessionindex.Entry, saved bool, err error) {
	if dir, err = state.Dir(); err != nil {
		return "", sessionindex.Entry{}, false, err
	}
	idx, err := sessionindex.Read(dir)
	if err != nil {
		return "", sessionindex.Entry{}, false, err
	}
	e, saved = idx.Lookup(name)
	return dir, e, saved, nil
}

// backendSessionNames lists the names of the backend's user sessions, the ones
// a target resolves against: resolution matches names, never labels.
type backendSessionNames struct {
	ctx context.Context
}

func (l backendSessionNames) ListSessionNames() ([]string, error) {
	b, err := sessionBackend()
	if err != nil {
		return nil, err
	}
	return userSessionNames(l.ctx, b)
}

func init() {
	openCmd.Flags().StringP("exec", "e", "", "command to execute in the new session")
	openCmd.Flags().StringP("filter", "f", "", "open the picker pre-filtered by <text> (skips resolution)")
	openCmd.Flags().StringP("session", "s", "", "attach the named session or session glob (session-domain; never mints); an exact name that is a kept saved session is brought back first")
	openCmd.Flags().StringP("path", "p", "", "mint a new session at the given directory (path-domain; dir must exist)")
	openCmd.Flags().StringP("alias", "a", "", "mint a new session at the given alias key or key glob (alias-domain)")
	openCmd.Flags().StringP("zoxide", "z", "", "mint a new session at zoxide's best match (zoxide-domain; explicit error if zoxide is not installed)")
	openCmd.Flags().String("ack", "", "internal: <batch>:<token> — a spawned window's receipt, written once its attach begins")
	_ = openCmd.Flags().MarkHidden("ack")

	// -p/--path and -z/--zoxide deliberately register no completer, so cobra emits
	// ShellCompDirectiveDefault and the shell provides file/default completion.
	openCmd.ValidArgsFunction = completeOpenPositional
	_ = openCmd.RegisterFlagCompletionFunc("session", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeSessionNames(toComplete)
	})
	_ = openCmd.RegisterFlagCompletionFunc("alias", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeAliasKeys(toComplete)
	})

	rootCmd.AddCommand(openCmd)
}
