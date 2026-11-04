package proctest

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"golang.org/x/sys/unix"
)

func TestHoldExit(t *testing.T) {
	t.Run("a child already exited is held as exited, with its status, and left unreaped", func(t *testing.T) {
		cmd := exec.Command("/bin/sh", "-c", "exit 3")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Wait() })
		pid := cmd.Process.Pid
		if !harnesstest.PollUntil(t, heldWait, 5*time.Millisecond, func() bool {
			p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
			return err == nil && p.Proc.P_stat == szomb
		}) {
			t.Fatalf("pid %d never became a zombie", pid)
		}

		w, err := holdExit(pid)
		if err != nil {
			t.Fatalf("holdExit of a zombie: %v", err)
		}

		awaitClosed(t, w.done, "the zombie's watch")
		if w.err != nil || !w.status.Exited() || w.status.ExitStatus() != 3 {
			t.Errorf("watch = %v, %v; want exit 3", w.status, w.err)
		}
		// Signal 0 sends nothing; it reaches a zombie, never a reaped pid.
		if err := syscall.Kill(pid, 0); err != nil {
			t.Errorf("the zombie's pid is gone (%v): it was reaped", err)
		}
	})
}

func TestExitWatch_Await(t *testing.T) {
	t.Run("a kevent that fails ends the watch with its error", func(t *testing.T) {
		w := newExitWatch()
		failing := func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			return 0, syscall.EBADF
		}

		go w.await(-1, failing)

		awaitClosed(t, w.done, "the failed watch")
		if !errors.Is(w.err, syscall.EBADF) {
			t.Errorf("watch error = %v, want the kevent's EBADF", w.err)
		}
	})

	t.Run("a stopped watch ends, marked as stopped", func(t *testing.T) {
		w := newExitWatch()
		quiet := func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			time.Sleep(time.Millisecond)
			return 0, nil
		}
		go w.await(-1, quiet)

		w.stop()

		awaitClosed(t, w.done, "the stopped watch")
		if !errors.Is(w.err, errWatchStopped) {
			t.Errorf("watch error = %v, want errWatchStopped", w.err)
		}
	})

	t.Run("an interrupted kevent waits on", func(t *testing.T) {
		w := newExitWatch()
		calls := 0
		interrupted := func(_ int, _ []unix.Kevent_t, events []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			calls++
			if calls == 1 {
				return 0, syscall.EINTR
			}
			events[0].Data = 3 << 8
			return 1, nil
		}

		go w.await(-1, interrupted)

		awaitClosed(t, w.done, "the interrupted watch")
		if w.err != nil || w.status.ExitStatus() != 3 {
			t.Errorf("watch = %v, %v; want exit 3 after the interruption", w.status, w.err)
		}
	})
}
