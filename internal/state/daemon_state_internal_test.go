package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

	t.Run("preserves the *os.PathError so errors.Is(fs.ErrPermission) traverses the %w wrap", func(t *testing.T) {
		// Boundary class 3 contract: a non-ENOENT os.ReadFile failure must wrap
		// with %w so the underlying *os.PathError stays reachable. A 0o000 file
		// yields EACCES → errors.Is(err, fs.ErrPermission) must traverse the
		// "read <basename>: %w" wrap. (An errors.New or %s wrap would drop it.)
		if runtime.GOOS == "windows" {
			t.Skip("0o000 perm semantics differ on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root bypasses 0o000 mode bits")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "locked.txt")
		if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatalf("chmod 0o000: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

		data, err := readDaemonFile(path, sentinel)
		if err == nil {
			t.Fatal("readDaemonFile err = nil; want a permission error")
		}
		if errors.Is(err, sentinel) {
			t.Errorf("readDaemonFile err = sentinel; want a wrapped permission error")
		}
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("errors.Is(err, fs.ErrPermission) = false; *os.PathError dropped? err = %v", err)
		}
		if data != nil {
			t.Errorf("readDaemonFile data = %v; want nil on error", data)
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
