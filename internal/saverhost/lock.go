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

// The ceilings LockWait allows the backend's calls: a read of the saver
// session, its creation, and its kill. zmx's own are pinned within them.
const (
	ReadCeiling   = time.Second
	CreateCeiling = 10 * time.Second
	KillCeiling   = 5 * time.Second
)

// JudgeRun bounds a judgement of the saver's protocol: a read in Portal's
// protocol, a read in an older zmx's, and the end of a saver judged
// unusable, bounded as a kill. Ensure's caller keeps its judge of a saver
// made within it.
const JudgeRun = 2*ReadCeiling + KillCeiling

// holderRun is the longest a holder of saver.lock runs, each wait at its
// bound and each backend call at its ceiling: the ensure's wait for a saver
// still starting (ReadyWait, then the read in flight); the recycle's stop (a
// read of the session, TermWait, the kill bound, then the session's kill: a
// read, the kill, a read); then the create, its judgement, and the wait for
// its daemon (ReadyWait, then the read in flight).
const holderRun = ReadyWait + ReadCeiling +
	ReadCeiling + TermWait + killWait + ReadCeiling + KillCeiling + ReadCeiling +
	CreateCeiling + JudgeRun + ReadyWait + ReadCeiling

// LockWait bounds the wait for saver.lock: a holder's longest run, with 15 s
// over for what no bound covers, the proofs' ps calls and the reads of the
// daemon's files. A holder still running past it is stuck, and the wait gives
// up rather than queue behind it.
const LockWait = holderRun + 15*time.Second

// ErrLockHeld is a saver.lock another process held past the wait's bound.
var ErrLockHeld = errors.New("the saver's lock is held by another process")

// LockPath is saver.lock in the state directory dir.
func LockPath(dir string) string { return filepath.Join(dir, lockName) }

// lockCreating is lock, creating the state directory for it first.
func (h *Host) lockCreating(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(h.stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create the state directory: %w", err)
	}
	return h.lock(ctx)
}

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
