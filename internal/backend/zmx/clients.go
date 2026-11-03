package zmx

import (
	"fmt"
	"path/filepath"

	"github.com/leeovery/portal/internal/proctable"
)

// processTable is the part of internal/proctable AttachClients reads.
type processTable struct {
	list     func() ([]proctable.Process, error)
	readArgs func(pid int) (proctable.Args, error)
}

var systemProcesses = processTable{list: proctable.List, readArgs: proctable.ReadArgs}

// attachVerbs are the spellings of zmx's attach command.
var attachVerbs = map[string]bool{"attach": true, "a": true}

// AttachClients is the pids of the processes in a terminal running zmx attach
// on the session name in the Backend's directory, as the process table shows
// them: zmx doesn't list a session's clients. A process's argv names its
// session as zmx reads it, after the ZMX_SESSION_PREFIX of its environment,
// and its environment must resolve to the Backend's directory, each checked
// only when the kernel shows the environment. zmx's daemon keeps the argv of
// the attach it forked from, but has no terminal, so it is never one. A
// client zmx switched to another session in the same process still names the
// one it attached. A process that exits as it is read is skipped.
func (b *Backend) AttachClients(name string) ([]int, error) {
	procs, err := b.processes.list()
	if err != nil {
		return nil, fmt.Errorf("list the clients of session %q: %w", name, err)
	}
	var pids []int
	for _, p := range procs {
		if !p.Terminal {
			continue
		}
		args, err := b.processes.readArgs(p.PID)
		if err == nil && b.attaches(args, name) {
			pids = append(pids, p.PID)
		}
	}
	return pids, nil
}

// attaches reports whether a process exec'd with args runs zmx attach on the
// session name in the Backend's directory.
func (b *Backend) attaches(args proctable.Args, name string) bool {
	attached, ok := attachedName(args.Argv)
	if !ok {
		return false
	}
	if len(args.Env) == 0 {
		return attached == name
	}
	return lookup(args.Env, envPrefix)+attached == name && filepath.Clean(DirFor(args.Env)) == b.dir
}

// attachedName is the session zmx attach names in argv, before any prefix:
// zmx attach [--labels kv] <name> [command…], or its short verb a.
func attachedName(argv []string) (string, bool) {
	if len(argv) < 3 || filepath.Base(argv[0]) != "zmx" || !attachVerbs[argv[1]] {
		return "", false
	}
	rest := argv[2:]
	if rest[0] == "--labels" {
		rest = rest[min(2, len(rest)):]
	}
	if len(rest) == 0 || rest[0] == "" {
		return "", false
	}
	return rest[0], true
}
