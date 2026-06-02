package bootstrap

import (
	"log/slog"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// signalLogger is the signal-component-bound package logger for the
// FIFO-signaling mechanism (Subsystem prefix taxonomy: signal owns
// EagerSignalHydrate's per-FIFO write outcomes and the lower-level FIFO send
// plumbing in internal/state). The per-FIFO write-failure WARN and the
// per-FIFO success DEBUG breadcrumb both render under component=signal — NOT
// hydrate — so `grep signal:` reconstructs the FIFO-signaling behaviour.
//
// It MUST NOT be named a bare `logger`: EagerSignalHydrate carries no
// function-local `logger` anymore, but the package var stays explicitly named
// signalLogger to match the Phase 5 naming convention (captureLogger,
// cleanLogger, saverLogger) and to make the component routing legible at the
// call site. Bound once at package init via log.For so it routes through the
// shared handler indirection (observing later Init / SetTestHandler swaps).
var signalLogger = log.For("signal")

// EagerSignalCore is the orchestrator seam responsible for writing the
// hydrate signal byte to every freshly-armed `@portal-skeleton-*` pane's
// FIFO. Without this step, only the user's currently attached session's
// helpers receive their signal via the per-pane client-attached tmux hook —
// helpers in the N-1 non-attached sessions wait their 3s budget and time
// out, leaking @portal-skeleton-* markers and silently degrading scrollback
// save and on-resume hooks.
//
// Each dependency is a small interface so each can be mocked independently
// in tests, mirroring the dependency-shape pattern established by
// MarkerCleanupCore and FIFOSweeper:
//
//   - Markers enumerates the marker map via state.ListSkeletonMarkers; the
//     production adapter is *tmux.Client (satisfies state.ServerOptionLister
//     directly via ShowAllServerOptions).
//   - StateDir is the resolved Portal state directory used to derive each
//     pane's FIFO path via state.FIFOPath(StateDir, paneKey).
//   - Signaler performs the per-FIFO non-blocking write. The production
//     adapter is state.DefaultFIFOSignaler{} (zero value), whose SendSignal
//     delegates to state.SendHydrateSignal — the no-seam production entry
//     point that bundles state.OpenFIFOForSignal + time.Sleep + the bounded
//     retry ladder. Tests inject statetest.RecordingFIFOSignaler.
//   - Logger is a DI-wired seam retained for uniformity with sibling step
//     cores (MarkerCleanupCore, FIFOSweeper). As of the signal-component
//     re-attribution (Phase 5 Task 5-11) the per-FIFO write-failure WARN and
//     success DEBUG breadcrumb route through the package-level signalLogger
//     (component=signal), NOT this field — the FIFO-signaling mechanism is
//     homed under signal per the Subsystem prefix taxonomy. The field is
//     therefore nil-tolerant and currently unread by EagerSignalHydrate; it
//     is preserved (rather than removed) to avoid churning the DI wiring at
//     cmd/bootstrap_production.go and cmd/bootstrap/defaults.go.
//
// Markers and Signaler are mandatory: behaviour with either nil is undefined
// and will panic at first dereference inside EagerSignalHydrate. Only Logger
// is nil-tolerant. The orchestrator's production wiring at
// cmd/bootstrap_production.go and the integration-test builder's auto-default
// branch in cmd/bootstrap/defaults.go both supply non-nil values for both.
type EagerSignalCore struct {
	Markers  state.ServerOptionLister
	StateDir string
	Signaler state.FIFOSignaler
	Logger   *slog.Logger
}

var _ EagerHydrateSignaler = (*EagerSignalCore)(nil)

// EagerSignalHydrate writes the hydrate signal byte to every marker's FIFO.
//
// Algorithm:
//  1. Enumerate marker paneKeys via state.ListSkeletonMarkers(c.Markers).
//     On error, return the wrapped error so the orchestrator's Warn-and-
//     swallow site logs it uniformly with siblings (FIFOSweeper.Sweep,
//     CleanStaleMarkers).
//  2. If zero markers exist, return nil immediately — zero-marker no-op,
//     no FIFO writes attempted.
//  3. For each paneKey, derive fifoPath := state.FIFOPath(c.StateDir,
//     paneKey) and call c.Signaler.SendSignal(fifoPath). On error, emit a WARN
//     via signalLogger ("eager-signal write fifo failed", path/error/
//     error_class=unexpected) under component=signal and continue to the next
//     pane — a single failing FIFO must NEVER abort the loop, otherwise the
//     helpers in the remaining N-1 sessions stay stuck. On success, emit a
//     per-FIFO DEBUG breadcrumb ("fifo signalled", path) under signal.
//  4. Always return nil after the loop. Per-FIFO write failures are
//     soft warnings per spec §Failure Posture; only marker enumeration
//     failures travel up via the return value.
//
// EagerSignalHydrate never returns a *FatalError; every non-nil return is
// soft and the orchestrator (task 1-4) Warn-and-swallows it.
func (c *EagerSignalCore) EagerSignalHydrate() error {
	markers, err := state.ListSkeletonMarkers(c.Markers)
	if err != nil {
		return err
	}
	if len(markers) == 0 {
		return nil
	}

	for paneKey := range markers {
		fifoPath := state.FIFOPath(c.StateDir, paneKey)
		if err := c.Signaler.SendSignal(fifoPath); err != nil {
			// Per-FIFO failures are soft — an un-signalled pane drops a unit
			// of work, so this is a WARN under component=signal with
			// error_class=unexpected (level-discipline table). The wrapped
			// err is passed directly. Log and continue so the remaining
			// markers still get their signal.
			signalLogger.Warn("eager-signal write fifo failed", "path", fifoPath, "error", err, "error_class", "unexpected")
			continue
		}
		// Per-FIFO success breadcrumb under signal — the call-site transition
		// breadcrumb that lets `grep signal:` reconstruct the signaling.
		signalLogger.Debug("fifo signalled", "path", fifoPath)
	}
	return nil
}
