// Package saverhost runs the saver's lifecycle: `portal state daemon` in the
// _portal-saver session. It brings the saver up as this binary, replacing one
// running another version, and stops it, one run at a time under saver.lock.
// It signals a process only once it has proved it, as each signal is sent,
// the saver's daemon of this backend's sessions; a proof that fails or can't
// be read sends nothing more.
package saverhost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

var logger = log.For("saver")

const (
	// TermWait is how long a daemon sent SIGTERM has to exit before it is sent
	// SIGKILL: longer than the 6 s from its signal in which the daemon ends
	// its work in flight and its final flush, so a daemon keeping its own
	// bound is never cut short.
	TermWait = 8 * time.Second
	// killWait is how long a daemon sent SIGKILL, or found gone, has to finish
	// exiting: it runs nothing more, and waits only on its reap.
	killWait = 2 * time.Second
	// pollEvery spaces the reads a wait makes.
	pollEvery = 50 * time.Millisecond
)

var (
	// ErrNoProcess is a saver session that answers naming no process.
	ErrNoProcess = errors.New("the saver session names no process")
	// ErrReplaced is a stop of a pid the saver session no longer runs.
	ErrReplaced = errors.New("the saver session runs another process now")
)

// Config is what a Host works over.
type Config struct {
	// Backend holds the saver session.
	Backend backend.Backend
	// StateDir is the state directory the saver's daemon records itself in,
	// the saver session's working directory, and the home of saver.lock.
	StateDir string
	// Marks are the entries, KEY=value, that the environment of every process
	// the backend's saver session runs carries, and that no other process's
	// does: one naming the session, and those saying where it is. On zmx they
	// are zmx.SessionMarks: the session's name, which zmx sets, and the
	// backend's directory, which every zmx run the backend makes sets.
	Marks []string
	// Signal sends sig to pid. Only a pid proved the saver's daemon as it is
	// sent is ever its target.
	Signal func(pid int, sig syscall.Signal) error

	// Identify, Args and Running read processes; nil reads the kernel's.
	// Running reports whether a pid has yet to finish exiting.
	Identify func(pid int) (state.IdentifyResult, error)
	Args     func(pid int) (proctable.Args, error)
	Running  func(pid int) bool

	// LockWait, TermWait, KillWait and ReadyWait bound the wait for saver.lock,
	// for the daemon to exit after each signal, and for a new daemon to come
	// up, and Poll spaces their reads; zero is the default.
	LockWait, TermWait, KillWait, ReadyWait, Poll time.Duration
}

// Host runs the saver's lifecycle over one backend and state directory: its
// stop, an Ensurer's bringing it up, and the proof of which process is its
// daemon.
type Host struct {
	backend   backend.Backend
	stateDir  string
	proof     proof
	signal    func(pid int, sig syscall.Signal) error
	running   func(pid int) bool
	lockWait  time.Duration
	termWait  time.Duration
	killWait  time.Duration
	readyWait time.Duration
	poll      time.Duration
}

// NewHost binds a Host to cfg. It refuses a Config without a backend, a state
// directory, a signal, or marks naming the saver session and where it is.
func NewHost(cfg Config) (*Host, error) {
	switch {
	case cfg.Backend == nil:
		return nil, errors.New("saverhost: no backend")
	case cfg.StateDir == "":
		return nil, errors.New("saverhost: no state directory")
	case cfg.Signal == nil:
		return nil, errors.New("saverhost: no way to signal the daemon")
	}
	if err := checkMarks(cfg.Marks); err != nil {
		return nil, fmt.Errorf("saverhost: %w", err)
	}
	cfg = cfg.withDefaults()
	return &Host{
		backend:   cfg.Backend,
		stateDir:  cfg.StateDir,
		proof:     proof{marks: cfg.Marks, running: cfg.Running, identify: cfg.Identify, args: cfg.Args},
		signal:    cfg.Signal,
		running:   cfg.Running,
		lockWait:  cfg.LockWait,
		termWait:  cfg.TermWait,
		killWait:  cfg.KillWait,
		readyWait: cfg.ReadyWait,
		poll:      cfg.Poll,
	}, nil
}

// withDefaults is c with each reader and bound it leaves unset at its
// default.
func (c Config) withDefaults() Config {
	if c.Identify == nil {
		c.Identify = state.IdentifyDaemon
	}
	if c.Args == nil {
		c.Args = proctable.ReadArgs
	}
	if c.Running == nil {
		c.Running = running
	}
	for _, d := range []struct {
		bound *time.Duration
		def   time.Duration
	}{
		{&c.LockWait, LockWait}, {&c.TermWait, TermWait}, {&c.KillWait, killWait},
		{&c.ReadyWait, ReadyWait}, {&c.Poll, pollEvery},
	} {
		if *d.bound == 0 {
			*d.bound = d.def
		}
	}
	return c
}

// running reports whether pid has yet to finish exiting, as the kernel's
// table says.
func running(pid int) bool { return runningBy(proctable.Exited, pid) }

// runningBy reports whether pid has yet to finish exiting, as exited says. A
// zombie has finished: it holds no descriptor, daemon.lock's included. Where
// exited can't answer, kill(pid, 0) does, counting a zombie running.
func runningBy(exited func(pid int) (bool, error), pid int) bool {
	if done, err := exited(pid); err == nil {
		return !done
	}
	return state.IsProcessAlive(pid)
}

// Present reports whether the saver session is there, whatever it runs. A
// session the backend can tell neither live nor gone is its ErrTimeout.
func (h *Host) Present(ctx context.Context) (bool, error) {
	return h.backend.Exists(ctx, saver.SessionName)
}

// Prove proves pid the saver's daemon of the Host's backend, read as it is
// called. Its error wraps ErrGone, ErrNotSaverDaemon or ErrUnreadable.
func (h *Host) Prove(pid int) error {
	return h.proof.prove(pid)
}

// SaverPID is the process the saver session runs, and whether there is a saver
// session. A session that doesn't answer, or answers naming no process, is an
// error: nothing is known of its process.
func (h *Host) SaverPID(ctx context.Context) (pid int, present bool, err error) {
	session, err := h.backend.Session(ctx, saver.SessionName)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return 0, false, nil
	case err != nil:
		return 0, false, err
	case session.PID <= 0:
		return 0, false, ErrNoProcess
	}
	return session.PID, true, nil
}

// Stop stops the saver, holding saver.lock throughout, and reports whether
// there was one: it reads the saver session's process under the lock and
// stops it as stop does. With no state directory there is nothing to stop
// unless a saver runs without one, for which it creates the directory and
// its lock; it creates nothing when there is no saver.
func (h *Host) Stop(ctx context.Context) (bool, error) {
	return h.StopOr(ctx, nil)
}

// StopOr is Stop that hands a stop failing once it holds saver.lock to
// orElse, still holding it, and reports what orElse does: a saver whose
// daemon the stop can't read, as one speaking a zmx protocol Portal can't
// use, is judged and ended there, so never in place of one an ensure makes
// meanwhile. A nil orElse is Stop.
func (h *Host) StopOr(ctx context.Context, orElse func(ctx context.Context, err error) (bool, error)) (bool, error) {
	release, err := h.lock(ctx)
	if errors.Is(err, fs.ErrNotExist) {
		var present bool
		if present, err = h.Present(ctx); err != nil || !present {
			return false, err
		}
		release, err = h.lockCreating(ctx)
	}
	if err != nil {
		return false, err
	}
	defer release()
	stopped, err := h.stopHeld(ctx)
	if err != nil && orElse != nil {
		return orElse(ctx, err)
	}
	return stopped, err
}

// stopHeld is Stop's run under saver.lock.
func (h *Host) stopHeld(ctx context.Context) (bool, error) {
	pid, present, err := h.SaverPID(ctx)
	if err != nil || !present {
		return false, err
	}
	return true, h.stop(ctx, pid)
}

// stop stops the saver's daemon pid, read as the saver session's process, then
// kills the session. Its caller holds saver.lock. It sends nothing to this
// process, or to a pid the session no longer runs (ErrReplaced). It proves
// pid the daemon and sends it SIGTERM, then waits TermWait for it to exit;
// past that it proves it again and sends SIGKILL. A pid a proof finds gone
// may still be exiting, so that too is waited for. Only once the daemon has
// exited does it kill the session, which zmx ends with its process in any
// case. A proof that fails or can't be read fails the stop, sending nothing
// more and killing no session.
func (h *Host) stop(ctx context.Context, pid int) error {
	if err := h.stillSaver(ctx, pid); err != nil {
		return err
	}
	start := time.Now()
	if err := h.end(ctx, pid); err != nil {
		return err
	}
	logger.Info("daemon exited", "target_pid", pid, log.Took(start))
	return h.killSession(ctx, pid)
}

// stillSaver refuses pid when it is this process, or when the saver session
// runs another. A session gone since pid was read leaves pid to its proof.
func (h *Host) stillSaver(ctx context.Context, pid int) error {
	if pid == os.Getpid() {
		return refused(pid, fmt.Errorf("pid %d is this process", pid))
	}
	running, present, err := h.SaverPID(ctx)
	switch {
	case err != nil:
		return err
	case present && running != pid:
		return refused(pid, fmt.Errorf("%w: pid %d, not %d", ErrReplaced, running, pid))
	}
	return nil
}

// refused logs the stop's refusal of pid, and is err.
func refused(pid int, err error) error {
	logger.Warn("stop refused", "target_pid", pid, "error", err)
	return err
}

// end ends the daemon pid: SIGTERM, then SIGKILL once TermWait has passed,
// each sent only on a proof read as it is sent, and then waits for it to
// finish exiting.
func (h *Host) end(ctx context.Context, pid int) error {
	sent, err := h.signalProved(pid, syscall.SIGTERM,
		fmt.Sprintf("pid %d isn't confirmed as the save daemon, so nothing was sent to it", pid))
	if err != nil {
		return err
	}
	if sent {
		logger.Info("stop started", "target_pid", pid)
		if exited, err := h.awaitExit(ctx, pid, h.termWait); exited || err != nil {
			return err
		}
		sent, err = h.signalProved(pid, syscall.SIGKILL,
			fmt.Sprintf("pid %d, sent SIGTERM, still ran %s later and isn't confirmed as the save daemon any more, so it wasn't sent SIGKILL", pid, h.termWait))
		if err != nil {
			return err
		}
		if sent {
			logger.Info("stop escalated", "target_pid", pid, "reason", "term-timeout")
		}
	}
	return h.AwaitExit(ctx, pid)
}

// signalProved sends sig to pid once proved the saver's daemon, and reports
// whether it sent it: a daemon found gone is sent nothing. A pid it can't
// prove is sent nothing, and fails as refusal says.
func (h *Host) signalProved(pid int, sig syscall.Signal, refusal string) (bool, error) {
	switch err := h.proof.prove(pid); {
	case errors.Is(err, ErrGone):
		return false, nil
	case err != nil:
		return false, refused(pid, fmt.Errorf("%s: %w", refusal, err))
	}
	switch err := h.signal(pid, sig); {
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("couldn't send %s to pid %d: %w", unix.SignalName(sig), pid, err)
	}
	return true, nil
}

// AwaitExit waits up to the kill bound for pid, sent SIGKILL or found gone,
// to finish exiting, and fails if it hasn't: a process found gone may still
// be exiting, holding daemon.lock.
func (h *Host) AwaitExit(ctx context.Context, pid int) error {
	exited, err := h.awaitExit(ctx, pid, h.killWait)
	if err == nil && !exited {
		err = fmt.Errorf("pid %d still hadn't finished exiting %s later", pid, h.killWait)
	}
	return err
}

// awaitExit waits up to bound for pid to finish exiting, and reports whether
// it did. It fails only when ctx ends first.
func (h *Host) awaitExit(ctx context.Context, pid int, bound time.Duration) (bool, error) {
	deadline := time.NewTimer(bound)
	defer deadline.Stop()
	tick := time.NewTicker(h.poll)
	defer tick.Stop()
	for {
		if !h.running(pid) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("wait for pid %d to exit: %w", pid, context.Cause(ctx))
		case <-deadline.C:
			return !h.running(pid), nil
		case <-tick.C:
		}
	}
}

// killSession kills the saver session once its daemon pid has exited,
// leaving one that runs another process. zmx tears the session down as its
// process exits, so a kill that fails while it does, the session gone after,
// has nothing left to do.
func (h *Host) killSession(ctx context.Context, pid int) error {
	session, err := h.backend.Session(ctx, saver.SessionName)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return nil
	case err == nil && session.PID > 0 && session.PID != pid:
		return nil
	}
	err = h.backend.Kill(ctx, saver.SessionName)
	if err == nil || errors.Is(err, backend.ErrNoSuchSession) {
		return nil
	}
	if _, readErr := h.backend.Session(ctx, saver.SessionName); errors.Is(readErr, backend.ErrNoSuchSession) {
		return nil
	}
	return fmt.Errorf("couldn't end the save daemon's session: %w", err)
}
