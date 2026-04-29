// Package bootstrapadapter holds the production-shape adapters that wire
// tmux-client-level primitives to the bootstrap.Orchestrator step
// interfaces. Each adapter is a thin wrapper:
//
//   - Method-name shaping: bootstrap.RestoringMarker requires Set/Clear;
//     *tmux.Client exposes Set/UnsetServerOption with a name argument.
//   - Argument capture: the long-lived *tmux.Client is held on the adapter
//     so the orchestrator's step interfaces stay argument-free.
//
// The adapters live in their own package (rather than under cmd/) so test
// suites that need production-equivalent wiring can import them without
// pulling in the rest of cmd/. Production-only adapters that carry richer
// context (state dir, hooks store, restore orchestrator, logger) stay in
// cmd/bootstrap_production.go — they are not reusable from tests in their
// current shape.
package bootstrapadapter

import (
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// RestoringMarker manages the @portal-restoring server-option lifecycle
// that suppresses the save daemon during skeleton restore. Implements
// bootstrap.RestoringMarker — steps 3 (Set) and 6 (Clear) of the
// bootstrap sequence.
//
// The wrapped Client must be non-nil; behaviour with a nil client is
// undefined and will panic at the first method call (matching tmux.Client
// semantics elsewhere in the codebase).
type RestoringMarker struct {
	Client *tmux.Client
}

// Set writes @portal-restoring="1" at server scope. The option name comes
// from state.RestoringMarkerName so this adapter cannot drift from the
// canonical constant.
func (m *RestoringMarker) Set() error {
	return m.Client.SetServerOption(state.RestoringMarkerName, "1")
}

// Clear removes @portal-restoring at server scope. Idempotent under tmux
// — unsetting an already-absent option is a no-op and does not error.
func (m *RestoringMarker) Clear() error {
	return m.Client.UnsetServerOption(state.RestoringMarkerName)
}

// HookRegistrar wraps tmux.RegisterPortalHooks to satisfy
// bootstrap.HookRegistrar. Step 2 of the bootstrap sequence; idempotent
// — safe to invoke on every bootstrap.
type HookRegistrar struct {
	Client *tmux.Client
}

// RegisterPortalHooks delegates to the package-level helper on the
// wrapped client.
func (r *HookRegistrar) RegisterPortalHooks() error {
	return tmux.RegisterPortalHooks(r.Client)
}

// RestoreAdapter wraps a *restore.Orchestrator so its Restore method
// satisfies bootstrap.Restorer. The bootstrap orchestrator owns the
// @portal-restoring marker lifecycle separately (steps 3 and 6), so the
// inner Restore must not bundle marker management. The orphan-FIFO
// sweep that historically lived inside this adapter has been promoted
// to its own bootstrap step (FIFOSweeper / step 7) — keeping this
// adapter a pure pass-through means the same wrapper is usable from
// integration tests without dragging in state-dir / logger arguments.
//
// Inner must be non-nil; behaviour with a nil Inner is undefined and
// will panic at the first Restore call (matching the codebase's
// "explicit fields, fail loud" convention for adapter wiring).
type RestoreAdapter struct {
	Inner *restore.Orchestrator
}

// Restore delegates to the wrapped restore.Orchestrator's Restore method,
// returning the (corrupt, err) tuple verbatim under the bootstrap.Restorer
// contract.
func (a *RestoreAdapter) Restore() (bool, error) { return a.Inner.Restore() }

// FIFOSweeper satisfies bootstrap.FIFOSweeper. Step 7 of the bootstrap
// sequence — runs after step 6 clears @portal-restoring (so the daemon's
// suppression window has closed) but before step 8 (CleanStale), so the
// per-pane @portal-skeleton-* markers from step 5 are still set on the
// live tmux server. Those markers outlive @portal-restoring and are
// cleared per-pane on hydration; ListSkeletonMarkers is the source of
// truth for "which paneKeys deserve their FIFO".
//
// Sweep is best-effort:
//
//   - A ShowAllServerOptions failure (the seam ListSkeletonMarkers uses)
//     degrades to nil so a transient tmux failure does not interrupt
//     bootstrap — the next bootstrap retries.
//   - Per-FIFO removal errors are logged via Logger and skipped inside
//     state.SweepOrphanFIFOs.
//
// The adapter lives here (rather than cmd/) so integration tests can
// import it directly. Its dependencies are all internal package types
// (no cmd-package globals like the ldflags-injected version), which
// makes it reusable from tests without dragging in cmd/.
type FIFOSweeper struct {
	Client   *tmux.Client
	StateDir string
	Logger   *state.Logger // nil tolerated; *state.Logger is nil-safe.
}

// Sweep enumerates the live skeleton markers from the tmux server and
// removes any hydrate-*.fifo file in StateDir whose paneKey is not in
// that set. A non-nil ListSkeletonMarkers error degrades to nil so the
// orchestrator's Step 7 path is not interrupted by transient tmux
// failures.
func (s *FIFOSweeper) Sweep() error {
	markers, err := state.ListSkeletonMarkers(s.Client)
	if err != nil {
		return nil // soft-fail: sweep is best-effort.
	}
	return state.SweepOrphanFIFOs(s.StateDir, markers, s.Logger)
}
