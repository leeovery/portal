package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"syscall"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

type signalHydrateConfig struct {
	Session  string
	StateDir string
	Client   *tmux.Client
	Logger   *slog.Logger
	Signaler state.FIFOSignaler

	// Repaint asks the process group a waiting pane's process leads to redraw
	// the resume panel.
	Repaint func(pid int) error
}

// Fired from a tmux hook: every failure path is soft so it can never block the
// server or fail the user's attach. Hydration and the repaint are independent,
// so a read one of them could not make leaves the other to run.
func runSignalHydrate(cfg signalHydrateConfig) error {
	cfg.Logger = signalLoggerOrDefault(cfg.Logger)
	signalSkeletonPanes(cfg)
	repaintWaitingPanes(cfg)
	return nil
}

// signalSkeletonPanes wakes the hydrate helper of every pane of the session
// still waiting for its scrollback. The skeleton marker is deliberately left
// set — the hydrate helper unsets it, closing the capture-mid-dump race.
func signalSkeletonPanes(cfg signalHydrateConfig) {
	markers, err := state.ListSkeletonMarkers(cfg.Client)
	if err != nil {
		cfg.Logger.Warn("list skeleton markers failed", "error", err)
		return
	}

	panes, err := cfg.Client.ListPanesInSession(cfg.Session)
	if err != nil {
		cfg.Logger.Warn("list panes for session failed", "session", cfg.Session, "error", err)
		return
	}

	for _, p := range panes {
		livePaneKey := state.SanitizePaneKey(cfg.Session, p.Window, p.Pane)
		if _, found := markers[livePaneKey]; !found {
			continue
		}
		fifoPath := state.FIFOPath(cfg.StateDir, livePaneKey)
		if err := cfg.Signaler.SendSignal(fifoPath); err != nil {
			cfg.Logger.Warn("write fifo failed", "path", fifoPath, "error", err)
		}
	}
}

// repaintWaitingPanes asks every pane of the session that is waiting on its
// resume panel to draw it again. A panel is painted when its pane is restored
// and holds that paint for as long as it waits, so without this a client
// arriving days later meets the theme, and the light/dark half, that were in
// force at restore.
func repaintWaitingPanes(cfg signalHydrateConfig) {
	panes, err := cfg.Client.ListWaitingPanesInSession(cfg.Session)
	if err != nil {
		cfg.Logger.Warn("list waiting panes failed", "session", cfg.Session, "error", err)
		return
	}
	for _, p := range panes {
		if err := cfg.Repaint(p.PID); err != nil {
			cfg.Logger.Warn("repaint waiting pane failed", "session", cfg.Session, "tmux_pane", p.ID, "target_pid", p.PID, "error", err)
		}
	}
}

// repaintPaneGroup sends SIGWINCH to the process group led by a waiting pane's
// own process — the parked chain shell, with the waiter its child in the same
// group — which is where the pane's tty delivers the signal on a resize. The
// other processes the group can hold leave SIGWINCH at its default and ignore
// it, and a waiter from an older binary reads it as a resize that changed
// nothing, so it is harmless to all of them. A group that has gone since the
// listing is not a failure.
func repaintPaneGroup(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to signal process group %d", pid)
	}
	if err := syscall.Kill(-pid, syscall.SIGWINCH); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal process group %d: %w", pid, err)
	}
	return nil
}

func signalLoggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return signalLogger
	}
	return logger
}

var signalHydrateRunFunc = runSignalHydrate

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

		cfg := signalHydrateConfig{
			Session:  sessionName,
			StateDir: dir,
			Client:   tmux.DefaultClient(),
			Logger:   signalLogger,
			Signaler: state.DefaultFIFOSignaler{},
			Repaint:  repaintPaneGroup,
		}
		return signalHydrateRunFunc(cfg)
	},
}

func init() {
	stateCmd.AddCommand(stateSignalHydrateCmd)
}
