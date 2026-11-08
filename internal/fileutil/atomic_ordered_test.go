package fileutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
)

// syncRecorder records each sync AtomicWrite makes, with what target held at
// that moment, then makes the real sync.
type syncRecorder struct {
	t      *testing.T
	target string
	events []string
}

func (r *syncRecorder) file(f *os.File) error {
	data, err := os.ReadFile(f.Name())
	if err != nil {
		r.t.Fatalf("read the temp file being synced: %v", err)
	}
	r.events = append(r.events, "sync temp file holding "+string(data)+", target holding "+r.held())
	return f.Sync()
}

func (r *syncRecorder) dir(dir string) error {
	r.events = append(r.events, "sync "+dir+", target holding "+r.held())
	return nil
}

func (r *syncRecorder) held() string {
	data, err := os.ReadFile(r.target)
	if err != nil {
		return "nothing"
	}
	return string(data)
}

func TestAtomicWrite_Ordered(t *testing.T) {
	t.Run("it syncs the temp file before the rename and the directory after it", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "sessions.json")
		mustWrite(t, target, "old")
		rec := &syncRecorder{t: t, target: target}
		fileutil.SetSyncsForTest(t, rec.file, rec.dir)

		if err := fileutil.AtomicWrite(target, []byte("new")); err != nil {
			t.Fatalf("AtomicWrite: %v", err)
		}

		want := []string{
			"sync temp file holding new, target holding old",
			"sync " + dir + ", target holding new",
		}
		if !slices.Equal(rec.events, want) {
			t.Errorf("syncs = %q\nwant %q", rec.events, want)
		}
	})

	t.Run("the directory synced is the one the rename landed in, through a symlink", func(t *testing.T) {
		linkDir, realDir := t.TempDir(), t.TempDir()
		real := filepath.Join(realDir, "hooks.json")
		mustWrite(t, real, "old")
		mustSymlink(t, real, filepath.Join(linkDir, "hooks.json"))
		var synced []string
		fileutil.SetSyncsForTest(t, (*os.File).Sync, func(dir string) error { synced = append(synced, dir); return nil })

		if err := fileutil.AtomicWrite(filepath.Join(linkDir, "hooks.json"), []byte("new")); err != nil {
			t.Fatalf("AtomicWrite: %v", err)
		}

		if !slices.Equal(synced, []string{realDir}) {
			t.Errorf("synced %v, want %s alone", synced, realDir)
		}
	})

	t.Run("a temp file that fails its sync with EIO fails the write with ErrWriteFsync, renaming nothing", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "sessions.json")
		mustWrite(t, target, "old")
		dirSynced := false
		fileutil.SetSyncsForTest(t, func(*os.File) error { return syscall.EIO }, func(string) error { dirSynced = true; return nil })

		err := fileutil.AtomicWrite(target, []byte("new"))

		if !errors.Is(err, fileutil.ErrWriteFsync) || !errors.Is(err, syscall.EIO) {
			t.Errorf("AtomicWrite = %v, want ErrWriteFsync wrapping EIO", err)
		}
		requireContent(t, target, "old")
		requireNoTempFiles(t, dir)
		if dirSynced {
			t.Error("the directory was synced after a failed write")
		}
	})

	t.Run("a directory that fails its sync after the rename leaves the write landed, reported to ReportUnsynced", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "sessions.json")
		mustWrite(t, target, "old")
		fileutil.FailDirSyncForTest(t, syscall.EIO)
		var reported []error

		err := fileutil.AtomicWrite0600(target, []byte("new"), fileutil.ReportUnsynced(func(err error) { reported = append(reported, err) }))

		if err != nil {
			t.Errorf("AtomicWrite = %v, want nil: the rename landed", err)
		}
		requireContent(t, target, "new")
		requireNoTempFiles(t, dir)
		if len(reported) != 1 || !errors.Is(reported[0], fileutil.ErrWriteFsync) || !errors.Is(reported[0], syscall.EIO) {
			t.Errorf("reported = %v, want one ErrWriteFsync wrapping EIO", reported)
		}
	})

	t.Run("a directory that fails its sync after the rename leaves the write landed with no one to report to", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "hooks.json")
		fileutil.FailDirSyncForTest(t, syscall.EIO)

		if err := fileutil.AtomicWrite(target, []byte("new")); err != nil {
			t.Errorf("AtomicWrite = %v, want nil: the rename landed", err)
		}
		requireContent(t, target, "new")
	})
}

// BenchmarkAtomicWrite measures what ordering costs a write of a
// sessions.json-sized file, against the same write without its syncs.
func BenchmarkAtomicWrite(b *testing.B) {
	data := make([]byte, 4096)
	for name, nosync := range map[string]bool{"with its syncs": false, "without syncs": true} {
		b.Run(name, func(b *testing.B) {
			if nosync {
				fileutil.SetSyncsForTest(b, func(*os.File) error { return nil }, func(string) error { return nil })
			}
			path := filepath.Join(b.TempDir(), "sessions.json")
			for b.Loop() {
				if err := fileutil.AtomicWrite(path, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
