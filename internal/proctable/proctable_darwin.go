package proctable

import (
	"fmt"
	"os"
	"unsafe"

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
		list = append(list, processOf(p))
	}
	return list, nil
}

// Read reads pid, from the kernel as it is called, as List lists this user's
// processes. Unlike List, it filters by no uid: another user's pid reads too.
// It fails with ErrGone for a process that has exited or is exiting, a zombie
// among them.
func Read(pid int) (Process, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	switch {
	case err != nil:
		return Process{}, fmt.Errorf("read process %d: %w", pid, err)
	case goneFrom(procs):
		return Process{}, fmt.Errorf("read process %d: %w", pid, ErrGone)
	}
	return processOf(procs[0]), nil
}

func processOf(p unix.KinfoProc) Process {
	return Process{
		PID:             int(p.Proc.P_pid),
		PPID:            int(p.Eproc.Ppid),
		Group:           int(p.Eproc.Pgid),
		Terminal:        p.Eproc.Tdev != noDevice,
		ForegroundGroup: int(p.Eproc.Tpgid),
		Gone:            goneFrom([]unix.KinfoProc{p}),
		Command:         unix.ByteSliceToString(p.Proc.P_comm[:]),
	}
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

// goneFrom reports whether a kern.proc.pid listing shows a process gone:
// exited, or exiting.
func goneFrom(procs []unix.KinfoProc) bool {
	return exitedFrom(procs) || procs[0].Proc.P_flag&exiting != 0
}

// Exited reports whether pid has finished exiting: the kernel no longer lists
// it, or lists it a zombie, which holds no descriptor and no lock. A process
// still exiting has not.
func Exited(pid int) (bool, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if err != nil {
		return false, fmt.Errorf("read process %d: %w", pid, err)
	}
	return exitedFrom(procs), nil
}

// exitedFrom reports whether a kern.proc.pid listing shows a process that
// has finished exiting: not listed, or a zombie.
func exitedFrom(procs []unix.KinfoProc) bool {
	return len(procs) == 0 || procs[0].Proc.P_stat == zombie
}

// Parent is pid's parent, read from the kernel as it is called.
func Parent(pid int) (int, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, fmt.Errorf("read process %d: %w", pid, err)
	}
	return int(p.Eproc.Ppid), nil
}

// proc_info's call and flavour for a process's working and root directories:
// PROC_INFO_CALL_PIDINFO and PROC_PIDVNODEPATHINFO.
const (
	procInfoCallPIDInfo  = 2
	procPIDVnodePathInfo = 9
)

// vnodeInfoSize is the size of a struct vnode_info, which comes before each
// path in a struct proc_vnodepathinfo.
const vnodeInfoSize = 152

// vnodePathInfo is a struct proc_vnodepathinfo: two struct vnode_info_path,
// the working directory's then the root's, each a struct vnode_info and a
// NUL-terminated path of up to MAXPATHLEN bytes.
type vnodePathInfo [2 * (vnodeInfoSize + unix.PathMax)]byte

// Cwd reads pid's current working directory, from the kernel as it is
// called. It fails with ErrGone for a process that has exited or is exiting,
// a zombie among them, and otherwise for another user's.
func Cwd(pid int) (string, error) {
	var info vnodePathInfo
	// x/sys wraps no proc_info and the module has no cgo for libproc's
	// proc_pidinfo, so this traps into the kernel by syscall number, which
	// Apple doesn't promise to keep. Proven by experiment on macOS 27 to read
	// a live process's directory and refuse launchd's with EPERM; TestCwd
	// holds it to that on every run.
	n, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallPIDInfo, uintptr(pid), procPIDVnodePathInfo, 0, uintptr(unsafe.Pointer(&info[0])), uintptr(len(info))) //nolint:staticcheck // no libSystem wrapper exists without cgo; the direct call is proven by experiment, above

	switch {
	case errno != 0 && gone(pid):
		return "", fmt.Errorf("read the directory of process %d: %w: %w", pid, ErrGone, errno)
	case errno != 0:
		return "", fmt.Errorf("read the directory of process %d: %w", pid, errno)
	case int(n) != len(info):
		return "", fmt.Errorf("read the directory of process %d: the kernel answered %d bytes, want %d", pid, n, len(info))
	}
	return unix.ByteSliceToString(info[vnodeInfoSize : vnodeInfoSize+unix.PathMax]), nil
}
