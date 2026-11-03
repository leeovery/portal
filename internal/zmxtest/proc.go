package zmxtest

import (
	"fmt"
	"os"
	"slices"

	"github.com/leeovery/portal/internal/proctable"
)

// ownedBy reports whether pid is a process started for dir: its environment
// sets ZMX_DIR to exactly dir, or its parent chain reaches one whose does. The
// Backend sets ZMX_DIR on every zmx run, a daemon keeps the environment of the
// run it forked from, and a session's processes inherit the daemon's, but
// macOS hides a platform binary's environment, /bin/sh's among them, from
// kern.procargs2. It reads the process table afresh.
func ownedBy(pid int, dir string) bool {
	procs, err := proctable.List()
	if err != nil {
		return false
	}
	return ownedThrough(pid, dir, proctable.ReadArgs, parentsIn(procs))
}

// parentsIn reads each pid's parent from procs, one reading of this user's
// processes. A pid it doesn't hold, one gone or another user's, has no parent
// to read.
func parentsIn(procs []proctable.Process) func(pid int) (int, error) {
	parents := make(map[int]int, len(procs))
	for _, p := range procs {
		parents[p.PID] = p.PPID
	}
	return func(pid int) (int, error) {
		ppid, ok := parents[pid]
		if !ok {
			return 0, fmt.Errorf("process %d is not among this user's", pid)
		}
		return ppid, nil
	}
}

// ownedThrough walks from pid up its parents to one whose environment, read
// afresh, proves dir. A walk that reaches pid 1, fails a read or repeats
// a pid proves nothing, so an orphan reparented to launchd is never owned: it
// leaks rather than risk a wrong kill.
func ownedThrough(pid int, dir string, args func(int) (proctable.Args, error), parent func(int) (int, error)) bool {
	seen := map[int]bool{}
	for pid > 1 && !seen[pid] {
		seen[pid] = true
		a, err := args(pid)
		if err != nil {
			return false
		}
		if slices.Contains(a.Env, "ZMX_DIR="+dir) {
			return true
		}
		if pid, err = parent(pid); err != nil {
			return false
		}
	}
	return false
}

// owned lists the processes started for dir.
func owned(dir string) ([]int, error) {
	procs, err := proctable.List()
	if err != nil {
		return nil, err
	}
	parent := parentsIn(procs)
	var mine []int
	for _, p := range procs {
		if p.PID != os.Getpid() && ownedThrough(p.PID, dir, proctable.ReadArgs, parent) {
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
