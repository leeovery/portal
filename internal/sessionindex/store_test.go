package sessionindex_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"golang.org/x/sys/unix"
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

// holdCommitLock takes the commit lock through an open file description of
// its own, as another process would.
func holdCommitLock(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := os.OpenFile(commitlock.Path(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock commit lock: %v", err)
	}
	return f
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
		if got := idx.Label("a"); got != "Alpha" {
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

		err := sessionindex.Update(dir, func(idx *sessionindex.Index) error {
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
		if got := idx.Label("a"); got != "Alpha" {
			t.Errorf("Label(a) = %q, want Alpha", got)
		}
	})

	t.Run("fn sees the index on disk", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","dir":"/a"},{"name":"b"}]}`)

		err := sessionindex.Update(dir, func(idx *sessionindex.Index) error {
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

		err := sessionindex.Update(dir, func(idx *sessionindex.Index) error {
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
		_ = sessionindex.Update(dir, func(*sessionindex.Index) error { return errors.New("boom") })
		if _, err := os.Stat(sessionindex.Path(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat sessions.json = %v, want absent", err)
		}
	})

	t.Run("an unreadable index is returned with fn never run and the file kept", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"version":1,"sessions":[]}`
		writeIndex(t, dir, seed)
		ran := false

		err := sessionindex.Update(dir, func(*sessionindex.Index) error { ran = true; return nil })
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

		if err := sessionindex.Update(dir, func(idx *sessionindex.Index) error { return idx.SetLabel("a", "A") }); err != nil {
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
				errs <- sessionindex.Update(dir, func(idx *sessionindex.Index) error {
					// The pause widens the read-modify-write window an
					// unserialised store would lose updates in.
					time.Sleep(2 * time.Millisecond)
					return idx.SetLabel(fmt.Sprintf("s%d", i), "label")
				})
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
		holdCommitLock(t, dir)
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)
		ran := false

		err := sessionindex.Update(dir, func(*sessionindex.Index) error { ran = true; return nil })
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
		held := holdCommitLock(t, dir)
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = held.Close()
		}()
		if err := sessionindex.Update(dir, func(*sessionindex.Index) error { return nil }); err != nil {
			t.Errorf("Update = %v, want it to wait for the release", err)
		}
	})
}

func TestUpdate_Logging(t *testing.T) {
	t.Run("each landed label change and removal logs at INFO", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Old"},{"name":"b"}]}`)
		sink := logtest.Install(t)

		err := sessionindex.Update(dir, func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("c", "Gamma")
			_ = idx.SetLabel("a", "")
			idx.Remove("b")
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		set := sink.Records().Matching("sessions", "set-label").AtExactLevel(slog.LevelInfo).Only(t, "set-label")
		if set.AttrString(t, "session") != "c" || set.AttrString(t, "value") != "Gamma" || set.AttrString(t, "op") != "set-label" {
			t.Errorf("set-label record = %+v", set.Attrs)
		}
		clear := sink.Records().Matching("sessions", "clear-label").AtExactLevel(slog.LevelInfo).Only(t, "clear-label")
		if clear.AttrString(t, "session") != "a" || clear.HasAttr("value") {
			t.Errorf("clear-label record = %+v", clear.Attrs)
		}
		rm := sink.Records().Matching("sessions", "rm").AtExactLevel(slog.LevelInfo).Only(t, "rm")
		if rm.AttrString(t, "session") != "b" {
			t.Errorf("rm record = %+v", rm.Attrs)
		}
	})

	t.Run("an unchanged label logs nothing", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","label":"Alpha"}]}`)
		sink := logtest.Install(t)

		if err := sessionindex.Update(dir, func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if n := len(sink.Records()); n != 0 {
			t.Errorf("records = %d, want none", n)
		}
	})

	t.Run("a change fn rolls back logs nothing", func(t *testing.T) {
		dir := t.TempDir()
		sink := logtest.Install(t)

		_ = sessionindex.Update(dir, func(idx *sessionindex.Index) error {
			_ = idx.SetLabel("a", "Alpha")
			return errors.New("boom")
		})
		if n := len(sink.Records()); n != 0 {
			t.Errorf("records = %d, want none", n)
		}
	})

	t.Run("a failed write logs WARN with its error class and logs no change", func(t *testing.T) {
		dir := t.TempDir()
		f, err := commitlock.Acquire(dir)
		if err != nil {
			t.Fatalf("seed commit lock: %v", err)
		}
		_ = f.Close()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		sink := logtest.Install(t)

		err = sessionindex.Update(dir, func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") })
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Fatalf("Update = %v, want a temp-create failure", err)
		}
		rec := sink.Records().Matching("sessions", "write").AtExactLevel(slog.LevelWarn).Only(t, "write failure")
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)
		if n := len(sink.Records().WithMessage("set-label")); n != 0 {
			t.Errorf("set-label records = %d, want none for a change that did not land", n)
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
