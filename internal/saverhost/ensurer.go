package saverhost

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
)

// ReadyWait is how long a daemon the saver session starts has to hold its
// lock and record its version. Under a second on an idle machine: an exec,
// the state directory and sessions.json read, the lock's check of daemon.pid
// (a ps), then two small writes. The rest is for a machine under heavy load,
// where each of those can stall for seconds, while the bootstrap waiting on
// it stays bounded.
const ReadyWait = 10 * time.Second

// ErrNotReady is a saver whose daemon did not come up: its session ended, or
// within ReadyWait its daemon held no lock or recorded no version wanted.
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
	// Version is Exe's version. EnsureSaver keeps a live saver only while its
	// daemon records this version, a dev version keeping none, and waits for
	// a daemon it starts to record it.
	Version string
	// StateDir is the state directory the daemon records daemon.pid and
	// daemon.version in, and the session's working directory.
	StateDir string
}

// SaverStopper stops the saver's daemon pid, read as the saver session's
// process, as a Stopper does.
type SaverStopper interface {
	Stop(ctx context.Context, pid int) error
}

// EnsurerConfig is what an Ensurer works over.
type EnsurerConfig struct {
	// Backend creates and reads the saver session.
	Backend backend.Backend
	// Stopper stops a saver EnsureSaver replaces.
	Stopper SaverStopper
	// Daemon is the daemon the saver runs.
	Daemon Daemon
	// ReadyWait bounds the wait for a new daemon to come up, and Poll spaces
	// its reads; zero is the default.
	ReadyWait, Poll time.Duration
}

// Ensurer keeps the saver running as one Daemon.
type Ensurer struct {
	backend         backend.Backend
	stopper         SaverStopper
	daemon          Daemon
	readyWait, poll time.Duration
}

// NewEnsurer binds an Ensurer to cfg, which it refuses without a backend, a
// stopper, an absolute Exe, a Version or a StateDir.
func NewEnsurer(cfg EnsurerConfig) (*Ensurer, error) {
	switch d := cfg.Daemon; {
	case cfg.Backend == nil:
		return nil, errors.New("saverhost: no backend")
	case cfg.Stopper == nil:
		return nil, errors.New("saverhost: no stopper")
	case !filepath.IsAbs(d.Exe):
		return nil, fmt.Errorf("saverhost: the daemon's executable %q is no absolute path", d.Exe)
	case d.Version == "":
		return nil, errors.New("saverhost: no version for the daemon")
	case d.StateDir == "":
		return nil, errors.New("saverhost: no state directory for the daemon")
	}
	e := &Ensurer{backend: cfg.Backend, stopper: cfg.Stopper, daemon: cfg.Daemon, readyWait: cfg.ReadyWait, poll: cfg.Poll}
	if e.readyWait == 0 {
		e.readyWait = ReadyWait
	}
	if e.poll == 0 {
		e.poll = pollEvery
	}
	return e, nil
}

// EnsureSaver, the cold bootstrap's, keeps a live saver whose daemon records
// the Daemon's version, stopping any other, then creates the saver where
// there is none and waits for its daemon to hold the lock and record the
// version. A saver another process creates first, or puts in place of one
// being stopped, counts once its daemon does the same. A saver that can't be
// read, can't be stopped or doesn't come up is an error, never a wait past
// its bounds.
func (e *Ensurer) EnsureSaver(ctx context.Context) error {
	pid, present, err := saverPID(ctx, e.backend)
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
		if err := e.stopper.Stop(ctx, pid); err != nil && !errors.Is(err, ErrReplaced) {
			return fmt.Errorf("recycle the saver: %w", err)
		}
	}
	return e.start(ctx, func(r readiness) bool { return r.heldAt(e.daemon.Version) })
}

// EnsureRunning, the warm path's, creates the saver only when there is none,
// and waits for its daemon to hold the lock. It judges no version: a live
// saver is kept whatever it runs, so a dev build doesn't replace the saver
// on every command.
func (e *Ensurer) EnsureRunning(ctx context.Context) error {
	_, present, err := saverPID(ctx, e.backend)
	switch {
	case err != nil:
		return fmt.Errorf("read the saver: %w", err)
	case present:
		return nil
	}
	return e.start(ctx, readiness.held)
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

// start creates the saver session and waits until ready finds its daemon up.
func (e *Ensurer) start(ctx context.Context, ready func(readiness) bool) error {
	if err := e.create(ctx); err != nil {
		return err
	}
	return e.awaitReady(ctx, ready)
}

// create creates the saver session. A name another process created first
// counts as made.
func (e *Ensurer) create(ctx context.Context) error {
	command := []string{e.daemon.Exe, "state", "daemon"}
	session, err := e.backend.Create(ctx, saver.SessionName, e.daemon.StateDir, command)
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

// awaitReady waits up to the ReadyWait for ready to find the saver's daemon
// up. A session that ends first, its daemon refusing to run, fails at once.
func (e *Ensurer) awaitReady(ctx context.Context, ready func(readiness) bool) error {
	start := time.Now()
	deadline := time.NewTimer(e.readyWait)
	defer deadline.Stop()
	tick := time.NewTicker(e.poll)
	defer tick.Stop()
	for {
		r := e.readiness(ctx)
		switch {
		case ready(r):
			logger.Info("daemon ready", "target_pid", r.session, log.Took(start))
			return nil
		case r.ended():
			return fmt.Errorf("%w: its session ended", ErrNotReady)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the saver's daemon: %w", context.Cause(ctx))
		case <-deadline.C:
			return fmt.Errorf("%w within %s: %s", ErrNotReady, e.readyWait, r)
		case <-tick.C:
		}
	}
}

// readiness is one reading of a starting saver: the process its session runs,
// whether there is a session, or why it couldn't be read, and what
// daemon.pid and daemon.version hold.
type readiness struct {
	session    int
	present    bool
	sessionErr error
	holder     int
	holderErr  error
	version    string
	versionErr error
}

func (e *Ensurer) readiness(ctx context.Context) readiness {
	var r readiness
	r.session, r.present, r.sessionErr = saverPID(ctx, e.backend)
	r.holder, r.holderErr = state.ReadPIDFile(e.daemon.StateDir)
	r.version, r.versionErr = state.ReadVersionFile(e.daemon.StateDir)
	return r
}

// held reports whether the saver session's process holds the daemon's lock,
// daemon.pid naming it.
func (r readiness) held() bool {
	return r.present && r.holderErr == nil && r.holder == r.session
}

// heldAt is held, with daemon.version holding version: the daemon records
// its version just after its pid.
func (r readiness) heldAt(version string) bool {
	return r.held() && r.versionErr == nil && r.version == version
}

func (r readiness) ended() bool {
	return r.sessionErr == nil && !r.present
}

func (r readiness) String() string {
	if r.sessionErr != nil {
		return r.sessionErr.Error()
	}
	holder, version := strconv.Itoa(r.holder), strconv.Quote(r.version)
	if r.holderErr != nil {
		holder = r.holderErr.Error()
	}
	if r.versionErr != nil {
		version = r.versionErr.Error()
	}
	return fmt.Sprintf("the saver session runs %d; daemon.pid: %s; daemon.version: %s", r.session, holder, version)
}
