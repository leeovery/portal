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
	"log/slog"

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
// under the bootstrap component, and is also installed as the saver-barrier
// WARN sink (also bootstrap). nil is tolerated: the underlying tmux
// migration substitutes the io.Discard-backed default when Logger is nil.
//
// VersionLogger is installed as the "daemon.version write" breadcrumb sink so
// the bootstrap-side defensive WriteVersionFile emits under the daemon
// component — matching the daemon-startup call site (spec § Change 3). It is
// a separate field from Logger precisely because the breadcrumb is a daemon
// line while the migration/barrier lines are bootstrap lines. nil is
// tolerated (the setter ignores nil).
type HookRegistrar struct {
	Client        *tmux.Client
	Logger        *slog.Logger
	VersionLogger *slog.Logger
}

// RegisterPortalHooks delegates to tmux.RegisterPortalHooks so the
// migration logger seam is wired through.
//
// As a side-effect, the same *slog.Logger is installed as two
// internal/tmux package-level logger sinks:
//
//   - the saver-barrier Logger — used by BOTH saver-side barriers'
//     WARN-on-timeout paths: the kill-barrier helper
//     (killSaverAndWaitForDaemon / escalateKillToSIGKILL) AND the
//     readiness barrier (waitForSaverDaemonReady).
//   - the version-writer Logger — used by the bootstrap-side defensive
//     portalSaverWriteVersionFile call site so its "daemon.version write"
//     DEBUG breadcrumb lands in portal.log, matching the daemon-startup
//     call site (spec § Change 3, Acceptance #9).
//
// Both setters are idempotent and tolerate a nil argument, so calling
// them on every RegisterPortalHooks invocation is safe.
func (r *HookRegistrar) RegisterPortalHooks() error {
	tmux.SetBarrierLogger(r.Logger)
	tmux.SetVersionWriterLogger(r.VersionLogger)
	return tmux.RegisterPortalHooks(r.Client, r.Logger)
}

// RestoreAdapter wraps a *restore.Orchestrator so its Restore method
// satisfies bootstrap.Restorer. The bootstrap orchestrator owns the
// @portal-restoring marker lifecycle separately (steps 3 and 8), so the
// inner Restore must not bundle marker management. The orphan-FIFO
// sweep that historically lived inside this adapter has been promoted
// to its own bootstrap step (FIFOSweeper / step 10) — keeping this
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

// SetProgress installs the §10.4 per-session progress callback onto the wrapped
// restore.Orchestrator. It satisfies the optional bootstrap.RestoreProgressSink
// seam: bootstrap step 6 installs a ctx-emitter-forwarding callback ONLY on the
// cold/TUI concurrent route (where a progress emitter is wired); the synchronous
// warm/CLI route never calls this, so the inner Orchestrator's Progress stays
// nil and its restore loop is byte-for-byte unchanged. Keeping this off the
// Restorer interface preserves the Restore() (bool, error) contract — this is a
// purely additive instrumentation seam.
func (a *RestoreAdapter) SetProgress(fn func(n, m int)) { a.Inner.Progress = fn }

// NewRestoreAdapter constructs a *RestoreAdapter wrapping a freshly-built
// inner *restore.Orchestrator. It is the single canonical constructor for
// integration-test sites that previously open-coded the
// `restoreInner := &restore.Orchestrator{...}` /
// `&RestoreAdapter{Inner: restoreInner}` two-step preamble. Production
// wiring at cmd/bootstrap_production.go retains its open-coded form for
// parity with the surrounding inline-struct adapters at that site
// (HookRegistrar, RestoringMarker, EagerSignalCore, MarkerCleanupCore);
// migrating it is mechanical and out of scope for the constructor's
// introduction.
//
// Logger is forwarded into the inner Orchestrator unchanged; it must be a
// real *slog.Logger (production passes log.For("restore")).
func NewRestoreAdapter(client *tmux.Client, stateDir string, logger *slog.Logger) *RestoreAdapter {
	return &RestoreAdapter{
		Inner: &restore.Orchestrator{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
	}
}

// FIFOSweeper satisfies bootstrap.FIFOSweeper. Step 10 of the bootstrap
// sequence — runs after step 8 clears @portal-restoring (so the daemon's
// suppression window has closed) and after step 9 (CleanStaleMarkers) so
// any stale markers protecting orphan FIFOs are unset first, but before
// step 11 (CleanStale), so the per-pane @portal-skeleton-* markers from
// step 6 are still set on the live tmux server. Those markers outlive
// @portal-restoring and are cleared per-pane on hydration;
// ListSkeletonMarkers is the source of truth for "which paneKeys deserve
// their FIFO".
//
// Sweep is best-effort, but visibility-preserving:
//
//   - A ShowAllServerOptions failure (the seam ListSkeletonMarkers uses)
//     is wrapped and returned so the orchestrator's step-10 Warn-and-swallow
//     path logs it uniformly with the per-FIFO Warn lines below. Bootstrap
//     does not abort — the orchestrator continues to step 11 (CleanStale).
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
	Logger   *slog.Logger
}

// Sweep enumerates the live skeleton markers from the tmux server and
// removes any hydrate-*.fifo file in StateDir whose paneKey is not in
// that set. A ListSkeletonMarkers failure is wrapped and returned so the
// orchestrator's step-10 Warn-and-swallow path logs it uniformly — the
// orchestrator never aborts on a non-nil sweep error.
func (s *FIFOSweeper) Sweep() error {
	markers, err := state.ListSkeletonMarkers(s.Client)
	if err != nil {
		return fmt.Errorf("list skeleton markers: %w", err)
	}
	return state.SweepOrphanFIFOs(s.StateDir, markers, s.Logger)
}
