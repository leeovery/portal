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

// watchFailures collects what a watch reports from its goroutine. Read it only
// once the watch has finished.
type watchFailures struct{ errs []error }

func (f *watchFailures) record(err error) { f.errs = append(f.errs, err) }

func noWatchFailure(t *testing.T) func(error) {
	return func(err error) { t.Errorf("the watch failed: %v", err) }
}

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

		w, err := holdExit(pid, noWatchFailure(t))
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

	// kevent refuses a child as it starts to exit, a moment before the process
	// table shows it a zombie. Where that moment falls after Start depends on
	// the machine's load, so each run holds the child a little later than the
	// last, across the few milliseconds /usr/bin/false takes to exit; measured
	// unloaded, the sweep lands a few dozen runs in the window.
	t.Run("a child caught as it exits is held as exited, every time", func(t *testing.T) {
		const runs, step = 500, 8 * time.Microsecond
		for run := range runs {
			cmd := exec.Command("/usr/bin/false")
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			spinFor(time.Duration(run) * step)
			w, err := holdExit(cmd.Process.Pid, noWatchFailure(t))
			if err != nil {
				_ = cmd.Wait()
				t.Fatalf("run %d: holdExit: %v", run, err)
			}
			awaitClosed(t, w.done, "the exiting child's watch")
			w.stop()
			_ = cmd.Wait()
			if w.err != nil || w.status.ExitStatus() != 1 {
				t.Fatalf("run %d: watch = %v, %v; want exit 1", run, w.status, w.err)
			}
		}
	})
}

// spinFor busy-waits rather than sleeps: a sleep this short wakes when the
// scheduler next looks, not after d.
func spinFor(d time.Duration) {
	for end := time.Now().Add(d); time.Now().Before(end); {
	}
}

func TestExitWatch_Await(t *testing.T) {
	t.Run("a kevent that fails ends the watch failed, and fails the test", func(t *testing.T) {
		var failures watchFailures
		w := newExitWatch(failures.record)
		failing := func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			return 0, syscall.EBADF
		}

		go w.await(-1, failing)

		awaitClosed(t, w.done, "the failed watch")
		<-w.finished
		if !errors.Is(w.err, syscall.EBADF) {
			t.Errorf("watch error = %v, want the kevent's EBADF", w.err)
		}
		if len(failures.errs) != 1 || !errors.Is(failures.errs[0], syscall.EBADF) {
			t.Errorf("the watch reported %v, want the kevent's EBADF once", failures.errs)
		}
	})

	t.Run("a stopped watch ends with no exit reported and nothing failed", func(t *testing.T) {
		var failures watchFailures
		w := newExitWatch(failures.record)
		quiet := func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			time.Sleep(time.Millisecond)
			return 0, nil
		}
		go w.await(-1, quiet)

		w.stop()

		select {
		case <-w.finished:
		default:
			t.Fatal("stop returned while the watch was still waiting")
		}
		select {
		case <-w.done:
			t.Error("a stopped watch closed done, which reads as the process having exited")
		default:
		}
		if len(failures.errs) != 0 {
			t.Errorf("a stopped watch reported %v", failures.errs)
		}
	})

	t.Run("an interrupted kevent waits on", func(t *testing.T) {
		w := newExitWatch(noWatchFailure(t))
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
		w.stop()
		if w.err != nil || w.status.ExitStatus() != 3 {
			t.Errorf("watch = %v, %v; want exit 3 after the interruption", w.status, w.err)
		}
	})
}
