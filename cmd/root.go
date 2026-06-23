// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// skipTmuxCheck contains command names that do not require tmux.
// If any command in the parent chain matches, the tmux check is skipped.
//
// Per the resurrection spec, the exempt set is:
//   - alias / clean / help / init / version: user-facing config or help
//   - hooks: hooks set/rm/list are pure config-file operations that need
//     only $TMUX_PANE (already guaranteed because they run inside a tmux
//     pane) and a single tmux display-message round-trip via
//     buildHooksTmuxClient() in cmd/hooks.go to resolve the structural
//     pane key; they do not need daemon orchestration, saver bootstrap,
//     version-upgrade machinery, Restore, EagerSignalHydrate,
//     marker/FIFO cleanup, or hookStore.CleanStale. hooks list needs
//     nothing tmux-related at all. Stale-entry auto-cleanup remains
//     attached to bootstrap-triggering commands (portal open / x /
//     attach).
//   - state: every `portal state ...` subcommand. User-facing children
//     (status, cleanup) inspect or tear down machinery the bootstrap sets
//     up — running bootstrap first would be circular. Internal children
//     (daemon, notify, signal-hydrate, hydrate, migrate-rename) are invoked
//     by tmux hooks or as the pane's initial process; re-running bootstrap
//     would recursively register hooks and could spawn nested daemons.
var skipTmuxCheck = map[string]bool{
	"alias":   true,
	"clean":   true,
	"help":    true,
	"hooks":   true,
	"init":    true,
	"state":   true,
	"version": true,
}

// bootstrapDeps holds injectable dependencies for PersistentPreRunE. When
// nil, real implementations are used.
var bootstrapDeps *BootstrapDeps

// BootstrapDeps allows injecting bootstrap dependencies for testing.
//
// Orchestrator is the test seam — implementations of bootstrap.Runner
// whose Run is invoked by PersistentPreRunE. Client populates the
// tmuxClientKey context value (helpers like tmuxClient(cmd) panic
// without it). RegisterHooks is the seam for Portal's global tmux hook
// registration; when nil (production default), tmux.RegisterPortalHooks
// is used.
type BootstrapDeps struct {
	// Orchestrator is the test seam for bootstrap. When nil, runBootstrap
	// short-circuits to a (false, nil, nil) result so tests indifferent
	// to bootstrap can leave it unset.
	Orchestrator bootstrap.Runner

	// Client is exposed in cmd.Context() under tmuxClientKey so downstream
	// commands (list, attach, kill, …) can look it up.
	Client *tmux.Client

	// RegisterHooks is invoked after the orchestrator returns to register
	// Portal's global tmux hook table on the live client. Production
	// default is tmux.RegisterPortalHooks.
	RegisterHooks func(*tmux.Client) error

	// ForceMemoise opts the test into the production sync.Once gate. By
	// default tests bypass the gate so subtests that swap mocks do not
	// have to reset shared package state between Execute() calls. Only
	// the dedicated memoisation test sets this to true.
	ForceMemoise bool
}

// bootstrapOnce, bootstrapStarted, bootstrapWarningsSlice and bootstrapErr
// memoise the orchestrator call so PersistentPreRunE invokes Run exactly
// once per process. Tests reset the gate via resetBootstrapOnce(t). The
// pattern mirrors versionCheckOnce in cmd/version_guard.go.
var (
	bootstrapOnce          sync.Once
	bootstrapStarted       bool
	bootstrapWarningsSlice []bootstrap.Warning
	bootstrapErr           error
)

// buildBootstrapDeps returns the runner, shared client, and hook
// registration function used by PersistentPreRunE. When bootstrapDeps is
// set (test mode), uses the injected Orchestrator verbatim — runBootstrap
// short-circuits a nil runner to (false, nil, nil) for tests indifferent
// to bootstrap. In production, builds a fully-wired
// *bootstrap.Orchestrator that runs the canonical eleven-step sequence
// (see cmd/bootstrap_production.go).
//
// In production the returned hook-registration callback is nil: hook
// registration is owned by step 2 of the orchestrator. Tests still
// inject a non-nil callback when they want to assert on
// PersistentPreRunE's post-Run hook plumbing without standing up a real
// orchestrator.
func buildBootstrapDeps() (bootstrap.Runner, *tmux.Client, func(*tmux.Client) error) {
	if bootstrapDeps != nil {
		return bootstrapDeps.Orchestrator, bootstrapDeps.Client, bootstrapDeps.RegisterHooks
	}
	orch, client := buildProductionOrchestrator()
	return orch, client, nil
}

// runBootstrap invokes the runner with per-process memoisation. In
// production (bootstrapDeps == nil) the sync.Once gate guarantees Run is
// called exactly once. In test mode the gate is bypassed by default so
// tests that rebuild bootstrapDeps between subtests do not need to reset
// shared state — set BootstrapDeps.ForceMemoise to opt back into the
// gate when verifying memoisation behaviour itself.
//
// The middle return is the slice of soft Warnings the orchestrator
// accumulated during Run. Callers feed it into bootstrapWarnings (the
// package-level sink) so PersistentPreRunE / openTUI can drain it later.
func runBootstrap(ctx context.Context, runner bootstrap.Runner) (bool, []bootstrap.Warning, error) {
	if bootstrapDeps != nil && !bootstrapDeps.ForceMemoise {
		if runner == nil {
			return false, nil, nil
		}
		return runner.Run(ctx)
	}
	bootstrapOnce.Do(func() {
		if runner == nil {
			return
		}
		bootstrapStarted, bootstrapWarningsSlice, bootstrapErr = runner.Run(ctx)
	})
	return bootstrapStarted, bootstrapWarningsSlice, bootstrapErr
}

var rootCmd = &cobra.Command{
	Use:   "portal",
	Short: "An interactive session picker for tmux",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		for c := cmd; c != nil; c = c.Parent() {
			if skipTmuxCheck[c.Name()] {
				return nil
			}
		}
		if err := tmux.CheckTmuxAvailable(); err != nil {
			return bootstrap.NewFatal(err.Error(), err)
		}
		if err := runVersionCheck(); err != nil {
			return bootstrap.NewFatal(err.Error(), err)
		}

		runner, client, registerHooks := buildBootstrapDeps()
		started, warnings, err := runBootstrap(cmd.Context(), runner)
		if err != nil {
			return err
		}

		// Feed every soft warning into the package-level sink so the TUI
		// path can drain post-loading-page dismissal (task 6-10). The CLI
		// path (every command except `portal open` with zero positional
		// args) drains here so warnings precede the command's own
		// stdout/stderr — see spec, Observability → Proactive Health
		// Signals → TUI interaction.
		for _, w := range warnings {
			bootstrapWarnings.Add(w)
		}
		if !isTUIPath(cmd, args) {
			bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
		}

		ctx := context.WithValue(cmd.Context(), serverStartedKey, started)
		if client != nil {
			ctx = context.WithValue(ctx, tmuxClientKey, client)
		}
		cmd.SetContext(ctx)

		// §10.2 startup-flip routing seam (foundation gate, task 5-1).
		//
		// shouldRunConcurrentBootstrap scopes the eventual concurrent
		// cold-boot bootstrap to the COLD + TUI path only; every other path
		// (warm, and cold CLI/direct-path) keeps today's synchronous
		// behaviour byte-for-byte. The context above carries the
		// serverStarted flag the decider reads, so this is the first point
		// where the decision is available.
		//
		// For now this is a PURE-ROUTING stub: bootstrap has ALREADY run
		// synchronously above (runBootstrap), so the concurrent route resolves
		// to today's path and there is no behaviour change on any path. Tasks
		// 5-2..5-7 extend the concurrent route — moving the orchestrator into a
		// goroutine launched from the loading-page TUI, with progress /
		// warnings / fatal delivery over a channel — for the cold + TUI path
		// only. Until then runConcurrentBootstrap is a no-op that returns the
		// already-synchronous outcome.
		if shouldRunConcurrentBootstrap(cmd, args) {
			// task 5-2..5-7 extend this — concurrent cold-boot route.
			runConcurrentBootstrap()
		}

		// In production the orchestrator owns hook registration (step 2)
		// and buildBootstrapDeps returns a nil registerHooks — the guard
		// below is a no-op. The hook stays in place purely as a test
		// seam: BootstrapDeps.RegisterHooks lets tests inject a recorder
		// to assert on the post-Run plumbing without standing up a real
		// orchestrator.
		if registerHooks != nil && client != nil {
			if err := registerHooks(client); err != nil {
				return err
			}
		}
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// isTUIPath reports whether the invoked command will launch the Bubble
// Tea TUI (and therefore must NOT have warnings emitted to stderr from
// PersistentPreRunE — they would corrupt the alt-screen rendering). The
// only TUI-launching path is `portal open` with zero positional args; an
// `open <path>` invocation resolves directly via openPath without
// entering the TUI. See cmd/open.go's RunE for the gating logic that
// mirrors this check.
func isTUIPath(cmd *cobra.Command, args []string) bool {
	return cmd.Name() == "open" && len(args) == 0
}

// shouldRunConcurrentBootstrap is the cold-vs-warm routing decider for the
// §10.2 startup flip: it reports whether this invocation is the one path that
// the eventual concurrent cold-boot bootstrap is scoped to — COLD + TUI.
//
//   - Cold = serverWasStarted(cmd) is true, i.e. EnsureServer actually had to
//     start the tmux server this launch (§10.1). The flag is already threaded
//     through serverStartedKey from the orchestrator's return, so the gate
//     reads it directly and adds ZERO new tmux round-trips — the warm path's
//     fast synchronous behaviour is untouched. (ServerRunning()/has-server is
//     deliberately NOT used: a fresh probe would cost a tmux round-trip on the
//     warm path, and the started-vs-already-running signal is already known.)
//   - TUI = isTUIPath(cmd, args) is true, i.e. `portal open` with zero
//     positional args. `open <path>` resolves directly via openPath and is
//     therefore NOT the TUI path.
//
// Every other path — warm (any command), and cold CLI/direct-path — keeps
// today's exact synchronous bootstrap. This decider is the foundation gate
// that tasks 5-2..5-7 build the concurrent route on; see the wiring point in
// PersistentPreRunE.
func shouldRunConcurrentBootstrap(cmd *cobra.Command, args []string) bool {
	return serverWasStarted(cmd) && isTUIPath(cmd, args)
}

// runConcurrentBootstrap is the STUB body of the cold + TUI concurrent route
// (§10.2). It is intentionally a no-op: as of task 5-1 the orchestrator has
// already run synchronously in PersistentPreRunE, so the concurrent route
// resolves to today's path with no behaviour change. Tasks 5-2..5-7 replace
// this body — launching Bubble Tea immediately on the loading page, running
// the orchestrator in a goroutine, and streaming progress / warnings / fatal
// over a channel. Keeping the seam as a named call (rather than an empty
// branch) gives those tasks a single concrete extension point.
func runConcurrentBootstrap() {
	// task 5-2..5-7 replace this body with the concurrent cold-boot route.
}

// fatalErrorStderr is the sink for *bootstrap.FatalError user messages.
// Test seam: tests redirect to a buffer to assert the single-line output
// without invoking os.Stderr or building the binary.
var fatalErrorStderr io.Writer = os.Stderr

// Execute runs the root command. When PersistentPreRunE (or any subcommand
// in the chain) returns a *bootstrap.FatalError, Execute writes the
// fatal's UserMessage as a single line to fatalErrorStderr before
// returning the error. Callers (main.go) translate the returned error
// into the process exit code; Execute itself does not call os.Exit so
// tests can drive the FatalError path without subprocess fanout.
func Execute() error {
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}
	var fatal *bootstrap.FatalError
	if errors.As(err, &fatal) {
		_, _ = fmt.Fprintln(fatalErrorStderr, fatal.UserMessage)
	}
	return err
}
