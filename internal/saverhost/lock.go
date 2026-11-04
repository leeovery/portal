package saverhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// lockName is the lock every judgement, stop and creation of the saver holds,
// so no two run at once: two windows bootstrapping at login, or an ensure
// racing uninstall. The bootstrap's own lock, where one wraps it, is taken
// first.
const lockName = "saver.lock"

// LockWait bounds the wait for saver.lock. It covers the longest run another
// holder can make: a recycle's stop (TermWait, then the kill bound after its
// SIGKILL) and the create that follows (ReadyWait), with 20 s over for the
// backend calls between them, zmx bounding its create at 10 s, its kill at
// 5 s and each read at 1 s. A holder still running past it is stuck, and the
// wait gives up rather than queue behind it.
const LockWait = TermWait + killWait + ReadyWait + 20*time.Second

// ErrLockHeld is a saver.lock another process held past the wait's bound.
var ErrLockHeld = errors.New("the saver's lock is held by another process")

// LockPath is saver.lock in the state directory dir.
func LockPath(dir string) string { return filepath.Join(dir, lockName) }

// lock takes saver.lock exclusively, waiting up to the Host's LockWait, and
// returns its release. The kernel releases it too when the holder exits, so a
// crashed holder never wedges the next. A state directory that isn't there
// fails with an error wrapping fs.ErrNotExist.
func (h *Host) lock(ctx context.Context) (func(), error) {
	path := LockPath(h.stateDir)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open saver.lock: %w", err)
	}
	release := func() { _ = f.Close() }
	deadline := time.NewTimer(h.lockWait)
	defer deadline.Stop()
	tick := time.NewTicker(h.poll)
	defer tick.Stop()
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return release, nil
		case !errors.Is(err, unix.EWOULDBLOCK):
			release()
			return nil, fmt.Errorf("flock %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			release()
			return nil, fmt.Errorf("wait for %s: %w", path, context.Cause(ctx))
		case <-deadline.C:
			release()
			return nil, fmt.Errorf("%w for %s: %s", ErrLockHeld, h.lockWait, path)
		case <-tick.C:
		}
	}
}
