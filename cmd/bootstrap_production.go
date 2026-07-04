package cmd

// Production-shape adapters that wire the bootstrap.Orchestrator step
// interfaces to their concrete implementations across internal/tmux,
// internal/restore, and internal/state. Kept in cmd/
// (rather than cmd/bootstrap) so the bootstrap package stays free of
// dependencies on internal/restore and internal/state
// — the orchestrator owns ordering and the adapters own composition.
//
// Adapters split across two homes by reusability:
//
//   - internal/bootstrapadapter (Pascal-cased, exported): adapters whose
//     dependencies are all available from internal/* packages — the
//     *tmux.Client, the *restore.Orchestrator, the state directory, the
//     component-bound *slog.Logger. Test suites import these directly so production-shape
//     wirings are reusable without pulling in the rest of cmd/. Currently:
//     HookRegistrar, RestoringMarker, NewOrphanSweeper (Component B),
//     RestoreAdapter, FIFOSweeper.
//
//   - cmd/bootstrap_production.go (camelCase, unexported): adapters that
//     compose dependencies test code cannot reach in this package's
//     current shape — the package-level cmd.version variable
//     (saverAdapter). Lowercase reflects "this struct is the wiring
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
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

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

// commanderFactory is the indirection seam tests use to inject a
// wrapping tmux.Commander into the production orchestrator-builder
// chain. Production code leaves it at the default — a freshly
// constructed *tmux.RealCommander, byte-identical to what
// tmux.DefaultClient produces — so the production binary is
// unaffected. Integration tests under //go:build integration override
// this var (under t.Cleanup restore) to inject, for example, a
// TransientListPanesCommander wrapping a socket-anchored inner
// Commander so the entire bootstrap pipeline observes
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
// Logger: each step adapter is wired with the component-bound *slog.Logger
// it logs under (bootstrap for the orchestrator and most steps, restore for
// the Restore adapter, hydrate for the eager signaler, daemon for the
// version-writer breadcrumb). The handler is configured once by main ->
// log.Init.
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

	// Component-bound loggers. The orchestrator and most steps log under the
	// bootstrap component; the Restore adapter logs under restore, the eager
	// signaler under hydrate, and the version-writer breadcrumb under daemon.
	// Rotation and the append-only writer discipline are handler-owned
	// (Phase 2), so there is no per-process log open here.
	logger := bootstrapLogger

	// restoreInner.Progress is intentionally left nil here. The §10.4 per-session
	// N/M progress callback is installed at Run time by bootstrap step 6 via the
	// RestoreProgressSink seam (RestoreAdapter.SetProgress) — but ONLY on the
	// concurrent cold-boot route, where a progress emitter is wired through the
	// context. On the synchronous warm/CLI route no emitter exists, SetProgress is
	// never called, Progress stays nil, and the restore loop is byte-for-byte
	// unchanged. task 5-4 maps the forwarded RestoreN/M onto the friendly
	// "Restoring sessions (N/M)" loading-screen label.
	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   restoreLogger,
	}

	orch := &bootstrap.Orchestrator{
		Server:        client,
		Hooks:         &bootstrapadapter.HookRegistrar{Client: client, Logger: logger, VersionLogger: daemonLogger},
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
			Logger:   hydrateLogger,
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
		// Latch stamps the version-stamped @portal-bootstrapped latch as the
		// final action of a successful Run. *tmux.Client satisfies
		// bootstrap.LatchWriter via SetServerOption; Version is the same
		// ldflags-injected cmd.version the saverAdapter reads.
		Latch:   client,
		Version: version,
		Logger:  logger,
	}
	return orch, client
}
