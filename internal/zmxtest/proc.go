package zmxtest

import (
	"fmt"
	"os"
	"slices"
	"syscall"

	"github.com/leeovery/portal/internal/proctable"
)

// procReader reads this user's processes as each read is made: List, and a
// process's args and parent. A proof of ownership reads a process's parent
// only after its args, from the kernel as it then stands, so a pid reused
// between the two reads never borrows the parent its last holder had.
type procReader interface {
	List() ([]proctable.Process, error)
	Args(pid int) (proctable.Args, error)
	Parent(pid int) (int, error)
}

type kernelProcs struct{}

func (kernelProcs) List() ([]proctable.Process, error)   { return proctable.List() }
func (kernelProcs) Args(pid int) (proctable.Args, error) { return proctable.ReadArgs(pid) }
func (kernelProcs) Parent(pid int) (int, error)          { return proctable.Parent(pid) }

// processes is the reader every proof reads through; tests replace it.
var processes procReader = kernelProcs{}

// ownedBy reports whether pid is a process started for dir: its environment
// sets ZMX_DIR to exactly dir, or its parent chain reaches one whose does. The
// Backend sets ZMX_DIR on every zmx run, a daemon keeps the environment of the
// run it forked from, and a session's processes inherit the daemon's, but
// macOS hides a platform binary's environment, /bin/sh's among them, from
// kern.procargs2.
func ownedBy(pid int, dir string) bool {
	return ownedThrough(pid, dir, processes)
}

// ownedThrough walks from pid up its parents to one whose environment proves
// dir, reading each process's args, then its parent, and then checks the
// chain it walked still stands. A walk that reaches pid 1, fails a read or
// repeats a pid proves nothing, so an orphan reparented to launchd is never
// owned: it leaks rather than risk a wrong kill.
func ownedThrough(pid int, dir string, r procReader) bool {
	var chain []int
	for pid > 1 && !slices.Contains(chain, pid) {
		chain = append(chain, pid)
		a, err := r.Args(pid)
		if err != nil {
			return false
		}
		if slices.Contains(a.Env, "ZMX_DIR="+dir) {
			return standing(chain, r)
		}
		if pid, err = r.Parent(pid); err != nil {
			return false
		}
	}
	return false
}

// standing reports whether each process of chain still has the next as its
// parent, read after every args read the walk made. A process whose parent
// exits is reparented and never gets that parent back, so a match proves the
// parent outlived the read of its args: its pid was not taken by another
// process, a test's own zmx among them, that the read then proved.
func standing(chain []int, r procReader) bool {
	for i := 0; i+1 < len(chain); i++ {
		if parent, err := r.Parent(chain[i]); err != nil || parent != chain[i+1] {
			return false
		}
	}
	return true
}

// signalOwned sends sig to pid if it is a process started for dir, proved as
// the signal is sent: a pid proved earlier may since have been reused.
func signalOwned(pid int, dir string, sig syscall.Signal) error {
	if !ownedBy(pid, dir) {
		return fmt.Errorf("pid %d is no process of %s, so it is not signalled", pid, dir)
	}
	return syscall.Kill(pid, sig)
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
