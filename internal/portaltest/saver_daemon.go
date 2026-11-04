//go:build integration

package portaltest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// saverDaemonSettle bounds the wait for a daemon's exit once its session is
// killed: its final flush is bounded at about 5 s.
const saverDaemonSettle = 10 * time.Second

// SaverDaemon is a `portal state daemon` running in the saver session of a zmx
// directory of the test's own, over a state directory of the test's own.
type SaverDaemon struct {
	// Backend reaches the zmx directory whose sessions the daemon saves.
	Backend *zmx.Backend
	// Env is what the daemon runs with of this process's making:
	// IsolateStateForTest's isolation, and the logging variables this process
	// has set. Its session adds only the minimal set zmxtest passes every
	// session. A process run with it reaches the daemon's sessions once it
	// appends Backend.ReachEnv.
	Env []string
	// StateDir is the daemon's state directory.
	StateDir string
	// PID is the daemon's process, the saver session's own.
	PID int
}

// StartSaverDaemon isolates the test's state, makes it a zmx directory, and
// runs binary, an absolute path to a portal built through portalbintest, as
// `state daemon` in that directory's saver session, with IsolateStateForTest's
// isolation, whose sandbox registers the state directory, and nothing else of
// this process's environment but the logging variables it has set, such as a
// test's PORTAL_LOG_LEVEL. It returns once the daemon holds its lock,
// daemon.pid naming the session's process.
//
// At cleanup it kills the session, which ends the daemon through its final
// flush, and SIGKILLs a daemon still running past the flush's bound. Every
// signal is proved to reach a process of the zmx directory as it is sent. The
// directory's own teardown runs next, then the state directory goes.
func StartSaverDaemon(t *testing.T, binary string) *SaverDaemon {
	t.Helper()
	if !filepath.IsAbs(binary) {
		t.Fatalf("portaltest: StartSaverDaemon wants an absolute path to portal, got %q", binary)
	}
	env, stateDir := isolate(t)
	env = append(env, loggingEnv()...)
	RegisterStateDirTeardownGuard(t, stateDir)
	b := zmxtest.New(t, zmxtest.WithEnv(env...))
	if _, err := b.Create(t.Context(), saver.SessionName, t.TempDir(), []string{binary, "state", "daemon"}); err != nil {
		t.Fatalf("portaltest: create %s: %v", saver.SessionName, err)
	}
	d := &SaverDaemon{Backend: b, Env: env, StateDir: stateDir}
	t.Cleanup(func() { d.stop(t) })
	d.awaitLock(t)
	return d
}

// loggingEnv is the logging variables this process has set, as entries.
func loggingEnv() []string {
	var env []string
	for _, name := range log.Variables() {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// Signal sends sig to the daemon, proved a process of the zmx directory as it
// is sent.
func (d *SaverDaemon) Signal(sig syscall.Signal) error {
	return zmxtest.Signal(d.Backend, d.PID, sig)
}

// Exited reports whether the daemon is gone within timeout: no longer a
// process of the zmx directory.
func (d *SaverDaemon) Exited(t *testing.T, timeout time.Duration) bool {
	t.Helper()
	return harnesstest.PollUntil(t, timeout, 20*time.Millisecond, func() bool { return d.Signal(0) != nil })
}

// saverReadiness is one reading of a starting daemon: the saver session's
// process, or why it couldn't be read, and the pid daemon.pid names.
type saverReadiness struct {
	SessionPID int
	SessionErr string
	PIDFile    int
}

func (d *SaverDaemon) awaitLock(t *testing.T) {
	t.Helper()
	res := harnesstest.AwaitProgress(t, harnesstest.ProgressWait{}, func() saverReadiness {
		var r saverReadiness
		s, err := d.Backend.Session(t.Context(), saver.SessionName)
		if err != nil {
			r.SessionErr = err.Error()
		}
		r.SessionPID = s.PID
		r.PIDFile, _ = state.ReadPIDFile(d.StateDir)
		return r
	}, func(r saverReadiness) bool {
		return r.SessionPID > 0 && r.PIDFile == r.SessionPID
	})
	if !res.Reached {
		t.Fatalf("portaltest: the daemon in %s never held its lock (%s)\n--- portal.log ---\n%s",
			saver.SessionName, res, ReadPortalLogSafe(d.StateDir))
	}
	d.PID = res.Last.SessionPID
}

// stop kills the saver session and waits out the daemon's final flush. A
// daemon that never held its lock is left to the zmx directory's teardown.
func (d *SaverDaemon) stop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), saverDaemonSettle)
	defer cancel()
	if err := d.Backend.Kill(ctx, saver.SessionName); err != nil && !errors.Is(err, backend.ErrNoSuchSession) {
		t.Logf("portaltest: kill %s: %v", saver.SessionName, err)
	}
	if d.PID == 0 || d.Exited(t, saverDaemonSettle) {
		return
	}
	t.Errorf("portaltest: the daemon %d outlived its session's kill by %s; sending SIGKILL", d.PID, saverDaemonSettle)
	if err := d.Signal(syscall.SIGKILL); err != nil {
		t.Errorf("portaltest: SIGKILL the daemon %d: %v", d.PID, err)
	}
}
