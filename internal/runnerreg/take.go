package runnerreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// A take's files are in takes/ in the registry's directory, named by its
// session's stem, so takes of names a filesystem folds together keep their
// own: <stem>.json, the record of the session's last take, and <stem>.lock,
// the lock every take of the session holds.
const (
	takesDir      = "takes"
	takeRecordExt = ".json"
	takeLockExt   = ".lock"
)

// takeLockPoll spaces the tries a wait for a take's lock makes.
const takeLockPoll = 10 * time.Millisecond

// takeLife is how long a take's files are kept: a record is read only by
// the drop its take made, at once, and a lock's file outlives its take only
// when the take died holding it.
const takeLife = time.Hour

// selfPID is this process's pid, whose own runner a take never asks: a
// variable so a test's process can stand in for another runner's.
var selfPID = os.Getpid()

// taking is a take as recorded: where its session went, and when.
type taking struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

func (r Registry) takesPath() string { return filepath.Join(r.dir, takesDir) }

// takeFile is session's take file ending in ext. A name that can't name a
// file is refused with state.ErrUnsafeName.
func (r Registry) takeFile(session, ext string) (string, error) {
	stem, err := state.SessionStem(session)
	if err != nil {
		return "", err
	}
	return filepath.Join(r.takesPath(), stem+ext), nil
}

// makeTakesDir makes takes/, in the registry's directory, which only this
// user can reach.
func (r Registry) makeTakesDir() error {
	if err := r.makeDir(); err != nil {
		return err
	}
	return os.MkdirAll(r.takesPath(), 0o700)
}

// LockTake takes the lock every take of session holds, waiting up to wait
// for another holder, and returns its release, which may be called more than
// once. A take holds it from its first ask until the attach it takes for
// leads the session, so no take judges the session while another's attach is
// still on its way to it, and of two at once the later is the one left
// attached; a take of another session holds a lock of its own. The release
// removes the lock's file as it lets the lock go, and a take that finds the
// file it locked no longer the one there locks the one there now, so the
// lock leaves nothing behind and is never held twice. The kernel releases it
// too when its holder exits. A read-only registry refuses, taking nothing.
func (r Registry) LockTake(ctx context.Context, session string, wait time.Duration) (release func(), err error) {
	if r.readOnly {
		return nil, errReadOnly
	}
	path, err := r.takeFile(session, takeLockExt)
	if err == nil {
		err = r.makeTakesDir()
	}
	if err != nil {
		return nil, fmt.Errorf("lock the take of %s: %w", session, err)
	}
	deadline := time.Now().Add(wait)
	for {
		f, err := lockFile(ctx, path, time.Until(deadline))
		if err != nil {
			return nil, err
		}
		if isAt(f, path) {
			return sync.OnceFunc(func() { removeHeld(f, path) }), nil
		}
		_ = f.Close()
		if !time.Now().Before(deadline) {
			return nil, &fileutil.LockError{Step: fileutil.LockTimedOut, Path: path, Err: fileutil.ErrLockTimedOut}
		}
	}
}

// lockFile locks the file at path, making it if it isn't there, waiting up
// to wait for another holder.
func lockFile(ctx context.Context, path string, wait time.Duration) (*os.File, error) {
	return fileutil.LockWithin(ctx, path, fileutil.LockSpec{
		Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: wait, Poll: takeLockPoll,
	})
}

// isAt reports whether f, a lock's file, is still the file at path: a holder
// removes it as it lets go, and a lock on the file it removed locks nothing.
func isAt(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	there, err := os.Stat(path)
	return err == nil && os.SameFile(held, there)
}

// removeHeld removes path, the file f locks, then lets the lock go.
func removeHeld(f *os.File, path string) {
	_ = os.Remove(path)
	_ = f.Close()
}

// RecordTake records that session is being taken, now, to by: for the
// runners a take ends only through the backend's detach, which says nothing
// of where the session went. Each take of a session replaces its last one's
// record. A read-only registry refuses, recording nothing.
func (r Registry) RecordTake(session, by string) error {
	if r.readOnly {
		return errReadOnly
	}
	path, err := r.takeFile(session, takeRecordExt)
	var data []byte
	if err == nil {
		data, err = json.Marshal(taking{By: by, At: time.Now()})
	}
	if err == nil {
		err = r.makeTakesDir()
	}
	if err == nil {
		err = writeFile(path, data)
	}
	if err != nil {
		return fmt.Errorf("record the take of %s: %w", session, err)
	}
	return nil
}

// TakenBy is where session went as its last take recorded it: empty unless
// that take was recorded no earlier than since. A take recorded before an
// attach began is not the one that ended it.
func (r Registry) TakenBy(session string, since time.Time) string {
	path, err := r.takeFile(session, takeRecordExt)
	var data []byte
	if err == nil {
		data, err = os.ReadFile(path)
	}
	var tk taking
	if err == nil {
		err = decodeStrict(data, fields{"by": &tk.By, "at": &tk.At})
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ""
	case err != nil:
		logger.Debug("take record unreadable", "path", path, "error", err)
		return ""
	case tk.At.Before(since):
		return ""
	}
	return tk.By
}

// sweepTakes removes the take files older than takeLife: a record, and a
// lock's file no take holds. What it can't read or lock it leaves.
func (r Registry) sweepTakes() {
	if r.readOnly {
		return
	}
	files, err := os.ReadDir(r.takesPath())
	if err != nil {
		return
	}
	for _, file := range files {
		info, err := file.Info()
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < takeLife {
			continue
		}
		if path := filepath.Join(r.takesPath(), file.Name()); removeStaleTake(path) {
			logger.Debug("stale take removed", "path", path)
		}
	}
}

// removeStaleTake removes path, a take file past its life, and reports
// whether it did: a record, or a lock's file no take holds.
func removeStaleTake(path string) bool {
	switch filepath.Ext(path) {
	case takeRecordExt:
		return os.Remove(path) == nil
	case takeLockExt:
		return removeUnheld(path)
	}
	return false
}

// removeUnheld removes the lock's file at path if no take holds it, and
// reports whether it did.
func removeUnheld(path string) bool {
	f, err := lockFile(context.Background(), path, 0)
	if err != nil {
		return false
	}
	if !isAt(f, path) {
		_ = f.Close()
		return false
	}
	removeHeld(f, path)
	return true
}
