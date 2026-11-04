// Package saverhost runs the saver's lifecycle: `portal state daemon` in the
// _portal-saver session. It brings the saver up as this binary, replacing one
// running another version, and stops it. It signals a process only once it has
// proved it, as each signal is sent, the saver's daemon of this backend's
// sessions; a proof that fails or can't be read sends nothing more.
package saverhost

import (
	"context"
	"errors"
	"fmt"
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
	// ErrReplaced is a stop of a pid the saver session no longer runs:
	// another process replaced the saver since the pid was read.
	ErrReplaced = errors.New("the saver session runs another process now")
)

// Config is what a Stopper works over.
type Config struct {
	// Backend holds the saver session.
	Backend backend.Backend
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

	// TermWait and KillWait bound the waits for the daemon to exit after
	// each signal, and Poll spaces their reads; zero is the default.
	TermWait, KillWait, Poll time.Duration
}

// Stopper stops the saver, and proves which process is its daemon.
type Stopper struct {
	backend            backend.Backend
	proof              proof
	signal             func(pid int, sig syscall.Signal) error
	running            func(pid int) bool
	termWait, killWait time.Duration
	poll               time.Duration
}

// NewStopper binds a Stopper to cfg. It refuses a Config without a backend,
// a signal, or marks naming the saver session and where it is.
func NewStopper(cfg Config) (*Stopper, error) {
	switch {
	case cfg.Backend == nil:
		return nil, errors.New("saverhost: no backend")
	case cfg.Signal == nil:
		return nil, errors.New("saverhost: no way to signal the daemon")
	}
	if err := checkMarks(cfg.Marks); err != nil {
		return nil, fmt.Errorf("saverhost: %w", err)
	}
	cfg = cfg.withDefaults()
	return &Stopper{
		backend:  cfg.Backend,
		proof:    proof{marks: cfg.Marks, identify: cfg.Identify, args: cfg.Args},
		signal:   cfg.Signal,
		running:  cfg.Running,
		termWait: cfg.TermWait,
		killWait: cfg.KillWait,
		poll:     cfg.Poll,
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
	if c.TermWait == 0 {
		c.TermWait = TermWait
	}
	if c.KillWait == 0 {
		c.KillWait = killWait
	}
	if c.Poll == 0 {
		c.Poll = pollEvery
	}
	return c
}

// running reports whether pid has yet to finish exiting. A zombie has
// finished: it holds no descriptor, daemon.lock's included. Where the
// kernel's table can't be read, kill(pid, 0) answers, counting a zombie
// running.
func running(pid int) bool {
	if exited, err := proctable.Exited(pid); err == nil {
		return !exited
	}
	return state.IsProcessAlive(pid)
}

// Prove proves pid the saver's daemon of the Stopper's backend, read as it is
// called. Its error wraps ErrGone, ErrNotSaverDaemon or ErrUnreadable.
func (s *Stopper) Prove(pid int) error {
	return s.proof.prove(pid)
}

// SaverPID is the process the saver session runs, and whether there is a saver
// session. A session that doesn't answer, or answers naming no process, is an
// error: nothing is known of its process.
func (s *Stopper) SaverPID(ctx context.Context) (pid int, present bool, err error) {
	return saverPID(ctx, s.backend)
}

func saverPID(ctx context.Context, b backend.Backend) (int, bool, error) {
	session, err := b.Session(ctx, saver.SessionName)
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

// Stop stops the saver's daemon pid, which the caller read as the saver
// session's process, then kills the session. It sends nothing to this
// process, or to a pid the session no longer runs (ErrReplaced). It proves
// pid the daemon and sends it SIGTERM, then waits TermWait for it to exit;
// past that it proves it again and sends SIGKILL. A pid a proof finds gone
// may still be exiting, so that too is waited for. Only once the daemon has
// exited does it kill the session, which zmx ends with its process in any
// case. A proof that fails or can't be read fails the stop, sending nothing
// more and killing no session.
func (s *Stopper) Stop(ctx context.Context, pid int) error {
	if err := s.stillSaver(ctx, pid); err != nil {
		return err
	}
	start := time.Now()
	if err := s.end(ctx, pid); err != nil {
		return err
	}
	logger.Info("daemon exited", "target_pid", pid, log.Took(start))
	return s.killSession(ctx, pid)
}

// stillSaver refuses pid when it is this process, or when the saver session
// runs another. A session gone since pid was read leaves pid to its proof.
func (s *Stopper) stillSaver(ctx context.Context, pid int) error {
	if pid == os.Getpid() {
		return fmt.Errorf("pid %d is this process", pid)
	}
	running, present, err := s.SaverPID(ctx)
	switch {
	case err != nil:
		return err
	case present && running != pid:
		return fmt.Errorf("%w: pid %d, not %d", ErrReplaced, running, pid)
	}
	return nil
}

// end ends the daemon pid: SIGTERM, then SIGKILL once TermWait has passed,
// each sent only on a proof read as it is sent, and then waits for it to
// finish exiting.
func (s *Stopper) end(ctx context.Context, pid int) error {
	sent, err := s.signalProved(pid, syscall.SIGTERM,
		fmt.Sprintf("pid %d isn't confirmed as the save daemon, so nothing was sent to it", pid))
	if err != nil {
		return err
	}
	if sent {
		logger.Info("stop started", "target_pid", pid)
		if exited, err := s.awaitExit(ctx, pid, s.termWait); exited || err != nil {
			return err
		}
		sent, err = s.signalProved(pid, syscall.SIGKILL,
			fmt.Sprintf("pid %d, sent SIGTERM, still ran %s later and isn't confirmed as the save daemon any more, so it wasn't sent SIGKILL", pid, s.termWait))
		if err != nil {
			return err
		}
		if sent {
			logger.Info("stop escalated", "target_pid", pid, "reason", "term-timeout")
		}
	}
	return s.AwaitExit(ctx, pid)
}

// signalProved sends sig to pid once proved the saver's daemon, and reports
// whether it sent it: a daemon found gone is sent nothing. A pid it can't
// prove is sent nothing, and fails as refused says.
func (s *Stopper) signalProved(pid int, sig syscall.Signal, refused string) (bool, error) {
	switch err := s.proof.prove(pid); {
	case errors.Is(err, ErrGone):
		return false, nil
	case err != nil:
		logger.Warn("stop refused", "target_pid", pid, "error", err)
		return false, fmt.Errorf("%s: %w", refused, err)
	}
	switch err := s.signal(pid, sig); {
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
func (s *Stopper) AwaitExit(ctx context.Context, pid int) error {
	exited, err := s.awaitExit(ctx, pid, s.killWait)
	if err == nil && !exited {
		err = fmt.Errorf("pid %d still hadn't finished exiting %s later", pid, s.killWait)
	}
	return err
}

// awaitExit waits up to bound for pid to finish exiting, and reports whether
// it did. It fails only when ctx ends first.
func (s *Stopper) awaitExit(ctx context.Context, pid int, bound time.Duration) (bool, error) {
	deadline := time.NewTimer(bound)
	defer deadline.Stop()
	tick := time.NewTicker(s.poll)
	defer tick.Stop()
	for {
		if !s.running(pid) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("wait for pid %d to exit: %w", pid, context.Cause(ctx))
		case <-deadline.C:
			return !s.running(pid), nil
		case <-tick.C:
		}
	}
}

// killSession kills the saver session once its daemon pid has exited,
// leaving one that runs another process, made since. zmx tears the session
// down as its process exits, so a kill that fails while it does, the session
// gone after, has nothing left to do.
func (s *Stopper) killSession(ctx context.Context, pid int) error {
	session, err := s.backend.Session(ctx, saver.SessionName)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return nil
	case err == nil && session.PID > 0 && session.PID != pid:
		return nil
	}
	err = s.backend.Kill(ctx, saver.SessionName)
	if err == nil || errors.Is(err, backend.ErrNoSuchSession) {
		return nil
	}
	if _, readErr := s.backend.Session(ctx, saver.SessionName); errors.Is(readErr, backend.ErrNoSuchSession) {
		return nil
	}
	return fmt.Errorf("couldn't end the save daemon's session: %w", err)
}
