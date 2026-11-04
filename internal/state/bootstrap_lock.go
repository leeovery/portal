package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// bootstrapLockName is the lock a bootstrap holds for its whole run, so two
// never run at once: two windows opening at login would race on the
// restoring flag and restore each session twice. It is taken before
// saver.lock, never after.
const bootstrapLockName = "bootstrap.lock"

// bootstrapLockPoll spaces the tries a wait for bootstrap.lock makes.
const bootstrapLockPoll = 50 * time.Millisecond

// ErrBootstrapLockHeld is a bootstrap.lock another process held past the
// wait's bound.
var ErrBootstrapLockHeld = errors.New("bootstrap.lock is held by another process")

// BootstrapLock is bootstrap.lock in the state directory dir.
func BootstrapLock(dir string) string { return filepath.Join(dir, bootstrapLockName) }

// AcquireBootstrapLock takes bootstrap.lock in dir exclusively, creating dir
// for it, and returns its release. It waits up to wait for another holder,
// and fails with an error wrapping ErrBootstrapLockHeld once that has passed,
// or with ctx's cause should ctx end first. The kernel releases the lock too
// when its holder exits, so a crashed bootstrap never wedges the next.
func AcquireBootstrapLock(ctx context.Context, dir string, wait time.Duration) (release func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the state directory: %w", err)
	}
	path := BootstrapLock(dir)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	release = func() { _ = f.Close() }
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(bootstrapLockPoll)
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
			return nil, fmt.Errorf("%w for %s: %s", ErrBootstrapLockHeld, wait, path)
		case <-tick.C:
		}
	}
}
