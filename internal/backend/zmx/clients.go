package zmx

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/proctable"
)

// processTable is the part of internal/proctable AttachClients reads.
type processTable struct {
	list     func() ([]proctable.Process, error)
	readArgs func(pid int) (proctable.Args, error)
}

var systemProcesses = processTable{list: proctable.List, readArgs: proctable.ReadArgs}

// program is the name the kernel keeps for zmx's executable.
const program = "zmx"

// attachVerbs are the spellings of zmx's attach command.
var attachVerbs = map[string]bool{"attach": true, "a": true}

const labelsFlag = "--labels"

// AttachClients is the processes on a terminal running zmx attach on the
// session name in the Backend's directory, with the environment each was
// exec'd with, as the process table shows them: zmx doesn't list a session's
// clients. Only a process the kernel names zmx is read. Its argv names its
// session as zmx reads it, after the ZMX_SESSION_PREFIX of its environment,
// and its environment must resolve to the Backend's directory. zmx's daemon
// keeps the argv of the attach it forked from, but has no terminal, so it is
// never one. A client zmx switched to another session in the same process
// still names the one it attached. A process that exits as it is read is
// skipped; one that can't be read otherwise, or a zmx attach whose
// environment the kernel doesn't show, fails the search with
// backend.ErrUnreadableClient, since it might be one.
func (b *Backend) AttachClients(name string) ([]backend.ClientProcess, error) {
	procs, err := b.processes.list()
	if err != nil {
		return nil, fmt.Errorf("list the clients of session %q: %w", name, err)
	}
	var clients []backend.ClientProcess
	for _, p := range procs {
		if !p.Terminal || p.Command != program {
			continue
		}
		args, err := b.processes.readArgs(p.PID)
		if errors.Is(err, proctable.ErrGone) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("list the clients of session %q: %w: %w", name, backend.ErrUnreadableClient, err)
		}
		attached, ok := attachedName(args.Argv)
		if !ok {
			continue
		}
		if len(args.Env) == 0 {
			return nil, fmt.Errorf("list the clients of session %q: %w: process %d runs zmx attach with no environment the kernel shows", name, backend.ErrUnreadableClient, p.PID)
		}
		if lookup(args.Env, envPrefix)+attached == name && filepath.Clean(DirFor(args.Env)) == b.dir {
			clients = append(clients, backend.ClientProcess{PID: p.PID, Env: args.Env})
		}
	}
	return clients, nil
}

// attachedName is the session zmx attach names in argv, before any prefix,
// read as zmx's parseAttachArgs reads it: flags only before the name, each
// --labels taking a value of its own or after =, the first other word the
// name, and none at all naming the prefix alone. An argv zmx answers with help
// or an error attaches nothing.
func attachedName(argv []string) (string, bool) {
	if len(argv) < 2 || filepath.Base(argv[0]) != program || !attachVerbs[argv[1]] {
		return "", false
	}
	for i := 2; i < len(argv); i++ {
		switch arg := argv[i]; {
		case arg == "--help" || arg == "-h":
			return "", false
		case strings.HasPrefix(arg, labelsFlag+"="):
		case arg == labelsFlag:
			if i++; i == len(argv) {
				return "", false
			}
		default:
			return arg, true
		}
	}
	return "", true
}
