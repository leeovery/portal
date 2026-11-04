package zmxtest

import (
	"fmt"
	"os"
	"slices"
	"syscall"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/proctest"
	"golang.org/x/sys/unix"
)

// processes is the reader every proof reads through; tests replace it.
var processes proctest.Reader = proctest.Kernel{}

// groupOf reads the process group a pid is in; tests replace it.
var groupOf = unix.Getpgid

// ownedBy reports whether pid is a process started for dir: its environment
// sets ZMX_DIR to exactly dir, or its parent chain reaches one whose does. The
// Backend sets ZMX_DIR on every zmx run, a daemon keeps the environment of the
// run it forked from, and a session's processes inherit the daemon's, but
// macOS hides a platform binary's environment, /bin/sh's among them, from
// kern.procargs2.
func ownedBy(pid int, dir string) bool {
	return ownedThrough(pid, dir, processes)
}

// ownedThrough is ownedBy read through r.
func ownedThrough(pid int, dir string, r proctest.Reader) bool {
	return proctest.Proved(pid, proctest.EnvProof("ZMX_DIR="+dir), r)
}

// signalOwned sends sig to pid if it is a process started for dir, proved as
// the signal is sent: a pid proved earlier may since have been reused.
func signalOwned(pid int, dir string, sig syscall.Signal) error {
	if !ownedBy(pid, dir) {
		return fmt.Errorf("pid %d is no process of %s, so it is not signalled", pid, dir)
	}
	return syscall.Kill(pid, sig)
}

// signalGroupOwned sends sig to the group pgid leads if pgid is a process
// started for dir that leads its own group, proved as the signal is sent.
func signalGroupOwned(pgid int, dir string, sig syscall.Signal) error {
	if pgid <= 1 || !ownedBy(pgid, dir) {
		return fmt.Errorf("pid %d is no process of %s, so its group is not signalled", pgid, dir)
	}
	if group, err := groupOf(pgid); err != nil || group != pgid {
		return fmt.Errorf("pid %d leads no process group of its own, so none is signalled", pgid)
	}
	return syscall.Kill(-pgid, sig)
}

// owned lists the processes started for dir. The listing only names the pids
// to prove: each proof reads afresh.
func owned(dir string) ([]int, error) {
	procs, err := processes.List()
	if err != nil {
		return nil, err
	}
	var mine []int
	for _, p := range procs {
		if p.PID != os.Getpid() && ownedBy(p.PID, dir) {
			mine = append(mine, p.PID)
		}
	}
	return mine, nil
}

// checkProcessReader proves proctable reads this process's own argv, so an
// empty owned list means no process, not a reader that sees none.
func checkProcessReader() error {
	if _, err := proctable.List(); err != nil {
		return err
	}
	args, err := proctable.ReadArgs(os.Getpid())
	if err != nil {
		return fmt.Errorf("read this process's arguments: %w", err)
	}
	if !slices.Equal(args.Argv, os.Args) {
		return fmt.Errorf("read this process's argv as %q, want %q", args.Argv, os.Args)
	}
	return nil
}
