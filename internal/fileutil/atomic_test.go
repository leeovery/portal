package fileutil_test

import (
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
		// A permissive umask would normally let the temp file be created with
		// broader-than-0600 bits; the helper must defensively chmod the
		// final path so its mode does not depend on the caller's umask.
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
		// A path whose parent cannot be created (a regular file masquerading
		// as a directory) should propagate the AtomicWrite error verbatim.
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
