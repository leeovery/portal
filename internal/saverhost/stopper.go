// Package saverhost runs the saver's lifecycle: `portal state daemon` in the
// _portal-saver session. It brings the saver up as this binary, replacing one
// running another version, and stops it. It signals a process only once it has
// proved it, as each signal is sent, the saver's daemon of this backend's
// sessions; a proof that fails or can't be read sends nothing.
package saverhost

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
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
	// killWait is how long a daemon sent SIGKILL has to be gone: it runs
	// nothing more, and waits only on the session's reap.
	killWait = 2 * time.Second
	// pollEvery spaces the reads a wait makes.
	pollEvery = 50 * time.Millisecond
)

// ErrNoProcess is a saver session that answers naming no process.
var ErrNoProcess = errors.New("the saver session names no process")

// Config is what a Stopper works over.
type Config struct {
	// Backend holds the saver session.
	Backend backend.Backend
	// Marks are the entries, KEY=value, that the environment of every process
	// the backend's saver session runs carries, and that no other process's
	// does: on zmx the session's name, which zmx sets, and its directory,
	// which every zmx run the backend makes sets.
	Marks []string
	// Signal sends sig to pid. Only a pid proved the saver's daemon as it is
	// sent is ever its target.
	Signal func(pid int, sig syscall.Signal) error

	// Identify, Environ and Alive read processes; nil reads the kernel's.
	Identify func(pid int) (state.IdentifyResult, error)
	Environ  func(pid int) ([]string, error)
	Alive    func(pid int) bool

	// TermWait and KillWait bound the waits for the daemon to exit after
	// each signal, and Poll spaces their reads; zero is the default.
	TermWait, KillWait, Poll time.Duration
}

// Stopper stops the saver, and proves which process is its daemon.
type Stopper struct {
	backend            backend.Backend
	proof              proof
	signal             func(pid int, sig syscall.Signal) error
	alive              func(pid int) bool
	termWait, killWait time.Duration
	poll               time.Duration
}

// NewStopper binds a Stopper to cfg. It refuses a Config without a backend,
// a signal or marks to prove the daemon by.
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
		proof:    proof{marks: cfg.Marks, identify: cfg.Identify, environ: cfg.Environ},
		signal:   cfg.Signal,
		alive:    cfg.Alive,
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
	if c.Environ == nil {
		c.Environ = kernelEnviron
	}
	if c.Alive == nil {
		c.Alive = state.IsProcessAlive
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

// Prove proves pid the saver's daemon of the Stopper's backend, read as it is
// called. Its error wraps ErrGone, ErrNotSaverDaemon or ErrUnreadable.
func (s *Stopper) Prove(pid int) error {
	return s.proof.prove(pid)
}

// SaverPID is the process the saver session runs, and whether there is a saver
// session. A session that doesn't answer, or answers naming no process, is an
// error: nothing is known of its process.
func (s *Stopper) SaverPID(ctx context.Context) (pid int, present bool, err error) {
	session, err := s.backend.Session(ctx, saver.SessionName)
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

// Stop stops the saver and reports whether there was one; none is no failure.
// It proves the saver session's process the daemon and sends it SIGTERM, then
// waits TermWait for it to exit; past that it proves it again and sends
// SIGKILL. Only once the daemon has exited does it kill the session, which
// zmx ends with its process in any case. A proof that fails or can't be read
// fails the stop, having sent nothing more and killed no session.
func (s *Stopper) Stop(ctx context.Context) (bool, error) {
	pid, present, err := s.SaverPID(ctx)
	if err != nil || !present {
		return false, err
	}
	start := time.Now()
	if err := s.end(ctx, pid); err != nil {
		return false, err
	}
	logger.Info("daemon exited", "target_pid", pid, log.Took(start))
	if err := s.backend.Kill(ctx, saver.SessionName); err != nil && !errors.Is(err, backend.ErrNoSuchSession) {
		return false, fmt.Errorf("kill the saver session: %w", err)
	}
	return true, nil
}

// end ends the daemon pid: SIGTERM, then SIGKILL once TermWait has passed,
// each sent only on a proof read as it is sent.
func (s *Stopper) end(ctx context.Context, pid int) error {
	if gone, err := s.signalProved(pid, syscall.SIGTERM); gone || err != nil {
		return err
	}
	logger.Info("stop started", "target_pid", pid)
	if exited, err := s.awaitExit(ctx, pid, s.termWait); exited || err != nil {
		return err
	}
	if gone, err := s.signalProved(pid, syscall.SIGKILL); gone || err != nil {
		return err
	}
	logger.Info("stop escalated", "target_pid", pid, "reason", "term-timeout")
	exited, err := s.awaitExit(ctx, pid, s.killWait)
	if err == nil && !exited {
		err = fmt.Errorf("the saver's daemon %d still ran %s after SIGKILL", pid, s.killWait)
	}
	return err
}

// signalProved sends sig to pid once proved the saver's daemon. It reports a
// daemon already gone, which is sent nothing, and fails for a pid it can't
// prove, sending nothing.
func (s *Stopper) signalProved(pid int, sig syscall.Signal) (gone bool, err error) {
	name := unix.SignalName(sig)
	switch err := s.proof.prove(pid); {
	case errors.Is(err, ErrGone):
		return true, nil
	case err != nil:
		logger.Warn("stop refused", "target_pid", pid, "error", err)
		return false, fmt.Errorf("the saver session's process is not sent %s: %w", name, err)
	}
	switch err := s.signal(pid, sig); {
	case errors.Is(err, syscall.ESRCH):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("send %s to the saver's daemon %d: %w", name, pid, err)
	}
	return false, nil
}

// awaitExit waits up to bound for pid to exit, and reports whether it did. It
// fails only when ctx ends first.
func (s *Stopper) awaitExit(ctx context.Context, pid int, bound time.Duration) (bool, error) {
	deadline := time.NewTimer(bound)
	defer deadline.Stop()
	tick := time.NewTicker(s.poll)
	defer tick.Stop()
	for {
		if !s.alive(pid) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("wait for the saver's daemon %d to exit: %w", pid, context.Cause(ctx))
		case <-deadline.C:
			return !s.alive(pid), nil
		case <-tick.C:
		}
	}
}
