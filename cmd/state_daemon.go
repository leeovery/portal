package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

const (
	daemonTick = time.Second
	// pollEvery is how far apart, on the tick, the saver's polls are.
	pollEvery = 5 * time.Second
	// selfSupervisionHysteresisTicks is the run of ticks the saver session
	// must read as gone, or as running another process, before the daemon
	// self-ejects. TestDaemonSelfSupervisionHysteresisAgainstZmx holds it at
	// no less than twice the longest run a healthy daemon reads, within
	// [3, 9].
	selfSupervisionHysteresisTicks = 3
	// Gone-dir projects are inert clutter rather than a correctness hazard,
	// hence the slow cadence.
	projectCleanupInterval = time.Hour
	// flushAsking is how long the final flush may ask sessions for their
	// history, and flushBound how long the shutdown waits for it, naming
	// included: a kill's SIGHUP must still end the daemon promptly.
	flushAsking = 4 * time.Second
	flushBound  = 5 * time.Second
)

// daemonSaver is the saver's cycle as the daemon runs it.
type daemonSaver interface {
	Poll(ctx context.Context) (saver.Result, error)
	Flush(ctx context.Context) (saver.Result, error)
}

type daemonDeps struct {
	Dir     string
	Version string
	// Backend holds the saver session the daemon must be running in.
	Backend backend.Backend
	Saver   daemonSaver
	// Projects is the store the idle cleanup prunes; nil prunes nothing.
	Projects *project.Store
	// Ticks drives the loop, each tick carrying the time it fired.
	Ticks <-chan time.Time
	// FlushAsking and FlushBound bound the final flush, as flushAsking and
	// flushBound do in production.
	FlushAsking, FlushBound time.Duration

	// absences is the run of ticks the saver session has read as lost.
	absences           int
	lastPoll           time.Time
	lastProjectCleanup time.Time
	shutdownSignal     atomic.Pointer[os.Signal]
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

var daemonTickLoopFunc = defaultDaemonTickLoop

var acquireDaemonLock = state.AcquireDaemonLock

// The acquire call, its error guard and WritePIDFile must stay immediately
// consecutive statements: no other work may land between them.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	lockFile, err := acquireDaemonLock(deps.Dir)
	if err != nil {
		if errors.Is(err, state.ErrDaemonLockHeld) {
			daemonLogger.Warn("another daemon holds the lock; exiting")
			return nil
		}
		daemonLogger.Warn("acquire daemon lock failed", "error", err)
		return fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := state.WritePIDFile(deps.Dir, os.Getpid()); err != nil {
		_ = lockFile.Close()
		return fmt.Errorf("write PID file: %w", err)
	}
	if err := state.WriteVersionFile(deps.Dir, deps.Version, daemonLogger); err != nil {
		_ = lockFile.Close()
		return fmt.Errorf("write version file: %w", err)
	}

	daemonLogger.Info("lock acquired", "session", deps.Backend.Current())

	err = daemonTickLoopFunc(ctx, deps)
	// Closed only here, which keeps lockFile reachable for the daemon's whole
	// life: a collected *os.File's finalizer closes its fd, releasing the lock.
	_ = lockFile.Close()
	return err
}

// The self-check runs on every tick, ahead of the poll, which stands down
// while a restore runs: a daemon diverged from the saver must eject during
// one too.
func defaultDaemonTickLoop(ctx context.Context, deps *daemonDeps) error {
	for {
		select {
		case at := <-deps.Ticks:
			if deps.supervise(ctx) {
				// Unreachable past a real exit; keeps a fake one from falling
				// into the tick.
				return nil
			}
			deps.tick(ctx, at)
		case <-ctx.Done():
			return deps.shutdown()
		}
	}
}

// membership is what one tick's self-check read of the saver session.
type membership int

const (
	// membershipHeld is the saver session answering, running this daemon.
	membershipHeld membership = iota
	// membershipLost is the session gone, or running another process.
	membershipLost
	// membershipUnknown is the session not answering, or naming no process,
	// which proves nothing either way.
	membershipUnknown
)

func saverMembership(ctx context.Context, b backend.Backend, self int) (membership, error) {
	s, err := b.Session(ctx, saver.SessionName)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return membershipLost, err
	case err != nil:
		return membershipUnknown, err
	case s.PID == self:
		return membershipHeld, nil
	case s.PID == 0:
		return membershipUnknown, nil
	}
	return membershipLost, nil
}

// supervise checks the daemon is still the saver session's process, and
// self-ejects once a run of selfSupervisionHysteresisTicks ticks has read it
// lost. A tick that reads nothing neither adds to the run nor ends it. It
// reports whether it ejected, which a real exit never returns to.
func (d *daemonDeps) supervise(ctx context.Context) bool {
	m, err := saverMembership(ctx, d.Backend, os.Getpid())
	switch m {
	case membershipHeld:
		d.absences = 0
	case membershipUnknown:
		daemonLogger.Debug("saver-membership probe unanswered", "ticks", d.absences, "threshold", selfSupervisionHysteresisTicks, "error", err)
	case membershipLost:
		d.absences++
		if d.absences >= selfSupervisionHysteresisTicks {
			selfEject(d.absences)
			return true
		}
		daemonLogger.Debug("saver-membership probe failed", "ticks", d.absences, "threshold", selfSupervisionHysteresisTicks)
	}
	return false
}

// selfEject ends a daemon that is no longer the saver session's process
// without its shutdown flush, so a diverged daemon commits nothing more. It
// leaves daemon.pid stale on purpose: the next daemon's lock pre-check reads
// a dead pid as no daemon, and removing it here would race that check.
func selfEject(ticks int) {
	daemonLogger.Info("self-eject", "ticks", ticks, "threshold", selfSupervisionHysteresisTicks)
	// Emits the paired "process: exit code=0" marker without itself exiting.
	log.Close(0)
	osExit(0)
}

// tick polls every pollEvery, the first tick included, and on the ticks
// between runs the idle work.
func (d *daemonDeps) tick(ctx context.Context, at time.Time) {
	if d.lastPoll.IsZero() || at.Sub(d.lastPoll) >= pollEvery {
		d.lastPoll = at
		d.poll(ctx)
		return
	}
	d.maybeCleanProjects(at)
	d.sweepStaleHooks(at)
}

// poll runs one of the saver's polls, summarising one that did anything in
// one INFO line: a quiet poll, every five seconds, logs nothing at INFO.
func (d *daemonDeps) poll(ctx context.Context) {
	start := time.Now()
	result, err := d.Saver.Poll(ctx)
	switch {
	case errors.Is(err, saver.ErrRestoring):
		daemonLogger.Debug("poll stood down", "error", err)
	case err != nil && ctx.Err() != nil:
		// Cut short by the shutdown, whose final flush follows.
	case err != nil:
		daemonLogger.Warn("poll failed", "error", err)
	case !result.Quiet():
		captureLogger.Info("poll complete",
			"entries", result.Claimed,
			"natural_churn", result.Dropped,
			"sessions", result.Captured,
			"reaped", result.Removed,
			log.Took(start),
		)
	}
}

// The throttle anchor is reset after the cleanup runs, so a failing prune
// backs off to the next cadence instead of hammering the store every tick.
func (d *daemonDeps) maybeCleanProjects(at time.Time) {
	if d.Projects == nil || at.Sub(d.lastProjectCleanup) < projectCleanupInterval {
		return
	}
	if _, err := d.Projects.CleanStale(); err != nil {
		daemonLogger.Warn("projects stale-cleanup failed", "error", err)
	}
	d.lastProjectCleanup = at
}

// sweepStaleHooks is where an idle tick reaps the hooks no session and no
// entry names. It reaps none yet: hooks.StaleKeys judges by tmux's pane
// tokens, and would reap the hook of every zmx session whose name is
// token-shaped.
func (d *daemonDeps) sweepStaleHooks(time.Time) {}

// shutdown takes the final flush and logs the daemon's end.
func (d *daemonDeps) shutdown() error {
	start := time.Now()
	result, err := d.flush()
	switch {
	case errors.Is(err, saver.ErrRestoring):
		daemonLogger.Debug("final flush stood down", "error", err)
	case err != nil:
		daemonLogger.Warn("final flush failed", "error", err)
	case !result.Quiet():
		captureLogger.Info("flush complete", "sessions", result.Captured, log.Took(start))
	}
	daemonLogger.Info("shutdown", "reason", d.shutdownReason(), "flush_completed", err == nil)
	return nil
}

// flush runs the saver's flush, asking for histories for up to FlushAsking
// and waiting for it up to FlushBound. A flush past its bound is left to the
// process's exit, which every file it is writing survives whole, each write
// being atomic.
func (d *daemonDeps) flush() (saver.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.FlushAsking)
	defer cancel()
	type flushed struct {
		result saver.Result
		err    error
	}
	done := make(chan flushed, 1)
	go func() {
		result, err := d.Saver.Flush(ctx)
		done <- flushed{result, err}
	}()
	select {
	case f := <-done:
		return f.result, f.err
	case <-time.After(d.FlushBound):
		return saver.Result{}, fmt.Errorf("the final flush ran past its %s bound", d.FlushBound)
	}
}

// cancelOnShutdownSignal cancels the daemon on its first SIGHUP or SIGTERM,
// recording which, until ctx ends; stop undoes the catch.
func (d *daemonDeps) cancelOnShutdownSignal(ctx context.Context, cancel context.CancelFunc) (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-signals:
			// Recorded before the cancel: the shutdown reads it after.
			d.recordShutdownSignal(sig)
			cancel()
		case <-ctx.Done():
		}
	}()
	return func() { signal.Stop(signals) }
}

func newDaemonDeps(dir string, b backend.Backend, ticks <-chan time.Time) *daemonDeps {
	// Neither store failing aborts the daemon's primary job: a nil hook store
	// keeps a dropped session's hook, a nil project store prunes nothing.
	hookStore, err := loadHookStore()
	if err != nil {
		daemonLogger.Warn("load hook store failed; a dropped session's resume hook is kept", "error", err)
	}
	projectStore, err := loadProjectStore()
	if err != nil {
		daemonLogger.Warn("load project store failed; stale-project prune disabled", "error", err)
	}
	return &daemonDeps{
		Dir:         dir,
		Version:     Version(),
		Backend:     b,
		Saver:       saver.New(saver.Config{Backend: b, StateDir: dir, Hooks: hookStore, Now: time.Now}),
		Projects:    projectStore,
		Ticks:       ticks,
		FlushAsking: flushAsking,
		FlushBound:  flushBound,
		// Anchored to the daemon's start, so the prune first runs an
		// interval in.
		lastProjectCleanup: time.Now(),
	}
}

// The daemon runs in the _portal-saver session, which the bootstrap creates.
// It refuses an index this version must not touch before it holds anything.
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
		if err := refuseUnusableIndexIn(dir); err != nil {
			daemonLogger.Warn("refused to start", "path", sessionindex.Path(dir), "error", err)
			return err
		}
		b, err := sessionBackend()
		if err != nil {
			return err
		}
		ticker := time.NewTicker(daemonTick)
		defer ticker.Stop()
		deps := newDaemonDeps(dir, b, ticker.C)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer deps.cancelOnShutdownSignal(ctx, cancel)()

		return defaultDaemonRun(ctx, deps)
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
