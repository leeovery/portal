package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// signalHydrateConfig groups every dependency runSignalHydrate needs.
// Signaler is the per-FIFO write seam; production wires
// state.DefaultFIFOSignaler{} (whose SendSignal delegates to
// state.SendHydrateSignal — the no-seam production entry point that bundles
// state.OpenFIFOForSignal + time.Sleep + the bounded retry ladder). Tests
// inject recording fakes that satisfy state.FIFOSignaler. Logger is optional
// (a nil *state.Logger is a valid no-op).
type signalHydrateConfig struct {
	Session  string
	StateDir string
	Client   *tmux.Client
	Logger   *state.Logger
	Signaler state.FIFOSignaler
}

// runSignalHydrate enumerates panes in the named session and signals each
// skeleton-marked pane's FIFO via Signaler.SendSignal. Every failure path is
// soft (logs at WARN, returns nil) — signal-hydrate is fired from a tmux
// hook and must never block the tmux server or fail the user-visible attach.
// The skeleton marker is intentionally not touched here; the hydrate helper
// owns marker-unset to close the capture-mid-dump race (spec → "The 100ms
// Settle Sleep").
//
// The FIFO write primitive itself (open + retry ladder) lives in
// internal/state behind state.SendHydrateSignal / DefaultFIFOSignaler so the
// retry coverage stays in one place rather than duplicated at the cmd
// layer. The retry ladder is exhaustively tested in
// internal/state/signal_hydrate_test.go.
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
		if err := cfg.Signaler.SendSignal(fifoPath); err != nil {
			cfg.Logger.Warn(state.ComponentHydrate, "write fifo %s: %v", fifoPath, err)
			// Continue — don't touch marker, don't abort other panes.
		}
	}

	return nil
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
		defer func() { _ = logger.Close() }()

		cfg := signalHydrateConfig{
			Session:  sessionName,
			StateDir: dir,
			Client:   tmux.DefaultClient(),
			Logger:   logger,
			Signaler: state.DefaultFIFOSignaler{},
		}
		return signalHydrateRunFunc(cfg)
	},
}

func init() {
	stateCmd.AddCommand(stateSignalHydrateCmd)
}
