package proctable

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// noDevice is the controlling terminal of a process with none.
const noDevice = -1

// zombie is the p_stat of a process that has exited and not been reaped.
const zombie = 5

// exiting is the p_flag bit of a process working on exiting: P_WEXIT.
const exiting = 0x2000

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
			Command:  unix.ByteSliceToString(p.Proc.P_comm[:]),
		})
	}
	return list, nil
}

// ReadArgs reads pid's argv and environment. It fails with ErrGone for a
// process that has exited or is exiting, a zombie among them, and otherwise
// for another user's, or for one caught mid-exec.
func ReadArgs(pid int) (Args, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		if gone(pid) {
			err = fmt.Errorf("%w: %w", ErrGone, err)
		}
		return Args{}, fmt.Errorf("read arguments of process %d: %w", pid, err)
	}
	return parseArgs(raw)
}

// gone reports whether pid has exited or is exiting: the kernel answers
// EINVAL for kern.procargs2 of a process it no longer lists, of a zombie and
// of one on its way to being one alike.
func gone(pid int) bool {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	return err == nil && goneFrom(procs)
}

// goneFrom reports whether a kern.proc.pid listing shows a process gone: not
// listed, a zombie, or exiting.
func goneFrom(procs []unix.KinfoProc) bool {
	return len(procs) == 0 || procs[0].Proc.P_stat == zombie || procs[0].Proc.P_flag&exiting != 0
}
