package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
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
	f, err := fileutil.LockWithin(ctx, path, fileutil.LockSpec{
		Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: wait, Poll: bootstrapLockPoll,
	})
	if errors.Is(err, fileutil.ErrLockTimedOut) {
		return nil, fmt.Errorf("%w for %s: %s", ErrBootstrapLockHeld, wait, path)
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
