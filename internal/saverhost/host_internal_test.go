package saverhost

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestRunningBy(t *testing.T) {
	unreadable := func(int) (bool, error) { return false, errors.New("sysctl kern.proc.pid: EPERM") }

	t.Run("the kernel's table answers when it can be read", func(t *testing.T) {
		exited := func(int) (bool, error) { return true, nil }
		if runningBy(exited, os.Getpid()) {
			t.Error("runningBy = true, want the table's exited over kill's answer")
		}
	})

	t.Run("where the table can't be read, kill(pid, 0) finds a live process running", func(t *testing.T) {
		if !runningBy(unreadable, os.Getpid()) {
			t.Error("runningBy(this process) = false, want it running")
		}
	})

	t.Run("where the table can't be read, kill(pid, 0) finds a reaped process gone", func(t *testing.T) {
		child := exec.Command("/usr/bin/true")
		if err := child.Run(); err != nil {
			t.Fatal(err)
		}
		if runningBy(unreadable, child.Process.Pid) {
			t.Errorf("runningBy(the reaped child %d) = true, want it gone", child.Process.Pid)
		}
	})
}
