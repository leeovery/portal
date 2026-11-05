package migrate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// DaemonBound bounds the wait for the tmux version's daemon to exit once its
// saver session is killed. tmux's SIGHUP starts the daemon's final flush,
// which the tmux version never bounded: it waits up to 5 s for commit.lock,
// then captures every pane once and commits, a tmux call or two and a file
// write per pane. With some 40 panes that is a second or two, under heavy
// load several; 30 s covers the lock's whole bound and a capture pass at
// half a second a pane. A daemon still running then is left to finish, and
// migrate stops, so a re-run waits again.
const DaemonBound = 30 * time.Second

// daemonPoll spaces the reads of whether the daemon has exited.
const daemonPoll = 50 * time.Millisecond

// Daemon proves, and waits out, the tmux version's save daemon. A nil field
// is production's.
type Daemon struct {
	// Identify reads whether pid is a portal state daemon: state.IdentifyDaemon.
	Identify func(pid int) (state.IdentifyResult, error)
	// Sleep waits d, or until ctx ends, returning its cause; a timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is time.Now.
	Now func() time.Time
}

// AwaitExit waits, within DaemonBound, for the daemon the tmux version's
// state dir dir names in daemon.pid to exit, and returns its pid, zero when
// it names none. Nothing is signalled: killing the saver session sent the
// daemon tmux's SIGHUP, on which it flushes and exits. The pid is proved the
// daemon as it is read, its process's argv a portal state daemon's; a pid
// that names no process, or one that isn't the daemon, as a pid reused, has
// exited. waiting hears the pid once, when the daemon is first read running.
// A daemon still running at the bound, or one whose process can't be read
// until then, is refused with ErrDaemonRunning. A daemon.pid that can't be
// read is an error; one absent, or not a number, names no daemon.
func (d Daemon) AwaitExit(ctx context.Context, dir string, waiting func(pid int)) (int, error) {
	pid, err := state.ReadPIDFile(dir)
	if _, unnumbered := errors.AsType[*strconv.NumError](err); errors.Is(err, state.ErrPIDFileAbsent) || unnumbered {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read the tmux version's daemon.pid: %w", err)
	}
	deadline := d.now().Add(DaemonBound)
	told := false
	for {
		running, err := d.running(pid)
		if err == nil && !running {
			return pid, nil
		}
		if running && !told {
			waiting(pid)
			told = true
		}
		if !d.now().Before(deadline) {
			return pid, stillRunning(pid, err)
		}
		if err := d.sleep(ctx, daemonPoll); err != nil {
			return pid, fmt.Errorf("wait for the tmux version's daemon (pid %d): %w", pid, err)
		}
	}
}

// stillRunning refuses a run while the daemon pid runs, or can't be told to
// have exited, its process unreadable for unread.
func stillRunning(pid int, unread error) error {
	why := ""
	if unread != nil {
		why = fmt.Sprintf(daemonUnreadCopy, unread)
	}
	return refused(ErrDaemonRunning, daemonRunningCopy, pid, DaemonBound, why)
}

// running reads whether pid is a portal state daemon still.
func (d Daemon) running(pid int) (bool, error) {
	identify := d.Identify
	if identify == nil {
		identify = state.IdentifyDaemon
	}
	result, err := identify(pid)
	return err == nil && result == state.IdentifyIsPortalDaemon, err
}

func (d Daemon) now() time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

func (d Daemon) sleep(ctx context.Context, wait time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, wait)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}
