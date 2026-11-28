package proctable_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/proctest"
)

func TestCwd(t *testing.T) {
	t.Run("it reads the directory a process works in, as the kernel resolves it", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "a dir·x")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("/bin/sleep", "30")
		cmd.Dir = dir
		held := proctest.Start(t, cmd, "")

		if got, err := proctable.Cwd(held.PID()); err != nil || got != want {
			t.Errorf("Cwd(%d) = %q, %v; want %q", held.PID(), got, err, want)
		}
	})

	t.Run("a zombie is gone", func(t *testing.T) {
		held := proctest.Start(t, exec.Command("/usr/bin/true"), "")
		select {
		case <-held.Exited():
		case <-time.After(10 * time.Second):
			t.Fatal("true never exited")
		}

		if got, err := proctable.Cwd(held.PID()); !errors.Is(err, proctable.ErrGone) {
			t.Errorf("Cwd(zombie) = %q, %v; want ErrGone", got, err)
		}
	})

	t.Run("a reaped process is gone", func(t *testing.T) {
		child := exec.Command("/usr/bin/true")
		if err := child.Run(); err != nil {
			t.Fatal(err)
		}

		if got, err := proctable.Cwd(child.Process.Pid); !errors.Is(err, proctable.ErrGone) {
			t.Errorf("Cwd(reaped) = %q, %v; want ErrGone", got, err)
		}
	})

	t.Run("another user's running process is unreadable, and not gone", func(t *testing.T) {
		if got, err := proctable.Cwd(1); err == nil || errors.Is(err, proctable.ErrGone) {
			t.Errorf("Cwd(launchd) = %q, %v; want an error other than ErrGone", got, err)
		}
	})
}
