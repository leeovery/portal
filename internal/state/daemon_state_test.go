package state_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

func TestPIDFile(t *testing.T) {
	t.Run("writes and reads a PID file", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WritePIDFile(dir, 4321); err != nil {
			t.Fatalf("WritePIDFile: %v", err)
		}

		got, err := state.ReadPIDFile(dir)
		if err != nil {
			t.Fatalf("ReadPIDFile: %v", err)
		}
		if got != 4321 {
			t.Errorf("ReadPIDFile = %d; want 4321", got)
		}
	})

	t.Run("returns ErrPIDFileAbsent when file is missing", func(t *testing.T) {
		dir := t.TempDir()
		got, err := state.ReadPIDFile(dir)
		if !errors.Is(err, state.ErrPIDFileAbsent) {
			t.Fatalf("ReadPIDFile err = %v; want ErrPIDFileAbsent", err)
		}
		if got != 0 {
			t.Errorf("ReadPIDFile pid = %d; want 0 on absent", got)
		}
	})

	t.Run("returns error when PID file is unparseable", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte("not-a-number\n"), 0o600); err != nil {
			t.Fatalf("write bad pid file: %v", err)
		}

		got, err := state.ReadPIDFile(dir)
		if err == nil {
			t.Fatalf("ReadPIDFile err = nil; want parse error")
		}
		if errors.Is(err, state.ErrPIDFileAbsent) {
			t.Errorf("ReadPIDFile err = ErrPIDFileAbsent; want a parse error")
		}
		if got != 0 {
			t.Errorf("ReadPIDFile pid = %d; want 0 on parse error", got)
		}
	})

	t.Run("trims whitespace when reading the PID file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte("  1234  \n"), 0o600); err != nil {
			t.Fatalf("write padded pid file: %v", err)
		}

		got, err := state.ReadPIDFile(dir)
		if err != nil {
			t.Fatalf("ReadPIDFile: %v", err)
		}
		if got != 1234 {
			t.Errorf("ReadPIDFile = %d; want 1234", got)
		}
	})

	t.Run("writes PID file with mode 0600", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WritePIDFile(dir, 1); err != nil {
			t.Fatalf("WritePIDFile: %v", err)
		}

		info, err := os.Stat(filepath.Join(dir, "daemon.pid"))
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("daemon.pid mode = %o; want 0600", got)
		}
	})
}

func TestIsProcessAlive(t *testing.T) {
	t.Run("reports current process as alive", func(t *testing.T) {
		if !state.IsProcessAlive(os.Getpid()) {
			t.Errorf("IsProcessAlive(os.Getpid()) = false; want true")
		}
	})

	t.Run("reports a clearly-unused PID as dead", func(t *testing.T) {
		// 99999999 is well above typical PID ranges and effectively guaranteed
		// to be unused. Using an unused PID is a deterministic substitute for
		// the "freshly-reaped child" case, which is racy because the OS may
		// recycle PIDs.
		if state.IsProcessAlive(99999999) {
			t.Errorf("IsProcessAlive(99999999) = true; want false")
		}
	})

	t.Run("reports a freshly-reaped child as dead in the common case", func(t *testing.T) {
		bin, err := exec.LookPath("true")
		if err != nil {
			t.Skipf("cannot locate true binary: %v", err)
		}
		cmd := exec.Command(bin)
		if err := cmd.Start(); err != nil {
			t.Skipf("cannot spawn %s: %v", bin, err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("cmd.Wait: %v", err)
		}
		// After Wait, the kernel has reaped the child. signal(0) should
		// return ESRCH unless the OS has very quickly recycled the PID; we
		// accept the latter as a known-flaky case rather than failing.
		if state.IsProcessAlive(cmd.Process.Pid) {
			t.Logf("IsProcessAlive(%d) = true after reap; PID may have been recycled", cmd.Process.Pid)
		}
	})

	t.Run("reports invalid PIDs as dead", func(t *testing.T) {
		cases := []int{0, -1, -1234}
		for _, pid := range cases {
			if state.IsProcessAlive(pid) {
				t.Errorf("IsProcessAlive(%d) = true; want false", pid)
			}
		}
	})
}

func TestDaemonAlive(t *testing.T) {
	t.Run("returns true when both PID file and process exist", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WritePIDFile(dir, os.Getpid()); err != nil {
			t.Fatalf("WritePIDFile: %v", err)
		}
		if !state.DaemonAlive(dir) {
			t.Errorf("DaemonAlive = false; want true with our own PID")
		}
	})

	t.Run("returns false when PID file is absent", func(t *testing.T) {
		dir := t.TempDir()
		if state.DaemonAlive(dir) {
			t.Errorf("DaemonAlive = true; want false when PID file absent")
		}
	})

	t.Run("returns false when PID file points to a dead process", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WritePIDFile(dir, os.Getpid()+1_000_000); err != nil {
			t.Fatalf("WritePIDFile: %v", err)
		}
		if state.DaemonAlive(dir) {
			t.Errorf("DaemonAlive = true; want false when PID is unused")
		}
	})
}

func TestVersionFile(t *testing.T) {
	t.Run("writes and reads a version file", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteVersionFile(dir, "1.2.3", nil); err != nil {
			t.Fatalf("WriteVersionFile: %v", err)
		}

		got, err := state.ReadVersionFile(dir)
		if err != nil {
			t.Fatalf("ReadVersionFile: %v", err)
		}
		if got != "1.2.3" {
			t.Errorf("ReadVersionFile = %q; want %q", got, "1.2.3")
		}
	})

	t.Run("returns ErrVersionFileAbsent when file is missing", func(t *testing.T) {
		dir := t.TempDir()
		got, err := state.ReadVersionFile(dir)
		if !errors.Is(err, state.ErrVersionFileAbsent) {
			t.Fatalf("ReadVersionFile err = %v; want ErrVersionFileAbsent", err)
		}
		if got != "" {
			t.Errorf("ReadVersionFile = %q; want \"\" on absent", got)
		}
	})

	t.Run("distinguishes empty contents from absent file", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteVersionFile(dir, "", nil); err != nil {
			t.Fatalf("WriteVersionFile(\"\"): %v", err)
		}

		got, err := state.ReadVersionFile(dir)
		if err != nil {
			t.Fatalf("ReadVersionFile: %v", err)
		}
		if got != "" {
			t.Errorf("ReadVersionFile = %q; want \"\"", got)
		}
	})

	t.Run("round-trips the literal dev marker", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteVersionFile(dir, "dev", nil); err != nil {
			t.Fatalf("WriteVersionFile: %v", err)
		}

		got, err := state.ReadVersionFile(dir)
		if err != nil {
			t.Fatalf("ReadVersionFile: %v", err)
		}
		if got != "dev" {
			t.Errorf("ReadVersionFile = %q; want \"dev\"", got)
		}
	})

	t.Run("writes version file with mode 0600", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.WriteVersionFile(dir, "0.0.1", nil); err != nil {
			t.Fatalf("WriteVersionFile: %v", err)
		}

		info, err := os.Stat(filepath.Join(dir, "daemon.version"))
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("daemon.version mode = %o; want 0600", got)
		}
	})
}
