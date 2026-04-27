package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// daemonDeps bundles the inputs the daemon's run loop needs to operate.
// Held in one struct so the test seam (daemonRunFunc) can capture and assert
// against everything the RunE prepared on its behalf.
type daemonDeps struct {
	Dir    string
	Logger *state.Logger
	Client *tmux.Client
}

// daemonRunFunc and daemonShutdownFunc are package-level seams. Tests replace
// them via t.Cleanup so the unit-level tests never block on signals or run
// the production tick loop.
//
// Production callers leave them at the defaults below; later phases own the
// real tick loop and capture logic and may extend defaultDaemonRun.
var (
	daemonRunFunc      = defaultDaemonRun
	daemonShutdownFunc = defaultShutdownFlush
)

// defaultDaemonRun is the production daemon body for Phase 2 task 2-7.
// It blocks until ctx is cancelled (SIGHUP/SIGTERM) and then delegates to the
// shutdown-flush seam. The real per-second tick loop lands in task 2-12.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	<-ctx.Done()
	return daemonShutdownFunc(deps)
}

// defaultShutdownFlush implements the SIGHUP/SIGTERM final-flush handler from
// the spec's Save-Side Architecture → Signal Handling section.
//
// If @portal-restoring is set, capturing now would commit a mid-restore
// snapshot, so we skip the flush and exit. Otherwise we log the flush — the
// actual capture-and-write call lands in a later phase that owns the capture
// pipeline.
func defaultShutdownFlush(deps *daemonDeps) error {
	val, err := deps.Client.GetServerOption("@portal-restoring")
	if err == nil && val != "" {
		deps.Logger.Info("daemon", "skipping final flush: @portal-restoring=%s", val)
		return nil
	}
	deps.Logger.Info("daemon", "final flush")
	return nil
}

// stateDaemonCmd is the long-running save daemon hosted in the
// _portal-saver tmux session. Hidden from --help; invoked internally by tmux
// via "new-session -d -s _portal-saver 'portal state daemon'".
//
// RunE wires up the state directory, log file, PID/version markers, and a
// signal-cancelled context, then delegates the run loop to daemonRunFunc.
// The tick loop and capture logic land in later Phase 2 tasks; this body owns
// the scaffold (paths + lifecycle hooks).
var stateDaemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the Portal save daemon (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			return fmt.Errorf("ensure state dir: %w", err)
		}

		logger, err := state.OpenLogger(state.PortalLog(dir), true)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer func() { _ = logger.Close() }()

		logger.Info("daemon", "starting, version=%s, pid=%d", version, os.Getpid())

		// Defensive dirty-flag clear: a stale save.requested from a crashed or
		// version-mismatch-restarted daemon must not trigger an immediate save
		// during the restore window. See spec § Defensive Dirty-Flag Clear on
		// Daemon Startup.
		_ = os.Remove(state.SaveRequested(dir))

		if err := state.WritePIDFile(dir, os.Getpid()); err != nil {
			return fmt.Errorf("write PID file: %w", err)
		}
		if err := state.WriteVersionFile(dir, version); err != nil {
			return fmt.Errorf("write version file: %w", err)
		}

		client := tmux.NewClient(&tmux.RealCommander{})
		deps := &daemonDeps{Dir: dir, Logger: logger, Client: client}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		return daemonRunFunc(ctx, deps)
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
