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
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
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
	// A hook goes with its session's entry; the sweep is only the backstop
	// for one left behind, so it shares the projects' cadence.
	hookSweepInterval = time.Hour
	// shutdownBound is how long, from the first SIGTERM or SIGHUP, the work
	// in flight and the final flush have before the daemon exits without
	// them: a poll held on commit.lock must not keep a successor meeting a
	// live daemon.pid. A zmx kill grants far less, sending its uncatchable
	// signal about 0.5 s after the SIGHUP, so Portal's own stops
	// SIGTERM the daemon and wait saverhost.TermWait, longer than this, for
	// it to exit, rather than rely on a kill of _portal-saver.
	shutdownBound = 6 * time.Second
	// flushNaming is the end of the bound the final flush keeps for naming
	// what it wrote: it stops asking for histories that long before.
	flushNaming = time.Second
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
	// Hooks is the store the idle sweep prunes; nil prunes nothing.
	Hooks *hooks.Store
	// Ticks drives the loop, each tick carrying the time it fired.
	Ticks <-chan time.Time
	// Signals carries the SIGTERMs and SIGHUPs that end the daemon.
	Signals <-chan os.Signal
	// ShutdownBound and FlushNaming bound the shutdown, as shutdownBound and
	// flushNaming do in production.
	ShutdownBound, FlushNaming time.Duration

	// absences is the run of ticks the saver session has read as lost.
	absences           int
	lastPoll           time.Time
	lastProjectCleanup time.Time
	lastHookSweep      time.Time
	// signalled is the signal that began the shutdown, if one did.
	signalled atomic.Pointer[shutdownSignal]
	// ended is set by whichever logs the shutdown first: the loop, its flush
	// done, or runLoop, its bound passed.
	ended atomic.Bool
}

// shutdownSignal is the first SIGTERM or SIGHUP and when it came.
type shutdownSignal struct {
	sig os.Signal
	at  time.Time
}

func (d *daemonDeps) recordShutdownSignal(sig os.Signal, at time.Time) {
	d.signalled.Store(&shutdownSignal{sig: sig, at: at})
}

func (d *daemonDeps) shutdownReason() string {
	s := d.signalled.Load()
	if s == nil {
		return "exit"
	}
	switch s.sig {
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

// heldDaemonLock is the daemon's lock from its acquire until the process
// exits, which releases it. It is never closed, and is held here to stay
// reachable: a collected *os.File's finalizer would close its fd, releasing
// the lock while daemon.pid still names this live process.
var heldDaemonLock *os.File

// The acquire call, its error guard and WritePIDFile must stay immediately
// consecutive statements: no other work may land between them.
func defaultDaemonRun(ctx context.Context, deps *daemonDeps) error {
	var err error
	heldDaemonLock, err = acquireDaemonLock(deps.Dir)
	if err != nil {
		if errors.Is(err, state.ErrDaemonLockHeld) {
			daemonLogger.Warn("another daemon holds the lock; exiting")
			return nil
		}
		daemonLogger.Warn("acquire daemon lock failed", "error", err)
		return fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := state.WritePIDFile(deps.Dir, os.Getpid()); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	if err := state.WriteVersionFile(deps.Dir, deps.Version, daemonLogger); err != nil {
		return fmt.Errorf("write version file: %w", err)
	}

	daemonLogger.Info("lock acquired", "session", deps.Backend.Current())

	return deps.runLoop(ctx, daemonTickLoopFunc)
}

// runLoop runs loop until it returns, or until a SIGTERM or SIGHUP ends the
// daemon. The first cancels the loop, whose work in flight and final flush
// then have ShutdownBound; past it, or at a second signal, the daemon ends
// without them, which their atomic writes and the lock's release at exit
// make safe.
func (d *daemonDeps) runLoop(ctx context.Context, loop func(context.Context, *daemonDeps) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- loop(ctx, d) }()

	select {
	case err := <-done:
		return err
	case sig := <-d.Signals:
		d.recordShutdownSignal(sig, time.Now())
		cancel()
	}
	bound := time.NewTimer(d.ShutdownBound)
	defer bound.Stop()
	select {
	case err := <-done:
		return err
	case <-bound.C:
	case <-d.Signals:
	}
	d.logShutdown(false)
	return nil
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
	d.sweepStaleHooks(ctx, at)
}

// poll runs one of the saver's polls, summarising in one INFO line one that
// claimed, dropped, captured or removed anything: a quiet poll, every five
// seconds, logs nothing at INFO.
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
			"claimed", result.Claimed,
			"dropped", result.Dropped,
			"captured", result.Captured,
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

// sweepStaleHooks reaps, on an idle tick an interval after the last sweep, the
// hooks no listed session and no index entry names. The sweep logs its own
// outcome, a stand-down at DEBUG and a failure at WARN, so its error is not
// logged again here. The anchor moves whatever the outcome, so a failing sweep
// waits an interval like any other.
func (d *daemonDeps) sweepStaleHooks(ctx context.Context, at time.Time) {
	if d.Hooks == nil || at.Sub(d.lastHookSweep) < hookSweepInterval {
		return
	}
	_, _ = hooksweep.Run(ctx, hooksweep.Sources{List: d.Backend.List, StateDir: d.Dir}, d.Hooks)
	d.lastHookSweep = at
}

// shutdown takes the final flush and logs the daemon's end. The flush asks
// for histories until FlushNaming before the shutdown's bound, keeping the
// rest for naming what it wrote.
func (d *daemonDeps) shutdown() error {
	start := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), d.shutdownDeadline(start).Add(-d.FlushNaming))
	defer cancel()
	result, err := d.Saver.Flush(ctx)
	switch {
	case errors.Is(err, saver.ErrRestoring):
		daemonLogger.Debug("final flush stood down", "error", err)
	case err != nil:
		daemonLogger.Warn("final flush failed", "error", err)
	case !result.Quiet():
		captureLogger.Info("flush complete", "captured", result.Captured, log.Took(start))
	}
	d.logShutdown(err == nil)
	return nil
}

// shutdownDeadline is when the shutdown's bound passes: ShutdownBound after
// the signal that began it, or after from for a shutdown no signal began.
func (d *daemonDeps) shutdownDeadline(from time.Time) time.Time {
	if s := d.signalled.Load(); s != nil {
		from = s.at
	}
	return from.Add(d.ShutdownBound)
}

// logShutdown logs the daemon's end, once, whoever reaches it first.
func (d *daemonDeps) logShutdown(flushCompleted bool) {
	if d.ended.CompareAndSwap(false, true) {
		daemonLogger.Info("shutdown", "reason", d.shutdownReason(), "flush_completed", flushCompleted)
	}
}

func newDaemonDeps(dir string, b backend.Backend, ticks <-chan time.Time, signals <-chan os.Signal) *daemonDeps {
	// Neither store failing aborts the daemon's primary job: a nil hook store
	// keeps a dropped session's hook and sweeps nothing, a nil project store
	// prunes nothing.
	hookStore, err := loadHookStore()
	if err != nil {
		daemonLogger.Warn("load hook store failed; a dropped session's resume hook is kept", "error", err)
	}
	projectStore, err := loadProjectStore()
	if err != nil {
		daemonLogger.Warn("load project store failed; stale-project prune disabled", "error", err)
	}
	return &daemonDeps{
		Dir:           dir,
		Version:       Version(),
		Backend:       b,
		Saver:         saver.New(saver.Config{Backend: b, StateDir: dir, Hooks: hookStore, Now: time.Now}),
		Projects:      projectStore,
		Hooks:         hookStore,
		Ticks:         ticks,
		Signals:       signals,
		ShutdownBound: shutdownBound,
		FlushNaming:   flushNaming,
		// Anchored to the daemon's start, so each prune first runs an
		// interval in.
		lastProjectCleanup: time.Now(),
		lastHookSweep:      time.Now(),
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
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM)
		defer signal.Stop(signals)
		ticker := time.NewTicker(daemonTick)
		defer ticker.Stop()

		return defaultDaemonRun(context.Background(), newDaemonDeps(dir, b, ticker.C, signals))
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
