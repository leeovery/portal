package state

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// SignalHydrateRetryDelays is the back-off ladder used when the per-pane FIFO
// is not yet readable. The cumulative budget is 500ms (10+20+40+80+160+190 =
// 500). Spec § "Signal Mechanism: FIFO Per Pane" describes the per-pane
// FIFO contract; signal-hydrate retries O_WRONLY|O_NONBLOCK opens that
// return ENXIO/EAGAIN before giving up. The helper inside the pane will
// eventually reach its O_RDONLY call (spec § "Helper Behavior on Startup")
// and the next attach path will re-signal.
var SignalHydrateRetryDelays = []time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
	160 * time.Millisecond,
	190 * time.Millisecond,
}

// OpenFIFOForSignal is the production OpenFIFO seam. It opens path with
// O_WRONLY|O_NONBLOCK so a missing reader surfaces as ENXIO immediately
// rather than blocking the tmux server (signal-hydrate is invoked via
// run-shell, which is synchronous).
func OpenFIFOForSignal(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
}

// WriteFIFOSignal opens the per-pane FIFO O_WRONLY|O_NONBLOCK and writes a
// single byte. ENXIO (no reader yet) and EAGAIN are retried per
// SignalHydrateRetryDelays; any other error returns immediately. Retry-
// exhaustion is a soft failure (returned as a wrapped error so the caller
// can log) — the marker stays set and the next attach path re-signals.
//
// openFIFO and sleep are explicit seams so callers in different packages
// (cmd/state_signal_hydrate, cmd/bootstrap) can wire their own production
// or test implementations without sharing a config struct. Production
// callers pass OpenFIFOForSignal and time.Sleep.
func WriteFIFOSignal(path string, openFIFO func(string) (*os.File, error), sleep func(time.Duration)) error {
	var lastErr error
	for i := 0; i <= len(SignalHydrateRetryDelays); i++ {
		f, err := openFIFO(path)
		if err == nil {
			if _, werr := f.Write([]byte{1}); werr != nil {
				_ = f.Close()
				return fmt.Errorf("write byte to %s: %w", path, werr)
			}
			_ = f.Close()
			return nil
		}

		if !isRetryableFIFOError(err) {
			return fmt.Errorf("open fifo %s: %w", path, err)
		}

		lastErr = err
		if i < len(SignalHydrateRetryDelays) {
			sleep(SignalHydrateRetryDelays[i])
		}
	}
	return fmt.Errorf("retries exhausted opening fifo %s: %w", path, lastErr)
}

// isRetryableFIFOError reports whether err should trigger the retry ladder.
// Only ENXIO (no reader on a FIFO opened O_WRONLY|O_NONBLOCK) and EAGAIN
// (transient resource shortage) are retryable; everything else — including
// ENOENT (FIFO removed) — surfaces immediately so the caller can log.
func isRetryableFIFOError(err error) bool {
	return errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.EAGAIN)
}

// SendHydrateSignal is the production no-seam entry point that callers in
// cmd/state_signal_hydrate (the run-shell handler) and cmd/bootstrap_production
// (via DefaultFIFOSignaler) use to write the hydrate signal byte to a single
// FIFO. It pins the production seams (OpenFIFOForSignal + time.Sleep) so call
// sites stay closure-free and signature-uniform with sibling primitives.
//
// Test code that needs to substitute its own openFIFO/sleep seams (e.g. for
// retry-ladder coverage) should call WriteFIFOSignal directly — that lower-
// level entry point exposes both seams and remains the layer where the retry
// ladder is exercised.
func SendHydrateSignal(path string) error {
	return WriteFIFOSignal(path, OpenFIFOForSignal, time.Sleep)
}

// FIFOSignaler is the production-shape seam the bootstrap orchestrator's
// EagerSignalCore depends on for per-pane FIFO writes. The single-method
// shape mirrors the rest of the orchestrator's seam vocabulary
// (HookRegistrar.RegisterPortalHooks, FIFOSweeper.Sweep,
// MarkerCleaner.CleanStaleMarkers) so a future step that writes a FIFO byte
// can re-use the same seam without inventing a parallel closure-typed field.
//
// SendSignal must be safe to invoke from a tmux-hook context: the production
// implementation (DefaultFIFOSignaler) inherits WriteFIFOSignal's bounded
// retry ladder (~500ms total budget, see SignalHydrateRetryDelays) and
// non-blocking O_WRONLY|O_NONBLOCK open semantics, so it cannot hang the
// tmux server even when the helper has not yet reached its O_RDONLY call.
type FIFOSignaler interface {
	SendSignal(path string) error
}

// DefaultFIFOSignaler is the production FIFOSignaler. SendSignal delegates to
// SendHydrateSignal so the retry ladder + production seam wiring stay in one
// place. cmd/bootstrap_production.go drops a zero-value DefaultFIFOSignaler{}
// straight into the EagerSignalCore literal — there is no closure adapter
// glue at the wiring site (mirroring MarkerCleanupCore's pattern where
// *tmux.Client satisfies the Markers/Panes/Unsetter seams directly).
type DefaultFIFOSignaler struct{}

// SendSignal delegates to SendHydrateSignal verbatim so DefaultFIFOSignaler
// cannot drift from the no-seam production entry point.
func (DefaultFIFOSignaler) SendSignal(path string) error { return SendHydrateSignal(path) }
