package fileutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/fileutil"
)

func TestAtomicWrite(t *testing.T) {
	t.Run("writes data to file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "test.json")

		data := []byte(`{"key": "value"}`)
		if err := fileutil.AtomicWrite(filePath, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		if string(got) != string(data) {
			t.Errorf("file content = %q, want %q", string(got), string(data))
		}
	})

	t.Run("creates parent directories if missing", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "a", "b", "c")
		filePath := filepath.Join(nested, "test.json")

		data := []byte(`{"nested": true}`)
		if err := fileutil.AtomicWrite(filePath, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(nested)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file")
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if string(got) != string(data) {
			t.Errorf("file content = %q, want %q", string(got), string(data))
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "test.json")

		if err := os.WriteFile(filePath, []byte("old content"), 0o644); err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		newData := []byte("new content")
		if err := fileutil.AtomicWrite(filePath, newData); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if string(got) != string(newData) {
			t.Errorf("file content = %q, want %q", string(got), string(newData))
		}
	})

	t.Run("leaves no temp files on success", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "test.json")

		if err := fileutil.AtomicWrite(filePath, []byte("data")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}

		for _, entry := range entries {
			if entry.Name() != "test.json" {
				t.Errorf("unexpected file in directory: %s", entry.Name())
			}
		}
	})

	t.Run("writes empty data", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "empty.json")

		if err := fileutil.AtomicWrite(filePath, []byte{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("file content length = %d, want 0", len(got))
		}
	})
}

func TestAtomicWrite0600(t *testing.T) {
	t.Run("writes data with mode 0600 regardless of umask", func(t *testing.T) {
		// The final mode must not depend on the caller's umask.
		old := syscall.Umask(0)
		t.Cleanup(func() { syscall.Umask(old) })

		dir := t.TempDir()
		filePath := filepath.Join(dir, "secret.bin")

		data := []byte("sensitive")
		if err := fileutil.AtomicWrite0600(filePath, data); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if string(got) != string(data) {
			t.Errorf("file content = %q, want %q", string(got), string(data))
		}

		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("file mode = %#o, want %#o", mode, 0o600)
		}
	})

	t.Run("creates parent directories if missing", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "a", "b")
		filePath := filepath.Join(nested, "secret.bin")

		if err := fileutil.AtomicWrite0600(filePath, []byte("x")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("file mode = %#o, want %#o", mode, 0o600)
		}
	})

	t.Run("propagates AtomicWrite errors", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed blocker: %v", err)
		}

		filePath := filepath.Join(blocker, "child", "secret.bin")
		if err := fileutil.AtomicWrite0600(filePath, []byte("x")); err == nil {
			t.Fatalf("expected error when parent cannot be created; got nil")
		}
	})
}

// A config file kept in a dotfiles repository is linked into place, and a write
// must land on the file the link names while the link stays a link.
func TestAtomicWrite_WritesThroughSymlinks(t *testing.T) {
	t.Run("writes the linked file and keeps the link", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "repo", "prefs.json")
		link := filepath.Join(dir, "config", "prefs.json")
		mustWrite(t, real, "old")
		mustSymlink(t, real, link)

		if err := fileutil.AtomicWrite(link, []byte("new")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		requireLinkTo(t, link, real)
		requireContent(t, real, "new")
		requireNoTempFiles(t, filepath.Dir(real), filepath.Dir(link))
	})

	t.Run("follows a chain of relative links", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "repo", "prefs.json")
		mid := filepath.Join(dir, "repo", "current.json")
		link := filepath.Join(dir, "config", "prefs.json")
		mustWrite(t, real, "old")
		mustSymlink(t, "prefs.json", mid)
		mustSymlink(t, filepath.Join("..", "repo", "current.json"), link)

		if err := fileutil.AtomicWrite(link, []byte("new")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		requireLinkTo(t, link, filepath.Join("..", "repo", "current.json"))
		requireLinkTo(t, mid, "prefs.json")
		requireContent(t, real, "new")
	})

	t.Run("a dangling link creates the file it names", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "repo", "nested", "prefs.json")
		link := filepath.Join(dir, "config", "prefs.json")
		mustSymlink(t, real, link)

		if err := fileutil.AtomicWrite(link, []byte("new")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		requireLinkTo(t, link, real)
		requireContent(t, real, "new")
	})

	t.Run("reads a relative link from the directory it physically sits in", func(t *testing.T) {
		// config/ is itself a link to repo/home/, so the relative ../shared/prefs.json
		// names repo/shared/prefs.json, not the lexical sibling of config/.
		dir := t.TempDir()
		real := filepath.Join(dir, "repo", "shared", "prefs.json")
		mustWrite(t, real, "old")
		home := filepath.Join(dir, "repo", "home")
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		mustSymlink(t, filepath.Join("..", "shared", "prefs.json"), filepath.Join(home, "prefs.json"))
		mustSymlink(t, home, filepath.Join(dir, "config"))

		if err := fileutil.AtomicWrite(filepath.Join(dir, "config", "prefs.json"), []byte("new")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		requireContent(t, real, "new")
		if _, err := os.Lstat(filepath.Join(dir, "shared")); !os.IsNotExist(err) {
			t.Errorf("the write created %s, the lexical reading of the link; want nothing there (err %v)", filepath.Join(dir, "shared"), err)
		}
	})

	t.Run("a link cycle fails and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		a := filepath.Join(dir, "a.json")
		b := filepath.Join(dir, "b.json")
		mustSymlink(t, b, a)
		mustSymlink(t, a, b)

		err := fileutil.AtomicWrite(a, []byte("new"))
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Fatalf("error = %v, want one classed %v", err, fileutil.ErrWriteTempCreate)
		}

		requireLinkTo(t, a, b)
		requireLinkTo(t, b, a)
		requireNoTempFiles(t, dir)
	})

	t.Run("AtomicWrite0600 sets the mode on the linked file", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "repo", "hooks.json")
		link := filepath.Join(dir, "config", "hooks.json")
		mustWrite(t, real, "old")
		mustSymlink(t, real, link)

		if err := fileutil.AtomicWrite0600(link, []byte("new")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		requireLinkTo(t, link, real)
		requireContent(t, real, "new")
		info, err := os.Stat(real)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("linked file mode = %#o, want %#o", mode, 0o600)
		}
	})
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
}

func mustSymlink(t *testing.T, dest, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(dest, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, dest, err)
	}
}

func requireLinkTo(t *testing.T, link, dest string) {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is a %v, want the link left in place", link, info.Mode().Type())
	}
	if got, err := os.Readlink(link); err != nil || got != dest {
		t.Errorf("%s -> %q (err %v), want -> %q", link, got, err, dest)
	}
}

func requireContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func requireNoTempFiles(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, ".atomic-*.tmp"))
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		if len(matches) > 0 {
			t.Errorf("temp files left behind in %s: %v", dir, matches)
		}
	}
}
