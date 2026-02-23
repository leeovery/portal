package browser_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/leeovery/portal/internal/browser"
)

func TestListDirectories(t *testing.T) {
	t.Run("returns only directories", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "alpha"))
		mustMkdir(t, filepath.Join(dir, "beta"))
		mustCreateFile(t, filepath.Join(dir, "readme.txt"))
		mustCreateFile(t, filepath.Join(dir, "main.go"))

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{"alpha", "beta"})
		for _, e := range entries {
			if e.IsSymlink {
				t.Errorf("entry %q should not be a symlink", e.Name)
			}
		}
	})

	t.Run("excludes hidden when showHidden is false", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "visible"))
		mustMkdir(t, filepath.Join(dir, ".hidden"))

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{"visible"})
	})

	t.Run("includes hidden when showHidden is true", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "visible"))
		mustMkdir(t, filepath.Join(dir, ".hidden"))

		entries, err := browser.ListDirectories(dir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{".hidden", "visible"})
	})

	t.Run("returns empty for empty directory", func(t *testing.T) {
		dir := t.TempDir()

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if entries == nil {
			t.Fatal("expected non-nil empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("handles permission denied gracefully", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("permission tests not reliable on Windows")
		}

		dir := t.TempDir()
		restricted := filepath.Join(dir, "restricted")
		mustMkdir(t, restricted)
		if err := os.Chmod(restricted, 0o000); err != nil {
			t.Fatalf("failed to chmod: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(restricted, 0o755) // best-effort restore for cleanup
		})

		entries, err := browser.ListDirectories(restricted, false)
		if err != nil {
			t.Fatalf("expected no error on permission denied, got: %v", err)
		}

		if entries == nil {
			t.Fatal("expected non-nil empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("sorts entries alphabetically", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "cherry"))
		mustMkdir(t, filepath.Join(dir, "apple"))
		mustMkdir(t, filepath.Join(dir, "banana"))

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{"apple", "banana", "cherry"})
	})

	t.Run("includes symlinked directories", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "realdir")
		mustMkdir(t, target)
		link := filepath.Join(dir, "linkdir")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{"linkdir", "realdir"})

		for _, e := range entries {
			if e.Name == "linkdir" {
				if !e.IsSymlink {
					t.Errorf("entry %q should be marked as symlink", e.Name)
				}
			}
			if e.Name == "realdir" {
				if e.IsSymlink {
					t.Errorf("entry %q should not be marked as symlink", e.Name)
				}
			}
		}
	})

	t.Run("excludes symlinked files", func(t *testing.T) {
		dir := t.TempDir()
		mustMkdir(t, filepath.Join(dir, "realdir"))
		targetFile := filepath.Join(dir, "realfile.txt")
		mustCreateFile(t, targetFile)
		linkFile := filepath.Join(dir, "linkfile")
		if err := os.Symlink(targetFile, linkFile); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		entries, err := browser.ListDirectories(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertNames(t, entries, []string{"realdir"})
	})
}

// mustMkdir creates a directory, failing the test if it cannot.
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("failed to create directory %q: %v", path, err)
	}
}

// mustCreateFile creates an empty file, failing the test if it cannot.
func mustCreateFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file %q: %v", path, err)
	}
	_ = f.Close()
}

// assertNames checks that the entries have exactly the expected names in order.
func assertNames(t *testing.T, entries []browser.DirEntry, want []string) {
	t.Helper()
	if len(entries) != len(want) {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name
		}
		t.Fatalf("got %d entries %v, want %d entries %v", len(entries), names, len(want), want)
	}
	for i, e := range entries {
		if e.Name != want[i] {
			t.Errorf("entry[%d].Name = %q, want %q", i, e.Name, want[i])
		}
	}
}
