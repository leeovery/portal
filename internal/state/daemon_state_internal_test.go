package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadDaemonFile(t *testing.T) {
	sentinel := errors.New("sentinel-absent")

	t.Run("returns absent sentinel when file is missing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "missing.txt")

		data, err := readDaemonFile(path, sentinel)
		if !errors.Is(err, sentinel) {
			t.Fatalf("readDaemonFile err = %v; want sentinel", err)
		}
		if data != nil {
			t.Errorf("readDaemonFile data = %v; want nil on absent", data)
		}
	})

	t.Run("wraps other I/O errors with read <basename> prefix", func(t *testing.T) {
		dir := t.TempDir()
		// A directory cannot be read as a regular file — os.ReadFile returns a
		// non-ENOENT error here, exercising the generic-error branch.
		path := filepath.Join(dir, "subdir")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		data, err := readDaemonFile(path, sentinel)
		if err == nil {
			t.Fatalf("readDaemonFile err = nil; want a wrapped I/O error")
		}
		if errors.Is(err, sentinel) {
			t.Errorf("readDaemonFile err = sentinel; want a wrapped I/O error")
		}
		if data != nil {
			t.Errorf("readDaemonFile data = %v; want nil on error", data)
		}
		want := "read subdir:"
		if got := err.Error(); len(got) < len(want) || got[:len(want)] != want {
			t.Errorf("readDaemonFile err = %q; want prefix %q", got, want)
		}
	})

	t.Run("returns raw bytes on success", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ok.txt")
		payload := []byte("  hello  \n")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		data, err := readDaemonFile(path, sentinel)
		if err != nil {
			t.Fatalf("readDaemonFile: %v", err)
		}
		if string(data) != string(payload) {
			t.Errorf("readDaemonFile data = %q; want %q (no trim at helper level)", data, payload)
		}
	})
}
