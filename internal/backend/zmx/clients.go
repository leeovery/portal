package zmx

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/proctable"
)

// processTable is the part of internal/proctable the Backend reads.
type processTable struct {
	list     func() ([]proctable.Process, error)
	read     func(pid int) (proctable.Process, error)
	readArgs func(pid int) (proctable.Args, error)
}

var systemProcesses = processTable{list: proctable.List, read: proctable.Read, readArgs: proctable.ReadArgs}

// settleEvery and settleWithin space and bound the reads of a process's
// arguments that fail while it runs.
const (
	settleEvery  = 5 * time.Millisecond
	settleWithin = 100 * time.Millisecond
)

// args reads pid's argv and environment, reading again, every settleEvery
// until settleWithin has passed, while a read fails and the process hasn't
// gone: a process caught as it forks or execs fails a read it answers a
// moment later, and under load about a third of the table's scans catch one.
// It fails with proctable.ErrGone for a process gone meanwhile, and otherwise
// with the last read's failure.
func (t processTable) args(pid int) (proctable.Args, error) {
	deadline := time.Now().Add(settleWithin)
	for {
		args, err := t.readArgs(pid)
		if err == nil || errors.Is(err, proctable.ErrGone) || time.Now().After(deadline) {
			return args, err
		}
		time.Sleep(settleEvery)
	}
}

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
// skipped; one still unreadable once args gives up on it, or a zmx attach
// whose environment the kernel doesn't show, fails the search with
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
		args, err := b.processes.args(p.PID)
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

// attachedName is the session zmx attach names in argv, before any prefix, as
// attachTarget reads it.
func attachedName(argv []string) (string, bool) {
	if len(argv) < 2 || filepath.Base(argv[0]) != program || !attachVerbs[argv[1]] {
		return "", false
	}
	return attachTarget(argv[2:])
}

// attachTarget is the session zmx attach's arguments name, before any prefix,
// read as zmx's parseAttachArgs reads them: flags only before the name, each
// --labels taking a value of its own or after =, the first other word the
// name, and none at all naming the prefix alone. Arguments zmx answers with
// help or an error attach nothing.
func attachTarget(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--help" || arg == "-h":
			return "", false
		case strings.HasPrefix(arg, labelsFlag+"="):
		case arg == labelsFlag:
			if i++; i == len(args) {
				return "", false
			}
		default:
			return arg, true
		}
	}
	return "", true
}
