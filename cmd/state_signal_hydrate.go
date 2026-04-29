package cmd

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// signalHydrateRetryDelays is the back-off ladder used when the per-pane FIFO
// is not yet readable. The cumulative budget is 500ms (10+20+40+80+160+190 =
// 500). Spec → "FIFO open-for-write semantics": signal-hydrate retries
// O_WRONLY|O_NONBLOCK opens that return ENXIO/EAGAIN before giving up; the
// helper inside the pane will eventually reach its O_RDONLY call and the next
// attach path will re-signal.
var signalHydrateRetryDelays = []time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
	160 * time.Millisecond,
	190 * time.Millisecond,
}

// signalHydrateConfig groups every dependency runSignalHydrate needs. OpenFIFO
// and Sleep are test seams; production wires them to openFIFOForSignal and
// time.Sleep. Logger is optional (a nil *state.Logger is a valid no-op).
type signalHydrateConfig struct {
	Session  string
	StateDir string
	Client   *tmux.Client
	Logger   *state.Logger
	OpenFIFO func(path string) (*os.File, error)
	Sleep    func(d time.Duration)
}

// openFIFOForSignal is the production OpenFIFO seam. It opens path with
// O_WRONLY|O_NONBLOCK so a missing reader surfaces as ENXIO immediately
// rather than blocking the tmux server (signal-hydrate is invoked via
// run-shell, which is synchronous).
func openFIFOForSignal(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
}

// runSignalHydrate enumerates panes in the named session, opens each
// skeleton-marked pane's FIFO with retries, and writes a single byte. Every
// failure path is soft (logs at WARN, returns nil) — signal-hydrate is fired
// from a tmux hook and must never block the tmux server or fail the user-
// visible attach. The skeleton marker is intentionally not touched here; the
// hydrate helper owns marker-unset to close the capture-mid-dump race (spec
// → "The 100ms Settle Sleep").
func runSignalHydrate(cfg signalHydrateConfig) error {
	markers, err := state.ListSkeletonMarkers(cfg.Client)
	if err != nil {
		cfg.Logger.Warn(state.ComponentHydrate, "list skeleton markers: %v", err)
		return nil
	}

	panes, err := cfg.Client.ListPanesInSession(cfg.Session)
	if err != nil {
		cfg.Logger.Warn(state.ComponentHydrate, "list panes for %q: %v", cfg.Session, err)
		return nil
	}

	for _, p := range panes {
		livePaneKey := state.SanitizePaneKey(cfg.Session, p.Window, p.Pane)
		if _, found := markers[livePaneKey]; !found {
			continue
		}
		fifoPath := state.FIFOPath(cfg.StateDir, livePaneKey)
		if err := writeFIFOSignal(fifoPath, cfg); err != nil {
			cfg.Logger.Warn(state.ComponentHydrate, "write fifo %s: %v", fifoPath, err)
			// Continue — don't touch marker, don't abort other panes.
		}
	}

	return nil
}

// writeFIFOSignal opens the per-pane FIFO O_WRONLY|O_NONBLOCK and writes a
// single byte. ENXIO (no reader yet) and EAGAIN are retried per
// signalHydrateRetryDelays; any other error returns immediately. Retry-
// exhaustion is a soft failure (returned as a wrapped error so the caller
// can log) — the marker stays set and the next attach path re-signals.
func writeFIFOSignal(path string, cfg signalHydrateConfig) error {
	var lastErr error
	for i := 0; i <= len(signalHydrateRetryDelays); i++ {
		f, err := cfg.OpenFIFO(path)
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
		if i < len(signalHydrateRetryDelays) {
			cfg.Sleep(signalHydrateRetryDelays[i])
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

// signalHydrateRunFunc is the package-level seam tests use to short-circuit
// the body for argv-only assertions (see TestStateInternalSubcommandsAcceptValidArgv
// in cmd/state_test.go). Production points it at runSignalHydrate.
var signalHydrateRunFunc = runSignalHydrate

// stateSignalHydrateCmd is invoked by client-attached / client-session-changed
// hooks. It enumerates panes in the named session and signals any with a
// pending skeleton marker via their FIFO. Hidden from --help.
var stateSignalHydrateCmd = &cobra.Command{
	Use:    "signal-hydrate <session-name>",
	Short:  "Signal hydrate helpers for the named session (internal)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		dir, err := state.EnsureDir()
		if err != nil {
			return fmt.Errorf("ensure state dir: %w", err)
		}

		// Open portal.log via the non-daemon append-only path so signal-
		// hydrate failures (list-markers errors, FIFO write failures) land
		// in the central log. Per spec § Log Rotation → Concurrent-writer
		// discipline, only the daemon rotates; this hook-invoked writer
		// must not. On open failure logger is nil and the *state.Logger
		// nil-receiver no-ops every call.
		logger, _ := openNoRotateLogger()

		cfg := signalHydrateConfig{
			Session:  sessionName,
			StateDir: dir,
			Client:   tmux.DefaultClient(),
			Logger:   logger,
			OpenFIFO: openFIFOForSignal,
			Sleep:    time.Sleep,
		}
		return signalHydrateRunFunc(cfg)
	},
}

func init() {
	stateCmd.AddCommand(stateSignalHydrateCmd)
}
