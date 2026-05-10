package bootstrap_test

// Shared bootstrap.Orchestrator builder for tests in this package.
//
// Eleven test sites across cmd/bootstrap (and one sibling in cmd/) used to
// rebuild the same multi-step Orchestrator literal — adding a new step
// interface (e.g. StaleMarkers, EagerSignaler) meant touching every literal.
// This helper centralises the wiring so a future step addition is a
// one-file change in orchestratorOpts + buildIntegrationOrchestrator.
//
// Defaults policy: every step that the spec permits to degrade-and-continue
// defaults to its NoOp form (Hooks, Saver, Restore, EagerSignaler,
// StaleMarkers, Sweeper, Clean). RestoringMarker is always real because step
// 3 / step 7 are fatal-on-failure and the marker contract is exercised in
// every Run path.
//
// The sibling builder in cmd/reattach_integration_test.go (package cmd)
// cannot delegate to this helper because Go test-file symbols are not
// visible across packages — that file keeps its own thin builder. Adding a
// new step interface therefore requires editing two files: this one, and
// reattach_integration_test.go's buildReattachOrchestrator.

import (
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// orchestratorOpts captures every step that varies across the eleven
// integration sites. Unset (nil) fields default to their NoOp form per the
// "spec permits to degrade-and-continue" policy. Logger is optional — when
// nil, the Orchestrator substitutes its internal noopLogger at Run time.
//
// EagerSignaler default has one branch: when the caller has wired a real
// Restore adapter, leaving EagerSignaler nil yields a real
// *bootstrap.EagerSignalCore (mirroring buildProductionOrchestrator) so
// the eager-signal step actually fires in the integration scenario the
// caller is exercising. If the caller did not provide a Restore adapter
// (Restore stays NoOp → no skeleton markers will be set), the eager step
// would be vacuous and the builder retains the NoOp default. Tests that
// drive signal-hydrate via their own manual harness (notably the reboot
// round-trips) explicitly opt out by setting EagerSignaler to
// bootstrap.NoOpEagerHydrateSignaler{}.
type orchestratorOpts struct {
	Hooks         bootstrap.HookRegistrar
	Saver         bootstrap.SaverBootstrapper
	Restore       bootstrap.Restorer
	EagerSignaler bootstrap.EagerHydrateSignaler
	StaleMarkers  bootstrap.MarkerCleaner
	Sweeper       bootstrap.FIFOSweeper
	Clean         bootstrap.StaleCleaner
	Logger        bootstrap.Logger
}

// buildIntegrationOrchestrator returns a *bootstrap.Orchestrator wired with
// the supplied client as Server, a real bootstrapadapter.RestoringMarker, and
// every other field defaulted to its NoOp form unless the caller supplied a
// real adapter via opts.
func buildIntegrationOrchestrator(t *testing.T, client *tmux.Client, opts orchestratorOpts) *bootstrap.Orchestrator {
	t.Helper()
	if opts.Hooks == nil {
		opts.Hooks = bootstrap.NoOpHooks{}
	}
	if opts.Saver == nil {
		opts.Saver = bootstrap.NoOpSaver{}
	}
	// Track whether the caller wired a real Restore adapter BEFORE the
	// NoOp defaulting below — the EagerSignaler default-selection branch
	// keys off the original caller intent, not the post-defaulting field
	// value. Without this latch, a NoOp Restore (from defaulting) would
	// be indistinguishable from a real RestoreAdapter and the eager step
	// would always default to real, losing the "vacuous when no
	// skeletons are armed" guard.
	restoreWired := opts.Restore != nil
	if opts.Restore == nil {
		opts.Restore = bootstrap.NoOpRestorer{}
	}
	if opts.EagerSignaler == nil {
		if restoreWired {
			// Mirror buildProductionOrchestrator's wiring shape: the
			// production *tmux.Client satisfies state.ServerOptionLister
			// directly, stateDir is resolved via state.Dir() (which the
			// integration sites pre-set via newIntegrationStateDir →
			// PORTAL_STATE_DIR), and state.DefaultFIFOSignaler{} is the
			// production no-seam wrapper around state.SendHydrateSignal.
			// A stateDir resolution failure is tolerated: state.Dir
			// returns the resolved path even when the directory does not
			// exist on disk; EagerSignalCore only consults stateDir to
			// derive per-FIFO paths inside its loop.
			stateDir, _ := state.Dir()
			opts.EagerSignaler = &bootstrap.EagerSignalCore{
				Markers:  client,
				StateDir: stateDir,
				Signaler: state.DefaultFIFOSignaler{},
				Logger:   opts.Logger,
			}
		} else {
			opts.EagerSignaler = bootstrap.NoOpEagerHydrateSignaler{}
		}
	}
	if opts.StaleMarkers == nil {
		opts.StaleMarkers = bootstrap.NoOpMarkerCleaner{}
	}
	if opts.Sweeper == nil {
		opts.Sweeper = bootstrap.NoOpFIFOSweeper{}
	}
	if opts.Clean == nil {
		opts.Clean = bootstrap.NoOpStaleCleaner{}
	}
	return &bootstrap.Orchestrator{
		Server:        client,
		Hooks:         opts.Hooks,
		Restoring:     &bootstrapadapter.RestoringMarker{Client: client},
		Saver:         opts.Saver,
		Restore:       opts.Restore,
		EagerSignaler: opts.EagerSignaler,
		StaleMarkers:  opts.StaleMarkers,
		Sweeper:       opts.Sweeper,
		Clean:         opts.Clean,
		Logger:        opts.Logger,
	}
}

// openTestLogger opens a state.Logger writing to <stateDir>/portal.log and
// registers t.Cleanup to close it. Tests that wire a real Logger or any
// adapter that needs one (FIFOSweeper, HookRegistrar) share this helper to
// avoid duplicating the OpenLogger + Cleanup pattern.
func openTestLogger(t *testing.T, stateDir string) *state.Logger {
	t.Helper()
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger
}

// newIntegrationStateDir builds an isolated portal state directory rooted at
// t.TempDir(), wires it via PORTAL_STATE_DIR (auto-restored by t.Setenv on
// test teardown), and runs state.EnsureDir so callers can immediately write
// sessions.json / scrollback / FIFOs into the returned path.
//
// Paired with openTestLogger (which writes to <stateDir>/portal.log) the two
// helpers replace the nine-site stateDir + EnsureDir + OpenLogger preamble
// previously copy-pasted across the cmd/bootstrap integration tests. They
// remain split because not every site that needs the stateDir half also
// needs a real logger (e.g. the orchestrator end-to-end smoke test wires no
// logger at all and lets the orchestrator substitute its noopLogger).
func newIntegrationStateDir(t *testing.T) string {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	return stateDir
}
