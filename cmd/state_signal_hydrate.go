package cmd

import (
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// signalHydrateConfig groups every dependency runSignalHydrate needs.
// Signaler is the per-FIFO write seam; production wires
// state.DefaultFIFOSignaler{} (whose SendSignal delegates to
// state.SendHydrateSignal — the no-seam production entry point that bundles
// state.OpenFIFOForSignal + time.Sleep + the bounded retry ladder). Tests
// inject recording fakes that satisfy state.FIFOSignaler.
//
// Logger is the signal component's *slog.Logger — NOT hydrate. The
// signal-hydrate command's diagnostics are FIFO-signaling-mechanism lines, so
// they render under component=signal per the Subsystem prefix taxonomy (spec §
// signal row), matching its structural sibling EagerSignalHydrate. This is the
// subsystem (component), orthogonal to the command's process_role, which stays
// `hydrate` (the binary, resolved from argv). When left nil it normalizes to
// the package-level signalLogger via signalLoggerOrDefault.
type signalHydrateConfig struct {
	Session  string
	StateDir string
	Client   *tmux.Client
	Logger   *slog.Logger
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
//
// Its three failure-path WARNs render under component=signal (not hydrate):
// signal-hydrate is the per-session, hook-driven peer of the bootstrap
// EagerSignalHydrate step, doing the identical enumerate-markers-then-write-
// FIFO work, so its diagnostics belong to the FIFO-signaling subsystem for
// grep-completeness and sibling consistency. The component (subsystem) is
// orthogonal to process_role (which stays `hydrate` — the binary).
func runSignalHydrate(cfg signalHydrateConfig) error {
	cfg.Logger = signalLoggerOrDefault(cfg.Logger)
	markers, err := state.ListSkeletonMarkers(cfg.Client)
	if err != nil {
		cfg.Logger.Warn("list skeleton markers failed", "error", err)
		return nil
	}

	panes, err := cfg.Client.ListPanesInSession(cfg.Session)
	if err != nil {
		cfg.Logger.Warn("list panes for session failed", "session", cfg.Session, "error", err)
		return nil
	}

	for _, p := range panes {
		livePaneKey := state.SanitizePaneKey(cfg.Session, p.Window, p.Pane)
		if _, found := markers[livePaneKey]; !found {
			continue
		}
		fifoPath := state.FIFOPath(cfg.StateDir, livePaneKey)
		if err := cfg.Signaler.SendSignal(fifoPath); err != nil {
			cfg.Logger.Warn("write fifo failed", "path", fifoPath, "error", err)
			// Continue — don't touch marker, don't abort other panes.
		}
	}

	return nil
}

// signalLoggerOrDefault returns logger when non-nil, else the package's
// signal-component-bound signalLogger. It mirrors hydrateLoggerOrDefault but
// defaults to the signal component (not hydrate): signal-hydrate's diagnostics
// are FIFO-signaling-mechanism lines per the Subsystem prefix taxonomy. The
// nil-tolerant contract preserves the *slog.Logger nil-receiver safety a caller
// (or test) that leaves Logger nil relies on.
func signalLoggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return signalLogger
	}
	return logger
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

		// Diagnostics (list-markers errors, FIFO write failures) land in the
		// central log via the handler configured once by main -> log.Init.
		// Rotation and the append-only writer discipline are now
		// handler-owned (Phase 2), so this hook-invoked command no longer
		// opens or closes a per-process logger.
		//
		// signalLogger (component=signal), NOT hydrateLogger: signal-hydrate's
		// FIFO-signaling diagnostics are homed under the signal subsystem
		// (Subsystem prefix taxonomy), matching EagerSignalHydrate. The
		// command's process_role stays `hydrate` (argv-resolved binary) —
		// orthogonal to the subsystem component.
		cfg := signalHydrateConfig{
			Session:  sessionName,
			StateDir: dir,
			Client:   tmux.DefaultClient(),
			Logger:   signalLogger,
			Signaler: state.DefaultFIFOSignaler{},
		}
		return signalHydrateRunFunc(cfg)
	},
}

func init() {
	stateCmd.AddCommand(stateSignalHydrateCmd)
}
