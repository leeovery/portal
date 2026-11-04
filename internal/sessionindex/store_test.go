package sessionindex_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func writeIndex(t *testing.T, dir string, data string) {
	t.Helper()
	if err := os.WriteFile(sessionindex.Path(dir), []byte(data), 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

func readFile(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(sessionindex.Path(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return data
}

func TestPath(t *testing.T) {
	if got, want := sessionindex.Path("/s"), filepath.Join("/s", "sessions.json"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestRead(t *testing.T) {
	t.Run("a missing file is an empty version 2 index", func(t *testing.T) {
		idx, err := sessionindex.Read(t.TempDir())
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if idx.Version != sessionindex.SchemaVersion || idx.Sessions == nil || len(idx.Sessions) != 0 {
			t.Errorf("index = %+v, want empty version 2", idx)
		}
	})

	t.Run("it decodes the file on disk", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Alpha"}]}`)
		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if got := labelOf(idx, "a"); got != "Alpha" {
			t.Errorf("Label(a) = %q, want Alpha", got)
		}
	})

	t.Run("a version 1 file is ErrLegacyIndex", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":1,"sessions":[]}`)
		if _, err := sessionindex.Read(dir); !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("Read = %v, want ErrLegacyIndex", err)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("it writes fn's changes and stamps saved_at from the clock", func(t *testing.T) {
		dir := t.TempDir()
		at := time.Date(2026, 11, 2, 9, 30, 0, 0, time.FixedZone("x", 3600))
		sessionindex.SetClockForTest(t, at)

		_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel("a", "Alpha")
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if !idx.SavedAt.Equal(at) || idx.SavedAt.Location() != time.UTC {
			t.Errorf("saved_at = %v, want %v in UTC", idx.SavedAt, at)
		}
		if got := labelOf(idx, "a"); got != "Alpha" {
			t.Errorf("Label(a) = %q, want Alpha", got)
		}
	})

	t.Run("fn sees the index on disk", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","dir":"/a"},{"name":"b"}]}`)

		_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.Remove("b")
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if want := []sessionindex.Entry{{Name: "a", Dir: "/a"}}; !slices.Equal(idx.Sessions, want) {
			t.Errorf("sessions = %+v, want %+v", idx.Sessions, want)
		}
	})

	t.Run("nothing is written when fn errors", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"version":2,"sessions":[{"name":"a"}]}`
		writeIndex(t, dir, seed)
		boom := errors.New("boom")

		_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("a", "Alpha")
			return boom
		})
		if !errors.Is(err, boom) {
			t.Errorf("Update = %v, want fn's error", err)
		}
		if got := readFile(t, dir); string(got) != seed {
			t.Errorf("sessions.json = %s, want untouched %s", got, seed)
		}
	})

	t.Run("nothing is written for a missing file when fn errors", func(t *testing.T) {
		dir := t.TempDir()
		_, _ = sessionindex.Update(dir, "cli", func(*sessionindex.Index) error { return errors.New("boom") })
		if _, err := os.Stat(sessionindex.Path(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat sessions.json = %v, want absent", err)
		}
	})

	t.Run("an unreadable index is returned with fn never run and the file kept", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"version":1,"sessions":[]}`
		writeIndex(t, dir, seed)
		ran := false

		_, err := sessionindex.Update(dir, "cli", func(*sessionindex.Index) error { ran = true; return nil })
		if !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("Update = %v, want ErrLegacyIndex", err)
		}
		if ran {
			t.Errorf("fn ran over an unreadable index")
		}
		if got := readFile(t, dir); string(got) != seed {
			t.Errorf("sessions.json = %s, want untouched %s", got, seed)
		}
	})

	t.Run("it replaces the file atomically, leaving no temp file", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[]}`)
		before, err := os.Stat(sessionindex.Path(dir))
		if err != nil {
			t.Fatalf("stat: %v", err)
		}

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error { return idx.SetLabel("a", "A") }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		after, err := os.Stat(sessionindex.Path(dir))
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if before.Sys().(*syscall.Stat_t).Ino == after.Sys().(*syscall.Stat_t).Ino {
			t.Errorf("sessions.json kept its inode: written in place, not renamed over")
		}
		if perm := after.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %o, want 600", perm)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if want := []string{"commit.lock", "sessions.json"}; !slices.Equal(names, want) {
			t.Errorf("state dir = %v, want %v", names, want)
		}
	})

	t.Run("concurrent callers are serialised and lose no update", func(t *testing.T) {
		dir := t.TempDir()
		const callers = 20
		var wg sync.WaitGroup
		errs := make(chan error, callers)
		for i := range callers {
			wg.Go(func() {
				_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
					// The pause widens the read-modify-write window an
					// unserialised store would lose updates in.
					time.Sleep(2 * time.Millisecond)
					return idx.SetLabel(fmt.Sprintf("s%d", i), "label")
				})
				errs <- err
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("Update: %v", err)
			}
		}
		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if len(idx.Sessions) != callers {
			t.Errorf("sessions = %d, want %d", len(idx.Sessions), callers)
		}
	})

	t.Run("a lock held past the bound returns ErrHeld with nothing read or written", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"version":2,"sessions":[]}`
		writeIndex(t, dir, seed)
		commitlock.HoldForTest(t, dir)
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)
		ran := false

		_, err := sessionindex.Update(dir, "cli", func(*sessionindex.Index) error { ran = true; return nil })
		if !errors.Is(err, commitlock.ErrHeld) {
			t.Errorf("Update = %v, want ErrHeld", err)
		}
		if ran {
			t.Errorf("fn ran without the lock")
		}
		if got := readFile(t, dir); !bytes.Equal(got, []byte(seed)) {
			t.Errorf("sessions.json = %s, want untouched", got)
		}
	})

	t.Run("it waits out a holder that releases within the bound", func(t *testing.T) {
		dir := t.TempDir()
		held := commitlock.HoldForTest(t, dir)
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = held.Close()
		}()
		if _, err := sessionindex.Update(dir, "cli", func(*sessionindex.Index) error { return nil }); err != nil {
			t.Errorf("Update = %v, want it to wait for the release", err)
		}
	})

	t.Run("a fn that changes nothing writes nothing and keeps saved_at", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"version":2,"saved_at":"2026-01-01T00:00:00Z","sessions":[{"name":"a","label":"Alpha"}]}`
		writeIndex(t, dir, seed)
		sessionindex.SetClockForTest(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))

		_, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			if err := idx.SetLabel("a", "Alpha"); err != nil {
				return err
			}
			if err := idx.SetLabel("absent", ""); err != nil {
				return err
			}
			idx.Remove("absent")
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if got := readFile(t, dir); string(got) != seed {
			t.Errorf("sessions.json = %s, want untouched %s", got, seed)
		}
	})

	t.Run("a fn that changes nothing creates no file", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := sessionindex.Update(dir, "cli", func(*sessionindex.Index) error { return nil }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if _, err := os.Stat(sessionindex.Path(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat sessions.json = %v, want absent", err)
		}
	})
}

// sessionindex is the store internal/state will read labels from, so it may
// never import state or anything that reaches tmux.
func TestSessionindexPackage_ImportsOnlyLeaves(t *testing.T) {
	const prefix = "github.com/leeovery/portal/"
	allowed := []string{prefix + "internal/commitlock", prefix + "internal/fileutil", prefix + "internal/log"}
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, prefix+"internal/sessionindex", allowed, lane)
	}
}
