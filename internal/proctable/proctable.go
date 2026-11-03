// Package proctable reads this user's processes from the kernel: which are
// running, their parents, whether each has a controlling terminal, and the
// argv and environment each was exec'd with. It reads them on darwin alone;
// elsewhere every read fails with ErrUnsupported.
package proctable

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrUnsupported is every read on a system the package can't read.
var ErrUnsupported = errors.New("the process table is read only on darwin")

// errMalformed is a kern.procargs2 buffer that doesn't parse.
var errMalformed = errors.New("malformed kern.procargs2")

// Process is one of this user's processes as the kernel lists it.
type Process struct {
	PID, PPID int
	// Terminal reports whether the process has a controlling terminal.
	Terminal bool
}

// Args is a process's argv and environment as the kernel keeps them from its
// exec. A process that forked without exec, as a zmx daemon does, keeps its
// parent's. macOS hides a platform binary's environment, /bin/sh's among
// them, so Env is empty for one.
type Args struct {
	Argv []string
	Env  []string
}

// parseArgs reads kern.procargs2: argc as a native int32, the executable's
// path, NUL padding, argc NUL-terminated arguments, then the environment's
// NUL-terminated entries, up to an empty one or the end. What follows the
// environment is the loader's.
func parseArgs(raw []byte) (Args, error) {
	if len(raw) < 4 {
		return Args{}, fmt.Errorf("%w: %d bytes", errMalformed, len(raw))
	}
	argc := int(int32(binary.NativeEndian.Uint32(raw)))
	if argc < 0 {
		return Args{}, fmt.Errorf("%w: argc %d", errMalformed, argc)
	}
	rest := raw[4:]
	path := bytes.IndexByte(rest, 0)
	if path < 0 {
		return Args{}, fmt.Errorf("%w: unterminated executable path", errMalformed)
	}
	rest = bytes.TrimLeft(rest[path:], "\x00")
	var args Args
	for range argc {
		end := bytes.IndexByte(rest, 0)
		if end < 0 {
			return Args{}, fmt.Errorf("%w: %d of %d arguments", errMalformed, len(args.Argv), argc)
		}
		args.Argv = append(args.Argv, string(rest[:end]))
		rest = rest[end+1:]
	}
	for {
		end := bytes.IndexByte(rest, 0)
		if end <= 0 {
			return args, nil
		}
		args.Env = append(args.Env, string(rest[:end]))
		rest = rest[end+1:]
	}
}
