package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

type daemonDeps struct {
	Dir     string
	Version string
	Logger  *slog.Logger
	Client  *tmux.Client

	HookStore   *hooks.Store
	lastCleanup time.Time

	ProjectStore       *project.Store
	lastProjectCleanup time.Time

	HashMap      state.HashMap
	PrevIndex    *state.Index
	LastSaveAt   time.Time
	TickerPeriod time.Duration
	MaxGap       time.Duration

	shutdownSignal atomic.Pointer[os.Signal]
}

func (d *daemonDeps) recordShutdownSignal(sig os.Signal) {
	d.shutdownSignal.Store(&sig)
}

func (d *daemonDeps) shutdownReason() string {
	p := d.shutdownSignal.Load()
	if p == nil {
		return "exit"
	}
	switch *p {
	case syscall.SIGHUP:
		return "sighup"
	case syscall.SIGTERM:
		return "signal"
	default:
		return "exit"
	}
}

var (
	daemonRunFunc      = defaultDaemonRun
	daemonShutdownFunc = defaultShutdownFlush
)

var daemonTickLoopFunc = defaultDaemonTickLoop

var acquireDaemonLock = state.AcquireDaemonLock

// osExit is the seam over os.Exit; never call os.Exit directly in this package.
// Every call site must first log.Close(N), or the run leaves an unpaired
// "process: start" behind.
var osExit = os.Exit

var saverMembershipProbe = defaultSaverMembershipProbe

func defaultSaverMembershipProbe(c *tmux.Client, selfPID int) bool {
	if !c.HasSession(tmux.PortalSaverName) {
		return false
	}
	pid, present, err := tmux.SaverPanePIDOrAbsent(c, tmux.PortalSaverName)
	if err != nil || !present {
		return false
	}
	return pid == selfPID
}

// daemonLockFile retains the advisory-lock fd for the process lifetime. Never
// close it or attach a runtime.SetFinalizer: either silently releases the
// kernel-side flock, which the kernel would otherwise hold until exit.
var daemonLockFile *os.File

// selfSupervisionHysteresisTicks is the run of failing saver-membership probes
// required before the daemon self-ejects. Measured against real tmux 3.6b on
// 2026-05-23: steady state, attach/detach, client-attached hook fires and
// bootstrap kill-and-recreate all peaked at 0 ticks; a 2x safety factor clamped
// to the [3, 9] range gives 3.
const selfSupervisionHysteresisTicks = 3

// Stale hooks are inert, so the prune runs on a slack cadence that leaves the
// 1s tick free for capture work.
const hookCleanupInterval = 10 * time.Second

// Gone-dir projects are inert clutter rather than a correctness hazard, hence
// the far slower cadence.
const projectCleanupInterval = 1 * time.Hour

// The acquire call, its error guard and WritePIDFile must stay immediately
// consecutive statements: no other work may land between them.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	lockFile, err := acquireDaemonLock(deps.Dir)
	if err != nil {
		if errors.Is(err, state.ErrDaemonLockHeld) {
			deps.Logger.Warn("another daemon holds the lock; exiting")
			return nil
		}
		deps.Logger.Warn("acquire daemon lock failed", "error", err)
		return fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := state.WritePIDFile(deps.Dir, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	if err := state.WriteVersionFile(deps.Dir, deps.Version, deps.Logger); err != nil {
		return fmt.Errorf("write version file: %w", err)
	}
	daemonLockFile = lockFile

	deps.Logger.Info("lock acquired", "tmux_pane", os.Getenv("TMUX_PANE"))

	return daemonTickLoopFunc(ctx, deps)
}

// The membership self-check sits in the loop rather than inside tick: tick
// early-returns while @portal-restoring is set, and a divergent daemon must
// still eject during a restore window.
func defaultDaemonTickLoop(ctx context.Context, deps *daemonDeps) error {
	ticker := time.NewTicker(deps.TickerPeriod)
	defer ticker.Stop()
	var consecutiveAbsenceTicks int
	for {
		select {
		case <-ticker.C:
			if saverMembershipProbe(deps.Client, os.Getpid()) {
				consecutiveAbsenceTicks = 0
			} else {
				consecutiveAbsenceTicks++
				if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks {
					deps.Logger.Info("self-eject", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)
					// Emits the paired "process: exit code=0" marker without
					// itself exiting. Do not reorder or drop it.
					log.Close(0)
					osExit(0)
					// Unreachable in production; keeps a no-op osExit from
					// falling into tick.
					return nil
				}
				deps.Logger.Debug("saver-membership probe failed", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)
			}
			tick(ctx, deps)
		case <-ctx.Done():
			return daemonShutdownFunc(deps)
		}
	}
}

// Order matters: @portal-restoring suppresses the whole tick, including the
// dirty-flag clear, so a save.requested raised during restore survives it. The
// throttled prunes sit on the idle branch — behind the capture branch they
// would never fire on a mostly-idle server.
func tick(ctx context.Context, deps *daemonDeps) {
	restoring, err := state.RestoreWindowActive(state.IsRestoringSet(deps.Client))
	if err != nil {
		deps.Logger.Warn("read @portal-restoring failed", "error", err)
	}
	if restoring {
		return
	}

	dirty := fileExists(state.SaveRequested(deps.Dir))
	gap := time.Since(deps.LastSaveAt) >= deps.MaxGap
	if !dirty && !gap {
		maybeRunHookCleanup(deps)
		maybeRunProjectCleanup(deps)
		return
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		deps.Logger.Warn("tick failed", "error", err)
		return
	}

	deps.LastSaveAt = time.Now()

	if err := os.Remove(state.SaveRequested(deps.Dir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		deps.Logger.Warn("remove save.requested failed", "error", err)
	}
}

// The throttle anchor is reset after the cleanup body runs, so a failing prune
// backs off to the next cadence instead of hammering the store every tick.
func maybeRunHookCleanup(deps *daemonDeps) {
	if deps.HookStore == nil {
		return
	}
	if time.Since(deps.lastCleanup) < hookCleanupInterval {
		return
	}
	// The failure is the sweep's own to report, under the component that owns
	// the subsystem it swept; the tick survives it either way.
	_, _ = hooksweep.Run(deps.Client, deps.HookStore)
	deps.lastCleanup = time.Now()
}

// No mass-deletion hazard guard, unlike the hook prune: a gone directory is
// unambiguously stale regardless of tmux server state.
func maybeRunProjectCleanup(deps *daemonDeps) {
	if deps.ProjectStore == nil {
		return
	}
	if time.Since(deps.lastProjectCleanup) < projectCleanupInterval {
		return
	}
	if _, err := deps.ProjectStore.CleanStale(); err != nil {
		deps.Logger.Warn("projects stale-cleanup failed", "error", err)
	}
	deps.lastProjectCleanup = time.Now()
}

func captureAndCommit(ctx context.Context, deps *daemonDeps) error {
	// A cancellation returns nil, not an error: tick logs WARN on any non-nil
	// return, and a cancel must not produce a log line.
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	start := time.Now()

	skipSet, err := state.ListSkeletonMarkers(deps.Client)
	if err != nil {
		return fmt.Errorf("list markers: %w", err)
	}

	idx, pendingSet, err := state.CaptureAndRefile(deps.Client, deps.Dir, skipSet, deps.PrevIndex, deps.HashMap, deps.Logger)
	if err != nil {
		return fmt.Errorf("capture structure: %w", err)
	}

	sessions := len(idx.Sessions)
	var panes, naturalChurn, anomalous int

	select {
	case <-ctx.Done():
		return nil
	default:
	}

	anyScrollbackChanged := false
	for _, sess := range idx.Sessions {
		for _, win := range sess.Windows {
			for _, pane := range win.Panes {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				paneKey := state.SanitizePaneKey(sess.Name, win.Index, pane.Index)
				if paneSkipsScrollback(paneKey, skipSet, pendingSet) {
					continue
				}
				panes++
				captureLogger.Debug("pane captured", "pane_key", paneKey, "session", sess.Name)
				target := tmux.PaneTargetExact(sess.Name, win.Index, pane.Index)
				data, hash, err := state.CaptureAndHashPane(deps.Client, target)
				if err != nil {
					if isPaneVanishedError(err) {
						naturalChurn++
						captureLogger.Debug("pane vanished", "pane_key", paneKey, "error_class", "expected")
						continue
					}
					anomalous++
					deps.Logger.Warn("capture pane failed", "pane_key", paneKey, "error", err)
					continue
				}
				written, err := state.WriteScrollbackIfChanged(deps.Dir, paneKey, data, hash, deps.HashMap)
				if err != nil {
					anomalous++
					deps.Logger.Warn("write scrollback failed", "pane_key", paneKey, "error", err)
					continue
				}
				if written {
					anyScrollbackChanged = true
				}
			}
		}
	}

	if err := state.Commit(deps.Dir, idx, anyScrollbackChanged, deps.Logger); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	deps.PrevIndex = &idx

	captureLogger.Info("tick complete",
		"sessions", sessions,
		"panes", panes,
		"natural_churn", naturalChurn,
		"anomalous", anomalous,
		log.Took(start),
	)
	return nil
}

// Capturing a pane held behind a waiting resume panel writes back its
// transcript minus the screenful that panel covers, and no copy of those lines
// survives anywhere else.
func paneSkipsScrollback(paneKey string, skipSet, pendingSet map[string]struct{}) bool {
	if _, skipped := skipSet[paneKey]; skipped {
		return true
	}
	_, pending := pendingSet[paneKey]
	return pending
}

// tmux does not sentinel-wrap a vanished pane, so the "can't find " stderr
// phrasing is the load-bearing classifier; ErrNoSuchSession is defensive.
func isPaneVanishedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, tmux.ErrNoSuchSession) {
		return true
	}
	var cmdErr *tmux.CommandError
	if errors.As(err, &cmdErr) && strings.Contains(strings.ToLower(cmdErr.Stderr), "can't find ") {
		return true
	}
	return false
}

// The flush is skipped for the whole restore window: a half-restored topology
// must not become the committed snapshot, and a stale snapshot beats a
// guaranteed-bad one. The failed read the window's rule presumes set is still
// worth a WARN here — a shutdown flush gets no second attempt.
func defaultShutdownFlush(deps *daemonDeps) error {
	restoring, err := state.RestoreWindowActive(state.IsRestoringSet(deps.Client))
	if restoring {
		if err != nil {
			deps.Logger.Warn("read @portal-restoring at shutdown failed; skipping final flush", "error", err)
		} else {
			deps.Logger.Debug("skipping final flush: @portal-restoring set")
		}
		deps.Logger.Info("shutdown", "reason", deps.shutdownReason(), "flush_completed", false)
		return nil
	}
	deps.Logger.Debug("final flush")
	// Non-cancellable: the cancelled context is what triggered this flush.
	flushErr := captureAndCommit(context.Background(), deps)
	if flushErr != nil {
		deps.Logger.Warn("final flush failed", "error", flushErr)
	}
	deps.Logger.Info("shutdown", "reason", deps.shutdownReason(), "flush_completed", flushErr == nil)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// The daemon runs as the pane process of the _portal-saver session, spawned by
// bootstrap.
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

		logger := daemonLogger

		// Defensive dirty-flag clear: a stale save.requested from a crashed or
		// restarted daemon must not trigger a save during the restore window.
		_ = os.Remove(state.SaveRequested(dir))

		hm := state.SeedHashMap(dir, logger)

		// Skeleton-marked panes merge from this pre-boot state during the first
		// capture.
		var prevIdx *state.Index
		if idx, skip, err := state.ReadIndex(dir); !skip {
			prevIdx = &idx
		} else if err != nil {
			logger.Warn("ReadIndex failed", "error", err)
		}

		// A path-resolution failure must not abort the daemon's primary job, so
		// the nil store just disables stale-cleanup for this daemon's lifetime.
		hookStore, err := loadHookStore()
		if err != nil {
			logger.Warn("load hook store failed; hooks stale-cleanup disabled", "error", err)
			hookStore = nil
		}

		projectStore, err := loadProjectStore()
		if err != nil {
			logger.Warn("load project store failed; stale-project prune disabled", "error", err)
			projectStore = nil
		}

		client := tmux.DefaultClient()
		startedAt := time.Now()
		deps := &daemonDeps{
			Dir:     dir,
			Version: version,
			Logger:  logger,
			Client:  client,
			// Anchored to daemon start so each prune first fires one interval
			// in, not on the first idle tick.
			HookStore:          hookStore,
			lastCleanup:        startedAt,
			ProjectStore:       projectStore,
			lastProjectCleanup: startedAt,
			HashMap:            hm,
			PrevIndex:          prevIdx,
			TickerPeriod:       1 * time.Second,
			MaxGap:             30 * time.Second,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
		go func() {
			// Record before cancelling: defaultShutdownFlush reads the signal
			// only after ctx.Done().
			sig := <-sigCh
			deps.recordShutdownSignal(sig)
			cancel()
		}()

		return daemonRunFunc(ctx, deps)
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
