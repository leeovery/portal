package bootstrap_test

// Shared bootstrap.Orchestrator builder for tests in this package.
//
// Eleven test sites across cmd/bootstrap (and one sibling in cmd/) used to
// rebuild the same eight-step Orchestrator literal — adding a new step
// interface (e.g. StaleMarkers) meant touching every literal. This helper
// centralises the wiring so a future step addition is a one-file change in
// orchestratorOpts + buildIntegrationOrchestrator.
//
// Defaults policy: every step that the spec permits to degrade-and-continue
// defaults to its NoOp form (Hooks, Saver, Restore, StaleMarkers, Sweeper,
// Clean). RestoringMarker is always real because step 3 / step 6 are
// fatal-on-failure and the marker contract is exercised in every Run path.
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
type orchestratorOpts struct {
	Hooks        bootstrap.HookRegistrar
	Saver        bootstrap.SaverBootstrapper
	Restore      bootstrap.Restorer
	StaleMarkers bootstrap.MarkerCleaner
	Sweeper      bootstrap.FIFOSweeper
	Clean        bootstrap.StaleCleaner
	Logger       bootstrap.Logger
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
	if opts.Restore == nil {
		opts.Restore = bootstrap.NoOpRestorer{}
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
		Server:       client,
		Hooks:        opts.Hooks,
		Restoring:    &bootstrapadapter.RestoringMarker{Client: client},
		Saver:        opts.Saver,
		Restore:      opts.Restore,
		StaleMarkers: opts.StaleMarkers,
		Sweeper:      opts.Sweeper,
		Clean:        opts.Clean,
		Logger:       opts.Logger,
	}
}

// openTestLogger opens a state.Logger writing to <stateDir>/portal.log and
// registers t.Cleanup to close it. Tests that wire a real Logger or any
// adapter that needs one (StaleMarkerCleaner, FIFOSweeper, HookRegistrar)
// share this helper to avoid duplicating the OpenLogger + Cleanup pattern.
func openTestLogger(t *testing.T, stateDir string) *state.Logger {
	t.Helper()
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger
}
