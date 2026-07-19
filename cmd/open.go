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
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// resolveLogger binds the "resolve" log component once for the whole open
// command (a spec-governed amendment to the closed log taxonomy — see the spec
// § Wrong-guess feedback). openCmd.RunE emits exactly one INFO decision line per
// bare positional resolved through the guessing chain, so a confusing guess is
// reconstructable from portal.log. internal/resolver stays a pure, log-free
// library: the binding and emission live only here.
var resolveLogger = log.For("resolve")

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

// openSessionFunc is the function used to attach this terminal to an existing
// session (the session-domain outcome). Tests override it via t.Cleanup-restored
// assignment to capture the target without building a real connector.
var openSessionFunc = openSession

// openDeps holds injectable dependencies for the open command.
// When nil, real implementations are used.
var openDeps *OpenDeps

// OpenDeps allows injecting dependencies for testing.
type OpenDeps struct {
	SessionLister resolver.SessionLister
	AliasLookup   resolver.AliasLookup
	Zoxide        resolver.ZoxideQuerier
	DirValidator  resolver.DirValidator
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

// openSession connects this terminal to an existing named session — the
// session-domain outcome of resolution (Axiom 2: a session-domain hit attaches).
// buildSessionConnector selects switch-client (inside tmux) or exec attach
// (outside tmux); the "=" exact-match target is applied by the connector.
func openSession(cmd *cobra.Command, name string) error {
	return buildSessionConnector(tmuxClient(cmd)).Connect(name)
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

		// -f/--filter is the sole non-composing flag (spec § -f/--filter): not a
		// target, but a "skip resolution, open the picker pre-filtered" redirect.
		// Handled BEFORE resolution — it never routes through the query resolver.
		// It is mutually exclusive with a positional target (Phase 1 scope: pins
		// don't exist yet) and rejects an empty value, mirroring the empty -e guard.
		if cmd.Flags().Changed("filter") {
			filterVal, _ := cmd.Flags().GetString("filter")
			if destination != "" {
				return NewUsageError("cannot use -f/--filter with a target")
			}
			if filterVal == "" {
				return NewUsageError("-f/--filter value must not be empty")
			}
			return openTUIFunc(cmd, filterVal, command, serverWasStarted(cmd))
		}

		// -s/--session pin (spec § Domain-pinning flags): resolve the value in the
		// session domain only (exact name / glob) and dispatch the hit through the
		// shared outcome switch. A miss hard-fails — the pin never mints and never
		// opens the picker (spec § Pinned-domain contract) — and emits no resolve
		// line (pins are deterministic, not guesses). Placed BEFORE the no-target
		// early-return so `open -s <name>` with an empty positional resolves the pin
		// rather than launching the picker.
		if cmd.Flags().Changed("session") {
			sessionVal, _ := cmd.Flags().GetString("session")
			qr, err := buildQueryResolver(cmd)
			if err != nil {
				return err
			}
			result, err := qr.ResolveSessionPin(sessionVal)
			if err != nil {
				return err
			}
			return openResolved(cmd, result, command)
		}

		// -p/--path pin (spec § Domain-pinning flags): resolve the value in the path
		// domain only via ResolvePathPin (which reuses ResolvePath for tilde/relative
		// expansion + existence + is-directory validation) and dispatch the resulting
		// *PathResult through the shared outcome switch to mint. Because ResolvePath
		// stats the LITERAL path, a directory whose name contains glob metacharacters
		// (~/tmp/foo[1]) is reachable here — bypassing the glob pre-check that makes
		// it unreachable as a bare positional (spec § Glob targets). A non-existent
		// dir / non-directory file hard-fails; the pin never mints-to-picker (spec §
		// Pinned-domain contract) and emits no resolve line (pins are deterministic,
		// not guesses). Placed BEFORE the no-target early-return so `open -p <dir>`
		// with an empty positional resolves the pin rather than launching the picker.
		if cmd.Flags().Changed("path") {
			pathVal, _ := cmd.Flags().GetString("path")
			qr, err := buildQueryResolver(cmd)
			if err != nil {
				return err
			}
			result, err := qr.ResolvePathPin(pathVal)
			if err != nil {
				return err
			}
			return openResolved(cmd, result, command)
		}

		// -a/--alias pin (spec § Domain-pinning flags): resolve the value in the
		// alias domain only via ResolveAliasPin, which looks the key up directly in
		// the alias store — bypassing the session→path→alias precedence — so it is
		// the ONLY way to reach an alias key shadowed by a same-named session. A glob
		// value expands against the finite alias-key namespace (spec § Glob targets).
		// A hit mints (Axiom 2) and a *PathResult routes through the shared outcome
		// switch; an unknown key (or a glob matching zero keys) hard-fails and a gone
		// dir hard-fails with "Directory not found" — the pin never mints-to-picker
		// (spec § Pinned-domain contract) and emits no resolve line (pins are
		// deterministic, not guesses). Placed BEFORE the no-target early-return so
		// `open -a <key>` with an empty positional resolves the pin rather than
		// launching the picker.
		if cmd.Flags().Changed("alias") {
			aliasVal, _ := cmd.Flags().GetString("alias")
			qr, err := buildQueryResolver(cmd)
			if err != nil {
				return err
			}
			result, err := qr.ResolveAliasPin(aliasVal)
			if err != nil {
				return err
			}
			return openResolved(cmd, result, command)
		}

		// -z/--zoxide pin (spec § Domain-pinning flags): resolve the value in the
		// zoxide domain only via ResolveZoxidePin, which queries zoxide and makes its
		// outcome EXPLICIT — unlike the bare chain, which swallows any zoxide error and
		// silently falls through to the miss tail. A hit mints (Axiom 2) and the
		// *PathResult routes through the shared outcome switch; zoxide-not-installed
		// surfaces ErrZoxideNotInstalled verbatim (a script sees WHY), a no-match hard-
		// fails, and a gone best-match dir hard-fails with "Directory not found" — the
		// pin never mints-to-picker (spec § Pinned-domain contract) and emits no resolve
		// line (pins are deterministic, not guesses). Placed BEFORE the no-target early-
		// return so `open -z <query>` with an empty positional resolves the pin rather
		// than launching the picker.
		if cmd.Flags().Changed("zoxide") {
			zoxideVal, _ := cmd.Flags().GetString("zoxide")
			qr, err := buildQueryResolver(cmd)
			if err != nil {
				return err
			}
			result, err := qr.ResolveZoxidePin(zoxideVal)
			if err != nil {
				return err
			}
			return openResolved(cmd, result, command)
		}

		if destination == "" {
			return openTUIFunc(cmd, "", command, serverWasStarted(cmd))
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

		// Resolution-decision receipt (spec § Wrong-guess feedback — tmux is the
		// receipt): emit one durable INFO line per bare positional resolved through
		// the guessing chain (session → path → alias → zoxide), so a confusing guess
		// is reconstructable from portal.log. Gated on the glob predicate — glob (and
		// pinned) targets are deterministic, not guesses, so they emit no line.
		// Emitted on a miss too (domain=miss, empty resolved_path), IN ADDITION to the
		// separate stderr hard-fail below. A mid-chain hard error (DirNotFoundError)
		// returned above never reaches here: classification did not complete, so no
		// decision line fires.
		if !resolver.HasGlobMeta(query) {
			domain, resolvedPath := resolveDecision(result)
			resolveLogger.Info("resolved", "target", query, "domain", domain, "resolved_path", resolvedPath)
		}

		if miss, ok := result.(*resolver.MissResult); ok {
			// Total miss: hard-fail with the escape-hatch message (spec § Miss
			// handling). Handled inline in the bare-positional path — pins never
			// yield a MissResult. A plain (non-usage) error → exit code 1 via
			// main.classify; the TUI picker is never launched on a miss. The em-dash
			// is U+2014.
			return fmt.Errorf("nothing resolved for '%s' — try -f %s", miss.Target, miss.Target)
		}
		return openResolved(cmd, result, command)
	},
}

// openResolved dispatches a resolved query result to its outcome: a session-
// domain hit attaches (openSessionFunc); a directory-domain hit mints
// (openPathFunc), threading the mint-scoped command. It is the single shared
// dispatch point for the -s/--session pin, the mint pins (later Phase 2 tasks),
// and the bare-positional path, so every entry point routes a resolved result
// through the identical outcome switch. A MissResult is deliberately NOT handled
// here — the bare path renders its own escape-hatch message inline, and pins
// never yield a miss — so any unexpected result type is a defensive error.
func openResolved(cmd *cobra.Command, result resolver.QueryResult, command []string) error {
	switch r := result.(type) {
	case *resolver.SessionResult:
		return openSessionFunc(cmd, r.Name)
	case *resolver.PathResult:
		return openPathFunc(cmd, r.Path, command)
	default:
		return fmt.Errorf("unexpected resolution result: %T", result)
	}
}

// resolveDecision derives the (domain, resolved_path) attrs for the resolve
// decision log line from a completed classification result. resolved_path is
// overloaded per the spec: the resolved directory for a path/alias/zoxide hit,
// the session name for a session hit, and empty for a miss. It reads the domain
// off the already-obtained result (r.Domain / the "miss" literal) — it does not
// re-run the classification.
func resolveDecision(result resolver.QueryResult) (domain, resolvedPath string) {
	switch r := result.(type) {
	case *resolver.SessionResult:
		return r.Domain, r.Name
	case *resolver.PathResult:
		return r.Domain, r.Path
	case *resolver.MissResult:
		return "miss", ""
	default:
		return "", ""
	}
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
	// detector + resolve are the §6 async host-terminal detection seams. Built
	// once at TUI construction (detector over the shared *tmux.Client; resolve from
	// the config-aware buildResolver, terminals.json loaded once) and threaded into
	// tui.Deps. This is the SINGLE injection site — the picker burst reuses the
	// model's cached resolution and never re-injects.
	detector tui.TerminalDetector
	resolve  func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
	// §6-3 N≥2 picker-burst seams. Built once here (defaults mirroring the spawn
	// CLI's SpawnDeps: client.HasSession / a shared server-option ack channel /
	// os.Executable / os.Getenv) and threaded into tui.Deps. The burst REUSES the
	// resolve seam above (the cached resolution + a re-resolve for the adapter).
	sessionExists func(string) bool
	ackChannel    spawn.AckChannelFull
	spawnExe      spawn.ExecutableResolver
	spawnGetenv   func(string) string
	// spawnLogger is the §6-10 spawn-component logger the picker burst's completion
	// chokepoint emits its batch summary + per-window detail through (log.For("spawn")
	// in production; the parallel to cmd/spawn.go's package-level spawnLogger).
	spawnLogger    *slog.Logger
	cwd            string
	insideTmux     bool
	currentSession string
	serverStarted  bool
	// progressReceiver is the §10.2 concurrent cold-boot route's channel-receive
	// tea.Cmd. Set only on the cold + TUI path (where bootstrap was deferred to a
	// goroutine); nil on every synchronous path, leaving the model's today
	// behaviour intact.
	progressReceiver tea.Cmd
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
		CWD:              cfg.cwd,
		InitialMode:      cfg.initialMode,
		Appearance:       cfg.appearance,
		InitialFilter:    initialFilter,
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
	})
}

// processTUIResult handles the result of a TUI run.
//
// §10.5 fatal cold-boot: if the model carries a fatal (a fatal bootstrap step
// aborted the boot on the concurrent cold/TUI route, and q/Esc quit the error
// frame), return that fatal — the underlying *bootstrap.FatalError — BEFORE any
// connect. Execute writes its single UserMessage line and main.classify maps it
// to code 1 with no double-print, exactly as the synchronous warm/CLI path does;
// returning the SAME *bootstrap.FatalError instance keeps the exit byte-for-byte
// identical. A fatal model has no selection, so this also guarantees no connect.
//
// Otherwise: if the user selected a session, connect via the given connector; if
// the user quit without selecting, return nil (unchanged).
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

// openTUI launches the interactive session picker with an optional initial filter.
func openTUI(cmd *cobra.Command, initialFilter string, command []string, serverStarted bool) error {
	client := tmuxClient(cmd)
	gitResolver := &resolverAdapter{}
	gen := session.NewNanoIDGenerator()

	// §10.2 concurrent full-bootstrap route: PersistentPreRunE deferred the
	// orchestrator on the (latch-not-satisfied) TUI path. Build the progress
	// pipe, launch the orchestrator in a goroutine, and stream live per-step
	// progress to the loading page over the channel. The model renders the
	// loading page from frame one — serverStarted is forced true just below
	// because a full bootstrap is in progress on this route, NOT because the
	// server was necessarily cold: a warm-unlatched server (hand-started tmux hit
	// by `x`) reaches this route too, so "the server was not running" is not a
	// safe assumption. On every synchronous path pipe is nil and openTUI keeps
	// today's behaviour (serverStarted carried via the serverStartedKey context
	// that the caller already read).
	var pipe *bootstrapProgressPipe
	if deferred := deferredBootstrapFromContext(cmd); deferred != nil {
		pipe = newBootstrapProgressPipe()
		pipe.start(cmd.Context(), deferred.runner)
		// Full bootstrap in progress: the loading page must show, so force
		// serverStarted regardless of the caller's (false, deferred) flag.
		// serverStarted's sole effect is parking the model on the loading page
		// (WithServerStarted(true) -> activePage = PageLoading); on the deferred
		// route the server may or may not have pre-existed (warm-unlatched), but a
		// full bootstrap is running either way, which is exactly when the loading
		// page should show.
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
	// which is the default detection behaviour anyway. The value is honoured
	// downstream: a `light`/`dark` pin skips detection + the first-paint wait via
	// Build → armAppearanceDetection (§2.6).
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

	// Build the shared production spawn seams ONCE from the resolved client. This
	// is the same bundle the spawn CLI's buildSpawnDeps reads, so the picker's
	// §6/§6-3 detection + burst seams cannot silently diverge from the CLI's.
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
		// §6 async host-terminal detection seams, from the shared builder: the
		// detector over the shared *tmux.Client, and the config-aware resolver's
		// Resolve (buildResolver loads terminals.json once, degrading to an empty
		// native-only config on a configFilePath error). Detection runs off the
		// Update path as a tea.Cmd; the resolution is cached on the model and reused
		// by the later picker burst.
		detector: spawnSeams.Detector,
		resolve:  spawnSeams.Resolve,
		// §6-3 N≥2 picker-burst seams — the same shared bundle the spawn CLI's
		// SpawnDeps defaults from: the pre-flight has-session probe folds a probe
		// fault to gone (conservative), the shared server-option ack channel
		// confirms/cleans spawned windows, and the exe/PATH seams compose each
		// spawned attach argv.
		sessionExists: spawnSeams.Exists,
		ackChannel:    spawnSeams.Ack,
		spawnExe:      spawnSeams.Exe,
		spawnGetenv:   spawnSeams.Getenv,
		// §6-10: the picker burst's spawn-component logger — the TUI parallel to
		// cmd/spawn.go's package-level spawnLogger = log.For("spawn").
		spawnLogger: spawnSeams.Logger,
		// NO_COLOR carve-out (§2.5): read the env ONCE here (cmd layer) so
		// internal/tui stays env-free. The single colourless flag flows through
		// tui.Deps.NoColor and is inherited by every canvas-dependent surface.
		noColor: noColorEnabled(),
	}
	// §10.2 concurrent cold-boot route: wire the channel-receive tea.Cmd so the
	// loading-page model streams live per-step progress and the channel owns the
	// terminal BootstrapCompleteMsg (Init does NOT synthesize it on this route).
	if pipe != nil {
		cfg.progressReceiver = pipe.receiver()
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
	// Bootstrap ordering differs by route (§10.2):
	//   - Synchronous (warm/CLI): PersistentPreRunE already ran the orchestrator,
	//     so the model's Init emits BootstrapCompleteMsg from its first event-loop
	//     tick (carrying staged warnings), paired with the 1.2s LoadingMinElapsedMsg
	//     tick. Dismissal gates on both.
	//   - Concurrent (cold/TUI): the orchestrator runs in the pipe's goroutine; the
	//     channel streams BootstrapProgressMsg per step and the terminal
	//     BootstrapCompleteMsg, so Init wires the receiver instead of synthesizing
	//     the terminal event. Dismissal still gates on the 1.2s tick + the terminal
	//     channel event.
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
// The session lister is the user-visible (leading-underscore-filtered) session
// set: openDeps.SessionLister when injected, otherwise the shared *tmux.Client
// (which satisfies resolver.SessionLister via ListSessionNames).
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
	rootCmd.AddCommand(openCmd)
}
