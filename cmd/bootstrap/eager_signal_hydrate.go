package bootstrap

import (
	"github.com/leeovery/portal/internal/state"
)

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
//   - Logger is optional. When non-nil, per-FIFO write failures are emitted
//     via Logger.Warn under ComponentHydrate. A nil Logger is tolerated —
//     EagerSignalHydrate substitutes a no-op default at entry so call sites
//     can dispatch unconditionally, mirroring MarkerCleanupCore's contract.
type EagerSignalCore struct {
	Markers  state.ServerOptionLister
	StateDir string
	Signaler state.FIFOSignaler
	Logger   Logger
}

var _ EagerHydrateSignaler = (*EagerSignalCore)(nil)

// EagerSignalHydrate writes the hydrate signal byte to every marker's FIFO.
//
// Algorithm:
//  1. Substitute a local no-op Logger when none was injected so call sites
//     can invoke logger.Warn unconditionally (mirrors MarkerCleanupCore).
//  2. Enumerate marker paneKeys via state.ListSkeletonMarkers(c.Markers).
//     On error, return the wrapped error so the orchestrator's Warn-and-
//     swallow site logs it uniformly with siblings (FIFOSweeper.Sweep,
//     CleanStaleMarkers).
//  3. If zero markers exist, return nil immediately — zero-marker no-op,
//     no FIFO writes attempted.
//  4. For each paneKey, derive fifoPath := state.FIFOPath(c.StateDir,
//     paneKey) and call c.Signaler.SendSignal(fifoPath). On error, log via
//     logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s:
//     %v", fifoPath, err) and continue to the next pane — a single failing
//     FIFO must NEVER abort the loop, otherwise the helpers in the
//     remaining N-1 sessions stay stuck.
//  5. Always return nil after the loop. Per-FIFO write failures are
//     soft warnings per spec §Failure Posture; only marker enumeration
//     failures travel up via the return value.
//
// EagerSignalHydrate never returns a *FatalError; every non-nil return is
// soft and the orchestrator (task 1-4) Warn-and-swallows it.
func (c *EagerSignalCore) EagerSignalHydrate() error {
	// Substitute a local no-op Logger when none was injected so call sites
	// can invoke logger.Warn unconditionally. Use a local var rather than
	// mutating c.Logger so the receiver's state is not silently rewritten
	// across calls.
	logger := c.Logger
	if logger == nil {
		logger = noopLogger{}
	}

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
			// Per-FIFO failures are soft — log and continue so the
			// remaining markers still get their signal. The loop body's
			// last statement is the warn call, so the implicit fallthrough
			// to the next iteration is the continuation.
			logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)
		}
	}
	return nil
}
