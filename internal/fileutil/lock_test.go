package fileutil_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"golang.org/x/sys/unix"
)

const lockPoll = 5 * time.Millisecond

func exclusive(wait time.Duration) fileutil.LockSpec {
	return fileutil.LockSpec{Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: wait, Poll: lockPoll}
}

// holdLock takes how's lock on path through an open file description of its
// own, as another process would, until the returned file is closed or the
// test ends.
func holdLock(t *testing.T, path string, how int) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), how|unix.LOCK_NB); err != nil {
		t.Fatalf("flock %s: %v", path, err)
	}
	return f
}

// lockFree reports whether an exclusive lock on path is grantable now.
func lockFree(t *testing.T, path string) bool {
	t.Helper()
	probe, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open probe: %v", err)
	}
	defer func() { _ = probe.Close() }()
	err = unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil && !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("probe flock: %v", err)
	}
	return err == nil
}

// openFDs is how many descriptors this process has open.
func openFDs(t *testing.T) int {
	t.Helper()
	fds, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(fds)
}

func lockStep(t *testing.T, err error) fileutil.LockStep {
	t.Helper()
	var lockErr *fileutil.LockError
	if !errors.As(err, &lockErr) {
		t.Fatalf("err = %v (%T), want a *fileutil.LockError", err, err)
	}
	return lockErr.Step
}

func TestLockWithin(t *testing.T) {
	t.Run("it creates the file at mode 0600 and holds the lock until the file is closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")

		f, err := fileutil.LockWithin(t.Context(), path, exclusive(time.Second))
		if err != nil {
			t.Fatalf("LockWithin: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("mode = %o, want 600", mode)
		}
		if lockFree(t, path) {
			t.Error("the lock is free while held")
		}
		_ = f.Close()
		if !lockFree(t, path) {
			t.Error("the lock is still held once the file is closed")
		}
	})

	t.Run("without O_CREATE a missing file is an open failure, and nothing is made", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")

		_, err := fileutil.LockWithin(t.Context(), path, fileutil.LockSpec{Flag: os.O_RDONLY, How: unix.LOCK_SH, Wait: time.Second, Poll: lockPoll})

		if step := lockStep(t, err); step != fileutil.LockOpen {
			t.Errorf("step = %v, want LockOpen", step)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("err = %v, want it to wrap fs.ErrNotExist", err)
		}
		if errors.Is(err, fileutil.ErrLockTimedOut) {
			t.Errorf("an open failure %v reads as the bound passing", err)
		}
		if _, statErr := os.Stat(path); !errors.Is(statErr, fs.ErrNotExist) {
			t.Errorf("stat = %v, want the file never made", statErr)
		}
	})

	t.Run("an open failure in a missing directory fails at once, naming the step and the path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "x.lock")

		start := time.Now()
		_, err := fileutil.LockWithin(t.Context(), path, exclusive(time.Minute))

		if step := lockStep(t, err); step != fileutil.LockOpen {
			t.Errorf("step = %v, want LockOpen", step)
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("gave up after %s, want at once", took)
		}
		if want := "open " + path + ": open " + path + ": no such file or directory"; err.Error() != want {
			t.Errorf("err = %q, want %q", err, want)
		}
	})

	t.Run("a lock held by another descriptor until the bound is the timed-out error, no sooner", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		holdLock(t, path, unix.LOCK_EX)
		const wait = 60 * time.Millisecond
		before := openFDs(t)

		start := time.Now()
		f, err := fileutil.LockWithin(t.Context(), path, exclusive(wait))
		took := time.Since(start)

		if f != nil {
			_ = f.Close()
			t.Fatal("LockWithin returned a file for a held lock")
		}
		if step := lockStep(t, err); step != fileutil.LockTimedOut {
			t.Errorf("step = %v, want LockTimedOut", step)
		}
		if !errors.Is(err, fileutil.ErrLockTimedOut) {
			t.Errorf("err = %v, want it to wrap ErrLockTimedOut", err)
		}
		if took < wait {
			t.Errorf("gave up after %s, want no sooner than the bound, %s", took, wait)
		}
		if want := path + ": lock held past the bound"; err.Error() != want {
			t.Errorf("err = %q, want %q", err, want)
		}
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors = %d after giving up, want %d: the lock file's left open", after, before)
		}
	})

	t.Run("it gives up only on a try at or after the bound, so a holder that lets go within it is waited out", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		held := holdLock(t, path, unix.LOCK_EX)
		// The holder lets go well inside the bound, and the first try after
		// the one that found it held comes only past the bound.
		spec := fileutil.LockSpec{Flag: os.O_RDWR, How: unix.LOCK_EX, Wait: 50 * time.Millisecond, Poll: 200 * time.Millisecond}
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = held.Close()
		}()

		f, err := fileutil.LockWithin(t.Context(), path, spec)
		if err != nil {
			t.Fatalf("LockWithin = %v, want the lock its holder let go of", err)
		}
		_ = f.Close()
	})

	t.Run("it waits for a holder that lets go within the bound", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		held := holdLock(t, path, unix.LOCK_EX)
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = held.Close()
		}()

		f, err := fileutil.LockWithin(t.Context(), path, exclusive(5*time.Second))
		if err != nil {
			t.Fatalf("LockWithin: %v", err)
		}
		_ = f.Close()
	})

	t.Run("an uncontended lock is granted whatever the bound and the context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		for _, wait := range []time.Duration{0, -time.Second} {
			path := filepath.Join(t.TempDir(), "x.lock")
			f, err := fileutil.LockWithin(ctx, path, exclusive(wait))
			if err != nil {
				t.Fatalf("LockWithin at a %s bound with an ended context: %v", wait, err)
			}
			_ = f.Close()
		}
	})

	t.Run("a context that ends while it waits fails with its cause, leaving the file closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		holdLock(t, path, unix.LOCK_EX)
		before := openFDs(t)
		cause := errors.New("the command ended")
		ctx, cancel := context.WithCancelCause(t.Context())
		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel(cause)
		}()

		start := time.Now()
		_, err := fileutil.LockWithin(ctx, path, exclusive(time.Minute))

		if step := lockStep(t, err); step != fileutil.LockCancelled {
			t.Errorf("step = %v, want LockCancelled", step)
		}
		if !errors.Is(err, cause) {
			t.Errorf("err = %v, want the context's cause", err)
		}
		if took := time.Since(start); took > 10*time.Second {
			t.Errorf("gave up after %s, want soon after the context ended", took)
		}
		if want := "wait for " + path + ": the command ended"; err.Error() != want {
			t.Errorf("err = %q, want %q", err, want)
		}
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors = %d after the context ended, want %d: the lock file's left open", after, before)
		}
	})

	t.Run("a flock refused for a reason other than a holder fails at once, not as the bound", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		before := openFDs(t)

		// No lock operation at all is one flock refuses outright.
		start := time.Now()
		_, err := fileutil.LockWithin(t.Context(), path, fileutil.LockSpec{Flag: os.O_RDWR | os.O_CREATE, How: 0, Wait: time.Minute, Poll: lockPoll})

		if step := lockStep(t, err); step != fileutil.LockFlock {
			t.Errorf("step = %v, want LockFlock", step)
		}
		var errno unix.Errno
		if !errors.As(err, &errno) || errno == unix.EWOULDBLOCK {
			t.Errorf("err = %v, want flock's own refusal", err)
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("gave up after %s, want at once", took)
		}
		if want := "flock " + path + ": " + errno.Error(); err.Error() != want {
			t.Errorf("err = %q, want %q", err, want)
		}
		if after := openFDs(t); after != before {
			t.Errorf("open descriptors = %d after the refusal, want %d: the lock file's left open", after, before)
		}
	})

	t.Run("a shared lock shares with a shared holder and waits on an exclusive one", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "x.lock")
		shared := fileutil.LockSpec{Flag: os.O_RDONLY, How: unix.LOCK_SH, Wait: 30 * time.Millisecond, Poll: lockPoll}

		holdLock(t, path, unix.LOCK_SH)
		f, err := fileutil.LockWithin(t.Context(), path, shared)
		if err != nil {
			t.Fatalf("LockWithin beside a shared holder: %v", err)
		}
		_ = f.Close()

		other := filepath.Join(t.TempDir(), "y.lock")
		holdLock(t, other, unix.LOCK_EX)
		if _, err := fileutil.LockWithin(t.Context(), other, shared); !errors.Is(err, fileutil.ErrLockTimedOut) {
			t.Errorf("LockWithin beside an exclusive holder = %v, want the bound passing", err)
		}
	})
}
