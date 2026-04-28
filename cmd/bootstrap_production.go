package cmd

// Production-shape adapters that wire the bootstrap.Orchestrator step
// interfaces to their concrete implementations across internal/tmux,
// internal/restore, internal/state, and internal/hooks. Kept in cmd/
// (rather than cmd/bootstrap) so the bootstrap package stays free of
// dependencies on internal/restore, internal/state, and internal/hooks
// — the orchestrator owns ordering and the adapters own composition.
//
// The two simplest adapters (HookRegistrar, RestoringMarker) live in
// internal/bootstrapadapter so test suites can import production-shape
// wirings without pulling in the rest of cmd/. Production-only adapters
// that carry richer context (state dir, hooks store, restore
// orchestrator, logger) remain here — they are not reusable from tests
// in their current shape.

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// saverAdapter wraps tmux.EnsurePortalSaverVersion to satisfy
// bootstrap.SaverBootstrapper. Carries the binary's ldflags-injected
// version so the version-marker upgrade protocol kicks in on release-
// build mismatches. Step 4 of the bootstrap sequence.
type saverAdapter struct {
	client   *tmux.Client
	stateDir string
}

// EnsureSaver delegates to tmux.EnsurePortalSaverVersion using the
// package-level version variable (cmd.version, set via -ldflags).
func (a *saverAdapter) EnsureSaver() error {
	return tmux.EnsurePortalSaverVersion(a.client, a.stateDir, version)
}

// restoreOrchestratorAdapter wraps an *internal/restore.Orchestrator's
// Restore() method to satisfy bootstrap.Restorer. The bootstrap
// orchestrator owns the @portal-restoring marker lifecycle (steps 3 and
// 6); this adapter only runs the bare restore. After the inner restore
// completes it sweeps orphan hydrate-*.fifo files whose paneKey is no
// longer represented by a live skeleton marker. Step 5 of the bootstrap
// sequence.
type restoreOrchestratorAdapter struct {
	inner    *restore.Orchestrator
	client   *tmux.Client
	stateDir string
	logger   *state.Logger
}

// Restore runs the wrapped restore orchestrator and then sweeps orphan
// FIFOs. Sweep is best-effort: any failure to enumerate skeleton
// markers degrades to "skip the sweep" rather than aborting bootstrap.
// Inner restore errors propagate verbatim — the bootstrap orchestrator
// classifies state.ErrCorruptIndex via errors.Is and emits a soft
// warning.
func (a *restoreOrchestratorAdapter) Restore() error {
	if err := a.inner.Restore(); err != nil {
		return err
	}
	// Sweep orphan FIFOs. Skeleton markers are still set at this point
	// (step 6 clears @portal-restoring; per-pane skeleton markers stay
	// up until hydration completes per pane), so ListSkeletonMarkers is
	// the source of truth for "which paneKeys deserve their FIFO".
	markers, err := state.ListSkeletonMarkers(a.client)
	if err != nil {
		return nil // soft-fail: sweep is best-effort.
	}
	_ = state.SweepOrphanFIFOs(a.stateDir, markers, a.logger)
	return nil
}

// cleanStaleAdapter prunes the on-disk hooks store of entries whose
// structural key no longer matches a live tmux pane. Step 7 of the
// bootstrap sequence; best-effort per spec.
type cleanStaleAdapter struct {
	client *tmux.Client
	store  *hooks.Store
}

// CleanStale fetches the live pane keys from tmux and asks the hooks
// store to drop any entries that no longer match. A ListAllPanes
// failure degrades to no-op (returns nil) so a transient tmux error
// during bootstrap never aborts the user's command — matches the
// safety-net semantic in `portal clean`.
func (a *cleanStaleAdapter) CleanStale() error {
	livePanes, err := a.client.ListAllPanes()
	if err != nil {
		return nil
	}
	_, err = a.store.CleanStale(livePanes)
	return err
}

// noopStaleCleaner is the production fallback when loadHookStore
// fails to resolve the hooks.json path (e.g. HOME unset). Returning
// nil keeps bootstrap moving — failed hook-config resolution is not a
// reason to block the user's command.
type noopStaleCleaner struct{}

// CleanStale is a no-op.
func (noopStaleCleaner) CleanStale() error { return nil }

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
// CleanStale step degrades to noopStaleCleaner.
func buildProductionOrchestrator() (*bootstrap.Orchestrator, *tmux.Client) {
	client := tmux.NewClient(&tmux.RealCommander{})

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
		cleaner = &cleanStaleAdapter{client: client, store: hookStore}
	} else {
		cleaner = noopStaleCleaner{}
	}

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	orch := &bootstrap.Orchestrator{
		Server:    client,
		Hooks:     &bootstrapadapter.HookRegistrar{Client: client},
		Restoring: &bootstrapadapter.RestoringMarker{Client: client},
		Saver:     &saverAdapter{client: client, stateDir: stateDir},
		Restore: &restoreOrchestratorAdapter{
			inner:    restoreInner,
			client:   client,
			stateDir: stateDir,
			logger:   logger,
		},
		Clean:  cleaner,
		Logger: logger,
	}
	return orch, client
}
