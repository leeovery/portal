package commitlock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"golang.org/x/sys/unix"
)

func TestPath(t *testing.T) {
	if got, want := commitlock.Path("/s"), filepath.Join("/s", "commit.lock"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestAcquire(t *testing.T) {
	t.Run("it creates the lock file and holds it exclusively until closed", func(t *testing.T) {
		dir := t.TempDir()
		f, err := commitlock.Acquire(dir)
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		probe, err := os.OpenFile(commitlock.Path(dir), os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("open probe: %v", err)
		}
		defer func() { _ = probe.Close() }()
		if err := unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB); !errors.Is(err, unix.EWOULDBLOCK) {
			t.Fatalf("probe flock while held = %v, want EWOULDBLOCK", err)
		}
		_ = f.Close()
		if err := unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			t.Errorf("probe flock after close = %v, want granted", err)
		}
	})

	t.Run("a lock held past the bound returns an error wrapping ErrHeld", func(t *testing.T) {
		dir := t.TempDir()
		commitlock.HoldForTest(t, dir)
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)

		f, err := commitlock.Acquire(dir)
		if f != nil {
			_ = f.Close()
			t.Fatal("Acquire returned a file for a held lock")
		}
		if !errors.Is(err, commitlock.ErrHeld) {
			t.Errorf("error = %v, want ErrHeld", err)
		}
	})

	t.Run("it waits for a holder that releases within the bound", func(t *testing.T) {
		dir := t.TempDir()
		held := commitlock.HoldForTest(t, dir)
		commitlock.SetTimeoutForTest(t, 5*time.Second)
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = held.Close()
		}()

		f, err := commitlock.Acquire(dir)
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		_ = f.Close()
	})

	t.Run("an open failure is not contention", func(t *testing.T) {
		_, err := commitlock.Acquire(filepath.Join(t.TempDir(), "missing"))
		if err == nil {
			t.Fatal("Acquire in a missing dir succeeded")
		}
		if errors.Is(err, commitlock.ErrHeld) {
			t.Errorf("open failure %v reads as ErrHeld", err)
		}
	})
}

// A test holding the lock beside a process that commits in the same dir, such
// as a saver daemon, finds it held now and then.
func TestHoldForTest_WaitsOutAHolder(t *testing.T) {
	dir := t.TempDir()
	other, err := commitlock.Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = other.Close()
	}()

	held := commitlock.HoldForTest(t, dir)

	probe, err := os.OpenFile(commitlock.Path(dir), os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open probe: %v", err)
	}
	defer func() { _ = probe.Close() }()
	if err := unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB); !errors.Is(err, unix.EWOULDBLOCK) {
		t.Errorf("probe flock while held = %v, want EWOULDBLOCK", err)
	}
	_ = held.Close()
}

func TestCommitlockPackage_ImportsOnlyFileutil(t *testing.T) {
	allowed := []string{"github.com/leeovery/portal/internal/fileutil"}
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, "github.com/leeovery/portal/internal/commitlock", allowed, lane)
	}
}
