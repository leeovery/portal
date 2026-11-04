package saverhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/leeovery/portal/internal/backend"
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
// within ReadyWait its daemon held no lock or recorded another version.
var ErrNotReady = errors.New("the saver's daemon did not come up")

// Reasons a live saver is replaced.
const (
	reasonVersion = "version"
	reasonUnknown = "unknown"
)

// Daemon is the daemon the saver session runs: `Exe state daemon`.
type Daemon struct {
	// Exe is the absolute path of the portal binary to run, so the saver
	// never runs whichever portal PATH finds.
	Exe string
	// Version is Exe's version, a dev build's naming the build. A live saver
	// is kept only while its daemon records this version, and a daemon
	// started is up only once it records it.
	Version string
}

// Ensurer is the Host's ensure bound to one Daemon, as the bootstrap asks for
// it.
type Ensurer struct {
	host   *Host
	daemon Daemon
}

// Ensurer binds the Host to d, which it refuses without an absolute Exe or a
// Version.
func (h *Host) Ensurer(d Daemon) (*Ensurer, error) {
	switch {
	case !filepath.IsAbs(d.Exe):
		return nil, fmt.Errorf("saverhost: the daemon's executable %q is no absolute path", d.Exe)
	case d.Version == "":
		return nil, errors.New("saverhost: no version for the daemon")
	}
	return &Ensurer{host: h, daemon: d}, nil
}

// EnsureSaver ensures the saver runs the Ensurer's Daemon, as the Host's
// ensure does.
func (e *Ensurer) EnsureSaver(ctx context.Context) error {
	return e.host.ensure(ctx, e.daemon)
}

// ensure keeps a live saver whose daemon records d's version, stopping any
// other, then creates the saver where there is none and waits for its daemon
// to hold the lock and record the version. It holds saver.lock throughout,
// creating the state directory for it. A saver another process creates first
// counts once its daemon does the same. A saver that can't be read, can't be
// stopped or doesn't come up is an error, never a wait past its bounds.
func (h *Host) ensure(ctx context.Context, d Daemon) error {
	if err := os.MkdirAll(h.stateDir, 0o700); err != nil {
		return fmt.Errorf("create the state directory: %w", err)
	}
	release, err := h.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	pid, present, err := h.SaverPID(ctx)
	if err != nil {
		return fmt.Errorf("read the saver: %w", err)
	}
	if present {
		reason := h.replacing(pid, d.Version)
		if reason == "" {
			logger.Info("daemon kept", "target_pid", pid)
			return nil
		}
		logger.Info("recycle", "target_pid", pid, "reason", reason)
		if err := h.stop(ctx, pid); err != nil {
			return fmt.Errorf("recycle the saver: %w", err)
		}
	}
	if err := h.create(ctx, d.Exe); err != nil {
		return err
	}
	return h.awaitReady(ctx, d.Version)
}

// replacing is why the live saver running pid is replaced, or "" when it is
// kept: its daemon records version as the lock's holder.
func (h *Host) replacing(pid int, version string) string {
	holder, err := state.ReadPIDFile(h.stateDir)
	if err != nil || holder != pid {
		return reasonUnknown
	}
	switch recorded, err := state.ReadVersionFile(h.stateDir); {
	case err != nil:
		return reasonUnknown
	case recorded != version:
		return reasonVersion
	}
	return ""
}

// create creates the saver session running `exe state daemon`. A name another
// process created first counts as made.
func (h *Host) create(ctx context.Context, exe string) error {
	session, err := h.backend.Create(ctx, saver.SessionName, h.stateDir, []string{exe, "state", "daemon"})
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

// awaitReady waits up to ReadyWait for the saver session's process to hold
// the daemon's lock, daemon.pid naming it, and to record version: the daemon
// records its version just after its pid. A session that ends first, its
// daemon refusing to run, fails at once.
func (h *Host) awaitReady(ctx context.Context, version string) error {
	start := time.Now()
	deadline := time.NewTimer(h.readyWait)
	defer deadline.Stop()
	tick := time.NewTicker(h.poll)
	defer tick.Stop()
	for {
		r := h.readiness(ctx)
		switch {
		case r.ready(version):
			logger.Info("daemon ready", "target_pid", r.session, log.Took(start))
			return nil
		case r.ended():
			return fmt.Errorf("%w: its session ended", ErrNotReady)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the saver's daemon: %w", context.Cause(ctx))
		case <-deadline.C:
			return fmt.Errorf("%w within %s: %s", ErrNotReady, h.readyWait, r)
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

func (h *Host) readiness(ctx context.Context) readiness {
	var r readiness
	r.session, r.present, r.sessionErr = h.SaverPID(ctx)
	r.holder, r.holderErr = state.ReadPIDFile(h.stateDir)
	r.version, r.versionErr = state.ReadVersionFile(h.stateDir)
	return r
}

// ready reports whether the saver session's process holds the daemon's lock,
// daemon.pid naming it, and daemon.version holds version.
func (r readiness) ready(version string) bool {
	return r.present && r.holderErr == nil && r.holder == r.session &&
		r.versionErr == nil && r.version == version
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
