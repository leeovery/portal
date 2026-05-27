package cmd

// Production-shape adapters that wire the bootstrap.Orchestrator step
// interfaces to their concrete implementations across internal/tmux,
// internal/restore, internal/state, and internal/hooks. Kept in cmd/
// (rather than cmd/bootstrap) so the bootstrap package stays free of
// dependencies on internal/restore, internal/state, and internal/hooks
// — the orchestrator owns ordering and the adapters own composition.
//
// Adapters split across two homes by reusability:
//
//   - internal/bootstrapadapter (Pascal-cased, exported): adapters whose
//     dependencies are all available from internal/* packages — the
//     *tmux.Client, the *restore.Orchestrator, the state directory, the
//     *state.Logger. Test suites import these directly so production-shape
//     wirings are reusable without pulling in the rest of cmd/. Currently:
//     HookRegistrar, RestoringMarker, NewOrphanSweeper (Component B),
//     RestoreAdapter, FIFOSweeper.
//
//   - cmd/bootstrap_production.go (camelCase, unexported): adapters that
//     compose dependencies test code cannot reach in this package's
//     current shape — the package-level cmd.version variable
//     (saverAdapter), the hook-store path-resolution chain
//     (cleanStaleAdapter). Lowercase reflects "this struct is the wiring
//     this binary uses; tests compose their own." The stale-marker
//     cleanup core (bootstrap.MarkerCleanupCore) is also constructed
//     inline at the wiring site below — *tmux.Client satisfies every one
//     of its seam fields (Markers, Panes, Unsetter) directly, so no
//     adapter glue is needed.
//
//     The bootstrap.EagerSignalCore is deliberately NOT in
//     internal/bootstrapadapter for the same reason: its production
//     wiring is a zero-value seam composition (Markers=*tmux.Client,
//     Signaler=state.DefaultFIFOSignaler{}) with no method wrapping,
//     so it is constructed inline at the wiring site below — mirroring
//     MarkerCleanupCore. Tests build their own EagerSignalCore literal
//     when they need it.

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// cleanStaleNoopLogger is the local no-op stand-in substituted inside
// (*cleanStaleAdapter).CleanStale when the Logger field is nil. Mirrors
// the noopLogger pattern in cmd/bootstrap so the adapter can invoke
// Logger.Warn / Logger.Debug unconditionally — matching the nil-tolerance
// contract that MarkerCleanupCore.CleanStaleMarkers uses at
// cmd/bootstrap/stale_marker_cleanup.go:109-112.
type cleanStaleNoopLogger struct{}

// Debug is a no-op.
func (cleanStaleNoopLogger) Debug(component, format string, args ...any) {}

// Info is a no-op.
func (cleanStaleNoopLogger) Info(component, format string, args ...any) {}

// Warn is a no-op.
func (cleanStaleNoopLogger) Warn(component, format string, args ...any) {}

// Error is a no-op.
func (cleanStaleNoopLogger) Error(component, format string, args ...any) {}

// Compile-time assertion that cleanStaleNoopLogger satisfies bootstrap.Logger.
var _ bootstrap.Logger = cleanStaleNoopLogger{}

// saverAdapter wraps tmux.EnsurePortalSaverVersion to satisfy
// bootstrap.SaverBootstrapper. Carries the binary's ldflags-injected
// version so the version-marker upgrade protocol kicks in on release-
// build mismatches. Step 5 of the bootstrap sequence.
type saverAdapter struct {
	client   *tmux.Client
	stateDir string
}

// EnsureSaver delegates to tmux.EnsurePortalSaverVersion using the
// package-level version variable (cmd.version, set via -ldflags).
func (a *saverAdapter) EnsureSaver() error {
	return tmux.EnsurePortalSaverVersion(a.client, a.stateDir, version)
}

// cleanStaleAdapter prunes the on-disk hooks store of entries whose
// structural key no longer matches a live tmux pane. Step 11 of the
// bootstrap sequence; best-effort per spec.
//
// The lister field is the AllPaneLister interface (declared in
// cmd/clean.go) — production wiring passes a *tmux.Client which
// satisfies the interface via ListAllPanes. Using the interface (rather
// than the concrete *tmux.Client) lets unit tests substitute a stub
// without needing a test-only adapter mirror.
type cleanStaleAdapter struct {
	lister AllPaneLister
	store  *hooks.Store
	Logger bootstrap.Logger
}

// CleanStale delegates to runHookStaleCleanup with the bootstrap-step-11
// policy (returnError — the orchestrator Warn-and-swallows at its own
// level so errors surface through the StaleCleaner interface) and no
// per-removal callback (bootstrap has nothing to print to the user).
// Algorithm, hazard guard, and log format strings all live in the
// shared helper — see cmd/run_hook_stale_cleanup.go for the full design
// rationale.
func (a *cleanStaleAdapter) CleanStale() error {
	return runHookStaleCleanup(a.lister, a.store, a.Logger, returnError, nil)
}

// Compile-time assertion that *tmux.Client satisfies AllPaneLister so
// the production wiring at buildProductionOrchestrator can pass a
// *tmux.Client into the cleanStaleAdapter.lister field unchanged.
var _ AllPaneLister = (*tmux.Client)(nil)

// commanderFactory is the indirection seam tests use to inject a
// wrapping tmux.Commander into the production orchestrator-builder
// chain. Production code leaves it at the default — a freshly
// constructed *tmux.RealCommander, byte-identical to what
// tmux.DefaultClient produces — so the production binary is
// unaffected. Integration tests under //go:build integration override
// this var (under t.Cleanup restore) to inject, for example, a
// TransientListPanesCommander wrapping a socket-anchored inner
// Commander so the entire eleven-step bootstrap pipeline observes
// the test's failure policy via a single, structurally-pinned seam.
//
// Discipline: callers MUST NOT cache the Client built from this
// factory across builds — the factory is invoked once per
// buildProductionOrchestrator call so a test that flips the factory
// between phases gets the new Commander in the next build.
var commanderFactory = func() tmux.Commander { return &tmux.RealCommander{} }

// buildProductionOrchestrator constructs a fully-wired
// *bootstrap.Orchestrator and the underlying *tmux.Client to be shared
// with downstream commands via cmd.Context(). The construction is
// pulled into a helper so buildBootstrapDeps stays a small dispatcher
// between test-mode and production-mode wiring.
//
// Logger: opened via openNoRotateLogger (non-rotating since this is
// not the daemon). On any error the helper returns a nil logger;
// state.Logger and bootstrap.Logger both tolerate nil receivers /
// values, so callers downstream do not have to nil-check.
//
// HookStore: when loadHookStore fails (path resolution error) the
// CleanStale step degrades to bootstrap.NoOpStaleCleaner.
//
// Commander seam: the underlying *tmux.Client is built via
// commanderFactory rather than tmux.DefaultClient so integration
// tests can inject a wrapping Commander (see commanderFactory godoc).
// The default factory returns &tmux.RealCommander{}, byte-identical
// to tmux.DefaultClient's construction.
func buildProductionOrchestrator() (*bootstrap.Orchestrator, *tmux.Client) {
	client := tmux.NewClient(commanderFactory())

	// Resolve state dir once. An error here does not abort bootstrap —
	// state.EnsureDir will be retried inside individual subsystems and
	// the orchestrator's logger will surface the failure to portal.log
	// when it eventually flows through.
	stateDir, _ := state.Dir()

	// Open a non-rotating logger. Bootstrap is not the daemon so it
	// must not rename portal.log under another writer.
	logger, _ := openNoRotateLogger()

	// Resolve the hooks store. On failure the CleanStale step is
	// downgraded to a no-op rather than aborting bootstrap.
	var cleaner bootstrap.StaleCleaner
	if hookStore, err := loadHookStore(); err == nil && hookStore != nil {
		cleaner = &cleanStaleAdapter{lister: client, store: hookStore, Logger: logger}
	} else {
		cleaner = bootstrap.NoOpStaleCleaner{}
	}

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	orch := &bootstrap.Orchestrator{
		Server:        client,
		Hooks:         &bootstrapadapter.HookRegistrar{Client: client, Logger: logger},
		Restoring:     &bootstrapadapter.RestoringMarker{Client: client},
		OrphanSweeper: bootstrapadapter.NewOrphanSweeper(client, logger),
		Saver:         &saverAdapter{client: client, stateDir: stateDir},
		Restore:       &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
		// EagerSignaler is constructed inline (mirroring MarkerCleanupCore)
		// because every seam field is satisfiable directly: *tmux.Client
		// implements state.ServerOptionLister via ShowAllServerOptions, and
		// state.DefaultFIFOSignaler{} (the production no-seam wrapper around
		// state.SendHydrateSignal) drops in as a zero value. The
		// orchestrator-scope stateDir resolved above is reused (Restore,
		// FIFOSweeper, EagerSignalCore) so all three steps observe the same
		// state directory.
		EagerSignaler: &bootstrap.EagerSignalCore{
			Markers:  client,
			StateDir: stateDir,
			Signaler: state.DefaultFIFOSignaler{},
			Logger:   logger,
		},
		StaleMarkers: &bootstrap.MarkerCleanupCore{
			Markers:  client,
			Panes:    client,
			Unsetter: client,
			Logger:   logger,
		},
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		Clean:  cleaner,
		Logger: logger,
	}
	return orch, client
}
