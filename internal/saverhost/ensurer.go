package saverhost

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
)

// ReadyWait is how long a daemon the saver session starts has to hold its
// lock. Under a second on an idle machine: an exec, the state directory and
// sessions.json read, the lock's check of daemon.pid (a ps), then two small
// writes. The rest is for a machine under heavy load, where each of those can
// stall for seconds, while the bootstrap waiting on it stays bounded.
const ReadyWait = 10 * time.Second

// ErrNotReady is a saver whose daemon did not come up: its session ended, or
// its daemon held no lock within ReadyWait.
var ErrNotReady = errors.New("the saver's daemon did not come up")

// Reasons a live saver is replaced.
const (
	reasonDev     = "dev"
	reasonVersion = "version"
	reasonUnknown = "unknown"
)

// Daemon is the daemon the saver session runs: `Exe state daemon`.
type Daemon struct {
	// Exe is the absolute path of the portal binary to run, so the saver
	// never runs whichever portal PATH finds.
	Exe string
	// Version is Exe's version. A live saver is kept only while its daemon
	// records this version; a dev version keeps none.
	Version string
	// StateDir is the state directory the daemon records daemon.pid and
	// daemon.version in, and the session's working directory.
	StateDir string
	// ReadyWait bounds the wait for a new daemon to hold its lock; zero is
	// ReadyWait.
	ReadyWait time.Duration
}

// Ensurer keeps the saver running as one Daemon.
type Ensurer struct {
	stopper *Stopper
	daemon  Daemon
}

// NewEnsurer binds an Ensurer to stopper, which stops a saver it replaces,
// and to d, which it refuses without an absolute Exe, a Version or a
// StateDir.
func NewEnsurer(stopper *Stopper, d Daemon) (*Ensurer, error) {
	switch {
	case stopper == nil:
		return nil, errors.New("saverhost: no stopper")
	case !filepath.IsAbs(d.Exe):
		return nil, fmt.Errorf("saverhost: the daemon's executable %q is no absolute path", d.Exe)
	case d.Version == "":
		return nil, errors.New("saverhost: no version for the daemon")
	case d.StateDir == "":
		return nil, errors.New("saverhost: no state directory for the daemon")
	}
	if d.ReadyWait == 0 {
		d.ReadyWait = ReadyWait
	}
	return &Ensurer{stopper: stopper, daemon: d}, nil
}

// EnsureSaver keeps a live saver whose daemon records the Daemon's version,
// stopping any other, then creates the saver where there is none and waits
// for its daemon to hold its lock. A saver another process creates first
// counts, once its daemon holds the lock. A saver that can't be read, can't
// be stopped or doesn't come up is an error, never a wait past its bounds.
func (e *Ensurer) EnsureSaver(ctx context.Context) error {
	pid, present, err := e.stopper.SaverPID(ctx)
	if err != nil {
		return fmt.Errorf("read the saver: %w", err)
	}
	if present {
		reason := e.replacing(pid)
		if reason == "" {
			logger.Info("daemon kept", "target_pid", pid)
			return nil
		}
		logger.Info("recycle", "target_pid", pid, "reason", reason)
		if _, err := e.stopper.Stop(ctx); err != nil {
			return fmt.Errorf("recycle the saver: %w", err)
		}
	}
	if err := e.create(ctx); err != nil {
		return err
	}
	return e.awaitReady(ctx)
}

// replacing is why the live saver running pid is replaced, or "" when it is
// kept: its daemon records this version as the lock's holder. A dev build
// keeps none, its version naming no one build for sure.
func (e *Ensurer) replacing(pid int) string {
	if devversion.Is(e.daemon.Version) {
		return reasonDev
	}
	holder, err := state.ReadPIDFile(e.daemon.StateDir)
	if err != nil || holder != pid {
		return reasonUnknown
	}
	switch version, err := state.ReadVersionFile(e.daemon.StateDir); {
	case err != nil:
		return reasonUnknown
	case version != e.daemon.Version:
		return reasonVersion
	}
	return ""
}

// create creates the saver session. A name another process created first
// counts as made.
func (e *Ensurer) create(ctx context.Context) error {
	command := []string{e.daemon.Exe, "state", "daemon"}
	session, err := e.stopper.backend.Create(ctx, saver.SessionName, e.daemon.StateDir, command)
	switch {
	case errors.Is(err, backend.ErrSessionExists):
		logger.Debug("session created elsewhere", "error", err)
		return nil
	case err != nil:
		return fmt.Errorf("create the saver session: %w", err)
	}
	logger.Info("session created", "target_pid", session.PID)
	return nil
}

// awaitReady waits up to the Daemon's ReadyWait for the saver session's
// process to hold the daemon's lock, daemon.pid naming it. A session that
// ends first, its daemon refusing to run, fails at once.
func (e *Ensurer) awaitReady(ctx context.Context) error {
	start := time.Now()
	deadline := time.NewTimer(e.daemon.ReadyWait)
	defer deadline.Stop()
	tick := time.NewTicker(e.stopper.poll)
	defer tick.Stop()
	for {
		r := e.readiness(ctx)
		switch {
		case r.ready():
			logger.Info("daemon ready", "target_pid", r.session, log.Took(start))
			return nil
		case r.ended():
			return fmt.Errorf("%w: its session ended", ErrNotReady)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the saver's daemon: %w", context.Cause(ctx))
		case <-deadline.C:
			return fmt.Errorf("%w within %s: %s", ErrNotReady, e.daemon.ReadyWait, r)
		case <-tick.C:
		}
	}
}

// readiness is one reading of a starting saver: the process its session runs,
// whether there is a session, or why it couldn't be read, and the pid
// daemon.pid names.
type readiness struct {
	session    int
	present    bool
	sessionErr error
	holder     int
	holderErr  error
}

func (e *Ensurer) readiness(ctx context.Context) readiness {
	var r readiness
	r.session, r.present, r.sessionErr = e.stopper.SaverPID(ctx)
	r.holder, r.holderErr = state.ReadPIDFile(e.daemon.StateDir)
	return r
}

func (r readiness) ready() bool {
	return r.present && r.holderErr == nil && r.holder == r.session
}

func (r readiness) ended() bool {
	return r.sessionErr == nil && !r.present
}

func (r readiness) String() string {
	switch {
	case r.sessionErr != nil:
		return r.sessionErr.Error()
	case r.holderErr != nil:
		return fmt.Sprintf("the saver session runs %d; %v", r.session, r.holderErr)
	}
	return fmt.Sprintf("the saver session runs %d; daemon.pid names %d", r.session, r.holder)
}
