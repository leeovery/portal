package bootstrap_test

// Shared bootstrap.Orchestrator builder for tests in this package.
//
// Eleven test sites across cmd/bootstrap (and one sibling in cmd/) used to
// rebuild the same multi-step Orchestrator literal — adding a new step
// interface (e.g. StaleMarkers, EagerSignaler) meant touching every literal.
// The defaulting policy is now centralised in bootstrap.NewWithDefaults
// (cmd/bootstrap/defaults.go) — a non-test helper both this file and the
// cmd-package sibling buildReattachOrchestrator delegate to. A future
// eleventh step requires only a new bootstrap.With* option constructor
// plus one default wire-up at the helper site; no edits in either test
// file (they pass through any unhandled options via the same With* shape).
//
// Defaults policy: every step that the spec permits to degrade-and-continue
// defaults to its NoOp form (Hooks, OrphanSweeper, Saver, Restore,
// EagerSignaler, StaleMarkers, Sweeper, Clean). RestoringMarker is always
// real because step 3 / step 8 are fatal-on-failure and the marker contract
// is exercised in every Run path.

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// orchestratorOpts captures every step that varies across the eleven
// integration sites. Unset (nil) fields default per bootstrap.NewWithDefaults
// — see cmd/bootstrap/defaults.go for the full defaulting policy (NoOp for
// degrade-and-continue steps; EagerSignaler resolves to a real
// *bootstrap.EagerSignalCore when Restore is wired real).
type orchestratorOpts struct {
	Hooks         bootstrap.HookRegistrar
	OrphanSweeper bootstrap.OrphanSweeper
	Saver         bootstrap.SaverBootstrapper
	Restore       bootstrap.Restorer
	EagerSignaler bootstrap.EagerHydrateSignaler
	StaleMarkers  bootstrap.MarkerCleaner
	Sweeper       bootstrap.FIFOSweeper
	Clean         bootstrap.StaleCleaner
	Logger        *slog.Logger
}

// buildIntegrationOrchestrator returns a *bootstrap.Orchestrator wired with
// the supplied client as Server, a real bootstrapadapter.RestoringMarker, and
// every other field defaulted to its NoOp form unless the caller supplied a
// real adapter via opts. Delegates to bootstrap.NewWithDefaults so the
// defaulting policy lives in one place — see cmd/bootstrap/defaults.go.
//
// stateDir is resolved via state.Dir() (which the integration sites pre-set
// via newIntegrationStateDir → PORTAL_STATE_DIR). A stateDir resolution
// failure is tolerated: state.Dir returns the resolved path even when the
// directory does not exist on disk; EagerSignalCore only consults stateDir
// to derive per-FIFO paths inside its loop, so an empty / unresolved
// stateDir still produces a well-formed orchestrator.
func buildIntegrationOrchestrator(t *testing.T, client *tmux.Client, opts orchestratorOpts) *bootstrap.Orchestrator {
	t.Helper()

	stateDir, _ := state.Dir()

	// Translate orchestratorOpts → variadic With* options. Only fields the
	// caller explicitly set are forwarded; unset fields fall through to
	// NewWithDefaults' NoOp policy. The EagerSignaler conditional default
	// (real when Restore is real, NoOp otherwise, explicit opt-out wins)
	// is implemented inside NewWithDefaults — this builder threads the
	// caller's intent verbatim and lets the helper compute the default.
	var withOpts []bootstrap.Option
	if opts.Hooks != nil {
		withOpts = append(withOpts, bootstrap.WithHooks(opts.Hooks))
	}
	if opts.OrphanSweeper != nil {
		withOpts = append(withOpts, bootstrap.WithOrphanSweeper(opts.OrphanSweeper))
	}
	if opts.Saver != nil {
		withOpts = append(withOpts, bootstrap.WithSaver(opts.Saver))
	}
	if opts.Restore != nil {
		withOpts = append(withOpts, bootstrap.WithRestore(opts.Restore))
	}
	if opts.EagerSignaler != nil {
		withOpts = append(withOpts, bootstrap.WithEagerSignaler(opts.EagerSignaler))
	}
	if opts.StaleMarkers != nil {
		withOpts = append(withOpts, bootstrap.WithStaleMarkers(opts.StaleMarkers))
	}
	if opts.Sweeper != nil {
		withOpts = append(withOpts, bootstrap.WithSweeper(opts.Sweeper))
	}
	if opts.Clean != nil {
		withOpts = append(withOpts, bootstrap.WithClean(opts.Clean))
	}

	return bootstrap.NewWithDefaults(
		client,
		stateDir,
		opts.Logger,
		&bootstrapadapter.RestoringMarker{Client: client},
		withOpts...,
	)
}

// newIntegrationStateDir builds an isolated portal state directory rooted at
// t.TempDir(), wires it via PORTAL_STATE_DIR (auto-restored by t.Setenv on
// test teardown), and runs state.EnsureDir so callers can immediately write
// sessions.json / scrollback / FIFOs into the returned path.
//
// Paired with restoretest.OpenTestLogger (a silent *slog.Logger factory) the
// two helpers replace the nine-site stateDir + EnsureDir + log-open preamble
// previously copy-pasted across the cmd/bootstrap integration tests. They
// remain split because not every site that needs the stateDir half also needs
// a logger (e.g. the orchestrator end-to-end smoke test wires no logger at all
// and lets the orchestrator substitute its discardLogger).
func newIntegrationStateDir(t *testing.T) string {
	t.Helper()
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	return stateDir
}
