package proctable

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// noDevice is the controlling terminal of a process with none.
const noDevice = -1

// List lists this user's processes.
func List() ([]Process, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	list := make([]Process, 0, len(procs))
	for _, p := range procs {
		list = append(list, Process{
			PID:      int(p.Proc.P_pid),
			PPID:     int(p.Eproc.Ppid),
			Terminal: p.Eproc.Tdev != noDevice,
		})
	}
	return list, nil
}

// ReadArgs reads pid's argv and environment. It fails for a process that has
// exited, a zombie among them, and for another user's.
func ReadArgs(pid int) (Args, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return Args{}, fmt.Errorf("read arguments of process %d: %w", pid, err)
	}
	return parseArgs(raw)
}

// Parent is pid's parent.
func Parent(pid int) (int, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, fmt.Errorf("read process %d: %w", pid, err)
	}
	return int(p.Eproc.Ppid), nil
}
