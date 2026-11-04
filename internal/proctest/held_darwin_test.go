package proctest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
)

const heldWait = 10 * time.Second

// orphanEnv names the file the orphan helper writes its pid to; its presence
// makes TestOrphanHelperProcess the helper rather than a test.
const orphanEnv = "PROCTEST_ORPHAN_PIDFILE"

// TestOrphanHelperProcess is not a test: it is a binary of the test's own,
// whose environment the kernel shows, left running once its parent exits.
func TestOrphanHelperProcess(t *testing.T) {
	pidfile := os.Getenv(orphanEnv)
	if pidfile == "" {
		return
	}
	_ = os.WriteFile(pidfile, []byte(strconv.Itoa(os.Getpid())), 0o600)
	time.Sleep(heldWait)
}

func awaitPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	if !harnesstest.PollUntil(t, heldWait, 10*time.Millisecond, func() bool {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil
	}) {
		t.Fatalf("no pid at %s", path)
	}
	return pid
}

func awaitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(heldWait):
		t.Fatalf("%s never exited", what)
	}
}

func TestHeld(t *testing.T) {
	t.Run("it reports an exit and its status without reaping the process", func(t *testing.T) {
		cmd := exec.Command("/bin/sh", "-c", "read line; exit 3")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		h := Start(t, cmd, "")

		_ = stdin.Close()

		awaitClosed(t, h.Exited(), "the held shell")
		if got := h.Status(); !got.Exited() || got.ExitStatus() != 3 {
			t.Errorf("status = %v, want exit 3", got)
		}
		if cmd.ProcessState != nil {
			t.Error("the held shell was reaped before the test's cleanup")
		}
		// Signal 0 sends nothing; it reaches a zombie, never a reaped pid.
		if err := syscall.Kill(h.PID(), 0); err != nil {
			t.Errorf("the exited shell's pid is gone (%v): it was reaped, freeing it for reuse", err)
		}
	})

	t.Run("it signals a process below it and watches it exit", func(t *testing.T) {
		dir := t.TempDir()
		pidfile := filepath.Join(dir, "child")
		h := Start(t, exec.Command("/bin/sh", "-c", `sleep 30 & echo $! > "$0"; wait`, pidfile), "")
		child := awaitPID(t, pidfile)
		ended := h.Watch(t, child)

		if err := h.Signal(child, syscall.SIGTERM); err != nil {
			t.Fatalf("Signal(%d): %v", child, err)
		}

		awaitClosed(t, ended, "the held shell's child")
	})

	t.Run("it refuses, sending nothing, a process it does not prove", func(t *testing.T) {
		h := Start(t, exec.Command("/bin/sh", "-c", "sleep 30"), "")

		// Signal 0 sends nothing even were the refusal to fail.
		if err := h.Signal(os.Getpid(), 0); err == nil {
			t.Error("Signal of the test's own process succeeded; it is no process below the held one")
		}
	})

	t.Run("it proves a binary of the test's own by its marker once its parent is gone", func(t *testing.T) {
		bin, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		pidfile := filepath.Join(dir, "orphan")
		marker := orphanEnv + "=" + pidfile
		cmd := exec.Command("/bin/sh", "-c", `"$0" -test.run='^TestOrphanHelperProcess$' >/dev/null 2>&1 &`, bin)
		cmd.Env = append(os.Environ(), marker)
		marked := Start(t, cmd, marker)
		awaitClosed(t, marked.Exited(), "the orphan's parent")
		orphan := awaitPID(t, pidfile)
		ended := marked.Watch(t, orphan)

		unmarked := Start(t, exec.Command("/bin/sh", "-c", "sleep 30"), "")
		if err := unmarked.Signal(orphan, 0); err == nil {
			t.Error("a held process with no marker proved an orphan it never started")
		}
		if err := marked.Signal(orphan, syscall.SIGKILL); err != nil {
			t.Fatalf("Signal(%d) by its marker: %v", orphan, err)
		}

		awaitClosed(t, ended, "the orphan")
	})

	t.Run("it signals the group it leads", func(t *testing.T) {
		dir := t.TempDir()
		pidfile := filepath.Join(dir, "child")
		cmd := exec.Command("/bin/sh", "-c", `trap "" TERM; sleep 30 & echo $! > "$0"; wait`, pidfile)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		h := Start(t, cmd, "")
		child := awaitPID(t, pidfile)
		ended := h.Watch(t, child)

		if err := h.SignalGroup(syscall.SIGKILL); err != nil {
			t.Fatalf("SignalGroup: %v", err)
		}

		awaitClosed(t, ended, "the group's sleep")
		awaitClosed(t, h.Exited(), "the group's leader")
	})

	t.Run("its cleanup returns once every process of the group it leads has exited", func(t *testing.T) {
		var pgid int
		t.Run("a leader leaving a process in its group", func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", "sleep 30 &")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			h := Start(t, cmd, "")
			pgid = h.PID()
			awaitClosed(t, h.Exited(), "the group's leader")
		})

		// Signal 0 sends nothing; the id can't pass on while the group stands.
		if err := syscall.Kill(-pgid, 0); err != syscall.ESRCH {
			t.Errorf("group %d after cleanup: %v, want ESRCH: a process of it outlived the cleanup", pgid, err)
		}
	})

	t.Run("it refuses a group signal from a process leading no group", func(t *testing.T) {
		h := Start(t, exec.Command("/bin/sh", "-c", "sleep 30"), "")

		if err := h.SignalGroup(0); err == nil {
			t.Error("SignalGroup succeeded for a process started in the test's own group")
		}
	})
}
