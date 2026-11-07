package saverhost

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

// lockName is the lock every judgement, stop and creation of the saver holds,
// so no two run at once: two windows bootstrapping at login, or an ensure
// racing uninstall. The bootstrap's own lock, where one wraps it, is taken
// first.
const lockName = "saver.lock"

// The ceilings LockWait allows the backend's calls: a read of the saver
// session, its creation, and its kill; and RunGrace, how much longer than its
// ceiling a call that runs the backend's binary, killed at the ceiling, has
// for its output to close. zmx's own are pinned within them.
const (
	ReadCeiling   = time.Second
	CreateCeiling = 10 * time.Second
	KillCeiling   = 5 * time.Second
	RunGrace      = time.Second
)

// DetachCeiling is the ceiling a take's lock allows the backend's detach of
// a session's clients, pinned beside the others so every lock's bound reads
// zmx's ceilings from one place.
const DetachCeiling = 5 * time.Second

// JudgeRun bounds a judgement of the saver's protocol: a read in Portal's
// protocol, a read in an older zmx's, and the end of a saver judged
// unusable, zmx kill, a run of the binary, then the wait for a proved
// signal's end, each bounded as a kill. Ensure's caller keeps its judge of a
// saver made within it. A create that refuses the session an older zmx made,
// a read in its framing and then its zmx kill, takes the judge's place within
// the same bound.
const JudgeRun = 2*ReadCeiling + 2*KillCeiling + RunGrace

// holderRun is the longest a holder of saver.lock runs, each wait at its
// bound and each backend call at its ceiling: the ensure's wait for a saver
// still starting (ReadyWait, then the read in flight); the recycle's stop (a
// read of the session, TermWait, the kill bound, then the session's kill: a
// read, the kill, a read); then the create, a run of the binary, its
// judgement, and the wait for its daemon (ReadyWait, then the read in
// flight).
const holderRun = ReadyWait + ReadCeiling +
	ReadCeiling + TermWait + killWait + ReadCeiling + KillCeiling + ReadCeiling +
	CreateCeiling + RunGrace + JudgeRun + ReadyWait + ReadCeiling

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
	f, err := fileutil.LockWithin(ctx, path, fileutil.LockSpec{
		Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: h.lockWait, Poll: h.poll,
	})
	var lockErr *fileutil.LockError
	switch {
	case errors.Is(err, fileutil.ErrLockTimedOut):
		return nil, fmt.Errorf("%w for %s: %s", ErrLockHeld, h.lockWait, path)
	case errors.As(err, &lockErr) && lockErr.Step == fileutil.LockOpen:
		return nil, fmt.Errorf("open saver.lock: %w", lockErr.Err)
	case err != nil:
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
