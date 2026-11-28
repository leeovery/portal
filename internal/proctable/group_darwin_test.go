package proctable_test

import (
	"os"
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/proctest"
)

func TestListTellsAProcessGroupAndItsTerminalsForegroundGroup(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })
	// sh leads a session on the pty, and so its own group; with job control
	// on it runs sleep as a foreground job in a group of the job's own, which
	// the terminal's foreground then is. The trailing : keeps sh from
	// exec'ing sleep in its place.
	cmd := exec.Command("/bin/sh", "-c", "set -m; /bin/sleep 5; :")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	held := proctest.Start(t, cmd, "")
	_ = tty.Close()
	pid := held.PID()

	var sh, job proctable.Process
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		list, err := proctable.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			switch {
			case p.PID == pid:
				sh = p
			case p.PPID == pid && p.Command == "sleep":
				job = p
			}
		}
		if job.PID != 0 && sh.ForegroundGroup == job.Group {
			break
		}
	}
	if job.PID != 0 {
		// The job, alone in its group, is outside the group the held sh
		// leads, which its release kills, so it is ended here, while sh is
		// still held and so proves it its own.
		t.Cleanup(func() { _ = held.Signal(job.PID, syscall.SIGKILL) })
	}

	if sh.Group != pid || job.PID == 0 || job.Group != job.PID || sh.ForegroundGroup != job.Group || sh.Group == sh.ForegroundGroup {
		t.Errorf("sh %d: Group %d, ForegroundGroup %d; its job %d: Group %d; want sh in its own group and the job's group, apart from it, the foreground", pid, sh.Group, sh.ForegroundGroup, job.PID, job.Group)
	}
	list, err := proctable.List()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(list, func(p proctable.Process) bool { return p.PID == os.Getpid() })
	if i < 0 {
		t.Fatalf("List lacks this process %d", os.Getpid())
	}
	if got := list[i].Group; got != syscall.Getpgrp() {
		t.Errorf("this process: Group %d, want %d", got, syscall.Getpgrp())
	}
}
