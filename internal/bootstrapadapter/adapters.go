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
	"fmt"

	"github.com/leeovery/portal/cmd/bootstrap"
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
//
// Logger is forwarded to tmux.RegisterPortalHooks so the one-shot
// signal-hydrate migration's INFO/WARN diagnostics land in portal.log
// under the bootstrap component. nil is tolerated: *state.Logger is
// itself nil-safe and the underlying tmux migration substitutes a no-op
// MigrationLogger when Logger is nil.
type HookRegistrar struct {
	Client *tmux.Client
	Logger *state.Logger
}

// RegisterPortalHooks delegates to tmux.RegisterPortalHooks so the
// migration logger seam is wired through. *state.Logger satisfies
// tmux.MigrationLogger structurally.
func (r *HookRegistrar) RegisterPortalHooks() error {
	return tmux.RegisterPortalHooks(r.Client, r.Logger)
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

// FIFOSweeper satisfies bootstrap.FIFOSweeper. Step 8 of the bootstrap
// sequence — runs after step 6 clears @portal-restoring (so the daemon's
// suppression window has closed) and after step 7 (CleanStaleMarkers) so
// any stale markers protecting orphan FIFOs are unset first, but before
// step 9 (CleanStale), so the per-pane @portal-skeleton-* markers from
// step 5 are still set on the live tmux server. Those markers outlive
// @portal-restoring and are cleared per-pane on hydration;
// ListSkeletonMarkers is the source of truth for "which paneKeys deserve
// their FIFO".
//
// Sweep is best-effort, but visibility-preserving:
//
//   - A ShowAllServerOptions failure (the seam ListSkeletonMarkers uses)
//     is wrapped and returned so the orchestrator's step-8 Warn-and-swallow
//     path logs it uniformly with the per-FIFO Warn lines below. Bootstrap
//     does not abort — the orchestrator continues to step 9 (CleanStale).
//   - Per-FIFO removal errors are logged via Logger and skipped inside
//     state.SweepOrphanFIFOs.
//
// Client is typed as state.ServerOptionLister rather than *tmux.Client so
// unit tests can inject a stub that simulates a tmux failure without
// standing up a real server. Production wiring still passes *tmux.Client,
// which satisfies the interface via its ShowAllServerOptions method.
//
// The adapter lives here (rather than cmd/) so integration tests can
// import it directly. Its dependencies are all internal package types
// (no cmd-package globals like the ldflags-injected version), which
// makes it reusable from tests without dragging in cmd/.
type FIFOSweeper struct {
	Client   state.ServerOptionLister
	StateDir string
	Logger   *state.Logger // nil tolerated; *state.Logger is nil-safe.
}

// Sweep enumerates the live skeleton markers from the tmux server and
// removes any hydrate-*.fifo file in StateDir whose paneKey is not in
// that set. A ListSkeletonMarkers failure is wrapped and returned so the
// orchestrator's step-8 Warn-and-swallow path logs it uniformly — the
// orchestrator never aborts on a non-nil sweep error.
func (s *FIFOSweeper) Sweep() error {
	markers, err := state.ListSkeletonMarkers(s.Client)
	if err != nil {
		return fmt.Errorf("list skeleton markers: %w", err)
	}
	return state.SweepOrphanFIFOs(s.StateDir, markers, s.Logger)
}

// staleMarkerClient is the union of *tmux.Client primitives the
// StaleMarkerCleaner adapter consumes. Defining the interface inside this
// package keeps the bootstrap.StaleMarkerCleaner construction free of any
// direct *tmux.Client coupling so unit tests can inject a single stub
// satisfying all three methods. *tmux.Client satisfies the interface
// structurally via its existing ShowAllServerOptions, ListAllPanesWithFormat,
// and UnsetServerOption methods.
//
// The interface is intentionally narrow — three methods, the exact set the
// concrete bootstrap.StaleMarkerCleaner cleanup loop needs. A wider seam
// (e.g. embedding the whole *tmux.Client) would expose drift surface for
// future contributors to call ListAllPanes (the error-swallowing variant)
// from the cleanup loop and re-introduce the mass-unset hazard.
type staleMarkerClient interface {
	ShowAllServerOptions() (string, error)
	ListAllPanesWithFormat(format string) (string, error)
	UnsetServerOption(name string) error
}

// StaleMarkerCleaner satisfies bootstrap.MarkerCleaner. Step 7 of the
// bootstrap sequence — runs strictly after step 6 (Clear @portal-restoring)
// so it observes post-restore tmux state, and strictly before step 8
// (FIFOSweeper) so any stale markers protecting orphan FIFOs are unset
// first, allowing those FIFOs to be reclaimed in the same bootstrap.
//
// CleanStaleMarkers wires three production seams to the concrete
// bootstrap.StaleMarkerCleaner cleanup loop:
//
//   - MarkerLister: state.ListSkeletonMarkers driven by the wrapped
//     client's ShowAllServerOptions (*tmux.Client satisfies
//     state.ServerOptionLister structurally).
//   - LivePaneLister: the wrapped client's ListAllPanesWithFormat,
//     called with the canonical literal
//     `#{session_name}:#{window_index}.#{pane_index}`. This is the
//     error-propagating variant required by spec §Fix Component B
//     (Adapter Wiring) so a transient tmux failure surfaces as a soft
//     warning rather than the silently-empty-set mass-unset hazard
//     posed by the pre-fix ListAllPanes path.
//   - MarkerUnsetter: the wrapped client's UnsetServerOption; the
//     concrete bootstrap.StaleMarkerCleaner composes the option name as
//     state.SkeletonMarkerPrefix + paneKey before each call.
//
// Logger is forwarded into the concrete bootstrap.StaleMarkerCleaner so
// per-unset-failure and malformed-live-pane-line diagnostics land in
// portal.log under ComponentBootstrap. nil is tolerated: *state.Logger is
// itself nil-safe and the concrete cleanup loop's Warn calls become
// no-ops. This mirrors FIFOSweeper's Logger contract.
//
// Client must be non-nil; behaviour with a nil Client is undefined and
// will panic at the first method call (matching tmux.Client semantics
// elsewhere in the codebase). Production wiring passes *tmux.Client; tests
// inject lightweight stubs satisfying staleMarkerClient.
type StaleMarkerCleaner struct {
	Client staleMarkerClient
	Logger *state.Logger // nil tolerated; *state.Logger is nil-safe.
}

// markerListerFunc adapts state.ListSkeletonMarkers to the
// bootstrap.MarkerLister interface without standing up a per-call wrapper
// type. The concrete bootstrap.StaleMarkerCleaner accepts an interface; this
// keeps the seam surface minimal in CleanStaleMarkers below.
type markerListerFunc func() (map[string]struct{}, error)

func (f markerListerFunc) ListSkeletonMarkers() (map[string]struct{}, error) { return f() }

// CleanStaleMarkers constructs a fresh bootstrap.StaleMarkerCleaner per
// invocation, wiring the staleMarkerClient seams to the orchestrator's
// MarkerLister/LivePaneLister/MarkerUnsetter contracts. Per-call
// construction is deliberate: the adapter holds no mutable state, and a
// per-bootstrap re-wire avoids any risk of stale closures across
// bootstrap runs in long-lived test processes.
//
// Error propagation: any non-nil error from the underlying cleanup loop
// is returned verbatim. The orchestrator's step-7 Warn-and-swallow path
// (cmd/bootstrap/bootstrap.go) logs the error and continues; bootstrap
// never aborts on a stale-marker cleanup failure. See spec §Fix Component
// B (Soft-Warning Posture).
func (a *StaleMarkerCleaner) CleanStaleMarkers() error {
	cleaner := &bootstrap.StaleMarkerCleaner{
		Markers:  markerListerFunc(func() (map[string]struct{}, error) { return state.ListSkeletonMarkers(a.Client) }),
		Panes:    a.Client,
		Unsetter: a.Client,
		Logger:   a.Logger,
	}
	return cleaner.CleanStaleMarkers()
}
