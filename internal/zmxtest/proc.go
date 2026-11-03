package zmxtest

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
)

var errProcArgs = errors.New("malformed kern.procargs2")

// procArgs is a process's argv and environment as the kernel keeps them from
// its exec. A process that forked without exec, as a zmx daemon does, keeps
// its parent's.
type procArgs struct {
	argv []string
	env  []string
}

// parseProcArgs reads kern.procargs2: argc as a native int32, the
// executable's path, NUL padding, argc NUL-terminated arguments, then the
// environment's NUL-terminated entries, up to an empty one or the end. What
// follows the environment is the loader's, and never names ZMX_DIR.
func parseProcArgs(raw []byte) (procArgs, error) {
	if len(raw) < 4 {
		return procArgs{}, fmt.Errorf("%w: %d bytes", errProcArgs, len(raw))
	}
	argc := int(int32(binary.NativeEndian.Uint32(raw)))
	if argc < 0 {
		return procArgs{}, fmt.Errorf("%w: argc %d", errProcArgs, argc)
	}
	rest := raw[4:]
	path := bytes.IndexByte(rest, 0)
	if path < 0 {
		return procArgs{}, fmt.Errorf("%w: unterminated executable path", errProcArgs)
	}
	rest = bytes.TrimLeft(rest[path:], "\x00")
	var args procArgs
	for range argc {
		end := bytes.IndexByte(rest, 0)
		if end < 0 {
			return procArgs{}, fmt.Errorf("%w: %d of %d arguments", errProcArgs, len(args.argv), argc)
		}
		args.argv = append(args.argv, string(rest[:end]))
		rest = rest[end+1:]
	}
	for {
		end := bytes.IndexByte(rest, 0)
		if end <= 0 {
			return args, nil
		}
		args.env = append(args.env, string(rest[:end]))
		rest = rest[end+1:]
	}
}

// ownedBy reports whether pid is a process started for dir: its environment
// sets ZMX_DIR to exactly dir, or its parent chain reaches one whose does. The
// Backend sets ZMX_DIR on every zmx run, a daemon keeps the environment of the
// run it forked from, and a session's processes inherit the daemon's, but
// macOS hides a platform binary's environment, /bin/sh's among them, from
// kern.procargs2.
func ownedBy(pid int, dir string) bool {
	return ownedThrough(pid, dir, processArgs, parentOf)
}

// ownedThrough walks from pid up its parents, read afresh, to one whose
// environment proves dir. A walk that reaches pid 1, fails a read or repeats
// a pid proves nothing, so an orphan reparented to launchd is never owned: it
// leaks rather than risk a wrong kill.
func ownedThrough(pid int, dir string, args func(int) (procArgs, error), parent func(int) (int, error)) bool {
	seen := map[int]bool{}
	for pid > 1 && !seen[pid] {
		seen[pid] = true
		a, err := args(pid)
		if err != nil {
			return false
		}
		if slices.Contains(a.env, "ZMX_DIR="+dir) {
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
	pids, err := processes()
	if err != nil {
		return nil, err
	}
	var mine []int
	for _, pid := range pids {
		if pid != os.Getpid() && ownedBy(pid, dir) {
			mine = append(mine, pid)
		}
	}
	return mine, nil
}

// checkProcessReader proves processArgs reads this process's own argv, so an
// empty owned list means no process, not a reader that sees none.
func checkProcessReader() error {
	if _, err := processes(); err != nil {
		return err
	}
	args, err := processArgs(os.Getpid())
	if err != nil {
		return fmt.Errorf("read this process's arguments: %w", err)
	}
	if !slices.Equal(args.argv, os.Args) {
		return fmt.Errorf("read this process's argv as %q, want %q", args.argv, os.Args)
	}
	return nil
}
