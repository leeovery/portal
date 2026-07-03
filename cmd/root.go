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
	"github.com/leeovery/portal/internal/state"
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
//     version-upgrade machinery, Restore, EagerSignalHydrate, or
//     marker/FIFO cleanup. hooks list needs nothing tmux-related at all.
//     Stale hook-entry cleanup is no longer a bootstrap step; `portal
//     clean` is the manual home.
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
// *bootstrap.Orchestrator that runs the canonical ten-step sequence
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

		// TODO(2-3): replace with the single upstream latch verdict computed once
		// in PersistentPreRunE. Transitional local computation so the re-keyed
		// shouldRunConcurrentBootstrap signature compiles; guarded on a non-nil
		// client because BootstrappedLatchSatisfied reads through the client.
		latchSatisfied := client != nil && state.BootstrappedLatchSatisfied(client, version)

		// §10.2 startup flip — concurrent full-bootstrap route on the TUI path.
		//
		// shouldRunConcurrentBootstrap scopes the concurrent full bootstrap to the
		// TUI path (`isTUIPath` decides TUI) when the latch is NOT satisfied — i.e.
		// whenever a FULL bootstrap must run behind the loading screen. The trigger
		// is latch-not-satisfied (the verdict computed upstream), NOT a server-down
		// probe. On that route the orchestrator is NOT run synchronously here — it
		// is DEFERRED to openTUI, which runs it in a goroutine while Bubble Tea
		// renders the loading page from frame one, streaming progress over a
		// channel (cmd/bootstrap_progress.go). Every other path — the latch-
		// satisfied route and cold CLI/direct-path — keeps today's exact
		// synchronous bootstrap below, byte-for-byte: the serverStartedKey context
		// delivery and the sync.Once memo are untouched off the deferred route.
		if shouldRunConcurrentBootstrap(cmd, args, client, latchSatisfied) {
			ctx := cmd.Context()
			if client != nil {
				ctx = context.WithValue(ctx, tmuxClientKey, client)
			}
			ctx = context.WithValue(ctx, deferredBootstrapKey, &deferredBootstrap{runner: runner})
			cmd.SetContext(ctx)
			// Bootstrap is deferred; serverStartedKey is NOT set here (openTUI
			// reads serverStarted from the progress pipe's terminal event). The
			// hook-registration test seam below is also skipped — on the
			// concurrent route the orchestrator (running in openTUI's goroutine)
			// owns step 2; PersistentPreRunE has nothing to register synchronously.
			return nil
		}

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

// shouldRunConcurrentBootstrap is the routing decider for the §10.2 startup
// flip: it reports whether this invocation takes the concurrent + loading-screen
// route rather than the synchronous one.
//
// The concurrent/loading route fires whenever a FULL bootstrap runs on the TUI
// path — keyed off latch-not-satisfied (the verdict computed once upstream in
// PersistentPreRunE), NOT server-down. A full bootstrap on the TUI path should
// always show the loading screen, so "loading screen" now means exactly "a full
// bootstrap is in progress" (whether the server was cold or warm-unlatched). The
// retired ServerRunning() has-server probe is gone; this decider issues zero
// tmux round-trips.
//
//   - TUI = isTUIPath(cmd, args) is true, i.e. `portal open` with zero
//     positional args. `open <path>` resolves directly via openPath and is
//     therefore NOT the TUI path.
//   - nil client (skipTmuxCheck commands never reach here, but be defensive)
//     classifies synchronous.
//
// latchSatisfied is passed in rather than recomputed here: the abridged branch
// returns upstream on the satisfied path (see Task 2-3), so by the time this
// decider is reached !latchSatisfied is effectively always true — but it is
// threaded explicitly to preserve the single-read invariant and to keep the
// contract self-describing.
func shouldRunConcurrentBootstrap(cmd *cobra.Command, args []string, client *tmux.Client, latchSatisfied bool) bool {
	if !isTUIPath(cmd, args) || client == nil {
		return false
	}
	return !latchSatisfied
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
