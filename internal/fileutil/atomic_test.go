package fileutil_test

import (
	"os"
	"path/filepath"
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
