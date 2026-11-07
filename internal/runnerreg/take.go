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
	"golang.org/x/sys/unix"
)

// takeFile holds the last take's record, and takeLockFile is the lock takes
// hold, both in the registry's directory under names no entry or socket has.
const (
	takeFile     = "take"
	takeLockFile = "take.lock"
)

// takeLockPoll spaces the tries a wait for the take lock makes.
const takeLockPoll = 10 * time.Millisecond

// selfPID is this process's pid, whose own runner a take never asks: a
// variable so a test's process can stand in for another runner's.
var selfPID = os.Getpid()

// taking is a take as recorded: the session taken, where it went, and when.
type taking struct {
	Session string    `json:"session"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

func (r Registry) takePath() string     { return filepath.Join(r.dir, takeFile) }
func (r Registry) takeLockPath() string { return filepath.Join(r.dir, takeLockFile) }

// LockTake takes the lock every take holds, waiting up to wait for another
// holder, and returns its release, which may be called more than once. A
// take holds it from its first ask until the attach it takes for leads the
// session, so no take judges a session while another's attach is still on
// its way to it, and of two at once the later is the one left attached. The
// kernel releases it too when its holder exits. A read-only registry
// refuses, taking nothing.
func (r Registry) LockTake(ctx context.Context, wait time.Duration) (release func(), err error) {
	if r.readOnly {
		return nil, errReadOnly
	}
	path := r.takeLockPath()
	if err := r.makeDir(); err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	f, err := fileutil.LockWithin(ctx, path, fileutil.LockSpec{
		Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: wait, Poll: takeLockPoll,
	})
	if err != nil {
		return nil, err
	}
	return sync.OnceFunc(func() { _ = f.Close() }), nil
}

// RecordTake records that session is being taken, now, to by: for the
// runners a take ends only through the backend's detach, which says nothing
// of where the session went. Each take's record replaces the last one's. A
// read-only registry refuses, recording nothing.
func (r Registry) RecordTake(session, by string) error {
	if r.readOnly {
		return errReadOnly
	}
	data, err := json.Marshal(taking{Session: session, By: by, At: time.Now()})
	if err == nil {
		err = r.makeDir()
	}
	if err == nil {
		err = writeFile(r.takePath(), data)
	}
	if err != nil {
		return fmt.Errorf("record the take of %s: %w", session, err)
	}
	return nil
}

// TakenBy is where session went as the last take recorded it: empty unless
// that take was of session, recorded no earlier than since. A take recorded
// before an attach began is not the one that ended it.
func (r Registry) TakenBy(session string, since time.Time) string {
	path := r.takePath()
	data, err := os.ReadFile(path)
	var tk taking
	if err == nil {
		err = decodeStrict(data, fields{"session": &tk.Session, "by": &tk.By, "at": &tk.At})
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ""
	case err != nil:
		logger.Debug("take record unreadable", "path", path, "error", err)
		return ""
	case tk.Session != session || tk.At.Before(since):
		return ""
	}
	return tk.By
}
