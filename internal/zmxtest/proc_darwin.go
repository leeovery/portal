package zmxtest

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// processes lists this user's processes.
func processes() ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	pids := make([]int, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, int(p.Proc.P_pid))
	}
	return pids, nil
}

// processArgs fails for a process that has exited, a zombie among them.
func processArgs(pid int) (procArgs, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return procArgs{}, fmt.Errorf("read arguments of process %d: %w", pid, err)
	}
	return parseProcArgs(raw)
}

func parentOf(pid int) (int, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, fmt.Errorf("read process %d: %w", pid, err)
	}
	return int(p.Eproc.Ppid), nil
}
