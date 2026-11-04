package zmx

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/proctable"
)

// startingVerbs are the spellings of zmx's commands that fork a session's
// daemon when its socket is missing or refuses: attach, run and write.
var startingVerbs = map[string]bool{"attach": true, "a": true, "run": true, "r": true, "write": true, "wr": true}

// unheld proves that no process holds the session whose socket is the
// directory entry, or says why it can't. A process may hold it when it was
// exec'd as one of startingVerbs on the entry's name, after its environment's
// ZMX_SESSION_PREFIX, in the Backend's directory as its environment resolves
// it, whatever the kernel names its executable: a zmx renamed or wrapped is
// zmx still, and a process miscounted errs towards a holder, the safe side. A
// session's daemon keeps the argv and environment of the command it forked
// from, so it is one; a client of the session may be counted too, and a
// client whose daemon has died exits on the closed socket. A table that can't
// be read, or a process it can't read, proves nothing.
func (b *Backend) unheld(entry string) error {
	procs, err := b.processes.list()
	if err != nil {
		return fmt.Errorf("list the processes that may hold it: %w", err)
	}
	for _, p := range procs {
		args, err := b.processes.readArgs(p.PID)
		if errors.Is(err, proctable.ErrGone) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read process %d, which may hold it: %w", p.PID, err)
		}
		switch started, ok := startedName(args.Argv); {
		case !ok || !b.mayHold(args, started, entry):
		case len(args.Env) == 0:
			return fmt.Errorf("process %d, which may hold it, runs %q with no environment the kernel shows", p.PID, args.Argv)
		default:
			return fmt.Errorf("process %d may hold it: it runs %q", p.PID, args.Argv)
		}
	}
	return nil
}

// mayHold reports whether a process exec'd with args, starting the session
// started after its prefix, may hold the entry: its environment's prefix and
// directory make the entry its session in the Backend's directory. One whose
// environment the kernel doesn't show, as macOS hides a platform or hardened
// binary's, may hold any entry that ends with started, its prefix and
// directory unknown, and no other: a hardened program of that argv shape on
// another name, such as tart run on a VM's, keeps no refusing socket from
// being proved stale.
func (b *Backend) mayHold(args proctable.Args, started, entry string) bool {
	if len(args.Env) == 0 {
		return strings.HasSuffix(entry, started)
	}
	return lookup(args.Env, envPrefix)+started == entry && b.mayBeDir(DirFor(args.Env))
}

// startedName is the session, before any prefix, whose daemon the zmx
// command argv starts when none answers, read as zmx reads it: attach's as
// attachTarget reads it, and the first word after run or write, none naming
// the prefix alone. An argv zmx answers with help starts nothing.
func startedName(argv []string) (string, bool) {
	if len(argv) < 2 || !startingVerbs[argv[1]] {
		return "", false
	}
	if attachVerbs[argv[1]] {
		return attachTarget(argv[2:])
	}
	if len(argv) == 2 {
		return "", true
	}
	if name := argv[2]; name != "--help" && name != "-h" {
		return name, true
	}
	return "", false
}

// mayBeDir reports whether dir may be the Backend's directory: they resolve
// to one directory, as /tmp and /private/tmp do on macOS, or dir is relative,
// so resolved against a working directory not known here.
func (b *Backend) mayBeDir(dir string) bool {
	return !filepath.IsAbs(dir) || b.isDir(dir)
}

// isDir reports whether dir, absolute, resolves to the Backend's directory.
func (b *Backend) isDir(dir string) bool {
	return filepath.IsAbs(dir) && resolved(dir) == resolved(b.dir)
}

// resolved is dir with its symbolic links resolved, or as written when they
// can't be: a directory that doesn't resolve is not one that does.
func resolved(dir string) string {
	if path, err := filepath.EvalSymlinks(dir); err == nil {
		return path
	}
	return filepath.Clean(dir)
}

// errUnproved is a session's daemon the process table doesn't prove.
var errUnproved = errors.New("no one process is proved the session's zmx daemon")

// endPoll spaces the looks for a signalled daemon's end.
const endPoll = 20 * time.Millisecond

// EndDaemon ends the zmx daemon of the session name, whatever protocol it
// speaks: SIGTERM, sent through signal to the one process the process table
// proves that daemon, proved again just before the signal, as a pid may be
// reused between a proof and the signal. A zmx daemon answers SIGTERM by
// unlinking its socket, then hanging up its program and exiting once the
// program has gone, so EndDaemon waits, within the kill ceiling, for the
// socket and the daemon both to go: once it returns, nothing the session ran
// holds anything. A session whose socket has already gone is ended. It
// signals nothing without the proof: no such process or more than one, or a
// table it can't read, as off darwin.
func (b *Backend) EndDaemon(ctx context.Context, name string, signal func(pid int, sig syscall.Signal) error) error {
	if err := b.endDaemon(ctx, name, signal); err != nil {
		return fmt.Errorf("end the daemon of session %q: %w", name, err)
	}
	return nil
}

func (b *Backend) endDaemon(ctx context.Context, name string, signal func(pid int, sig syscall.Signal) error) error {
	if err := b.ValidateName(name); err != nil {
		return err
	}
	if _, held, err := b.held(name); err != nil || !held {
		return err
	}
	pid, err := b.sessionDaemon(name)
	if err != nil {
		return err
	}
	switch again, err := b.sessionDaemon(name); {
	case err != nil:
		return fmt.Errorf("prove pid %d again: %w", pid, err)
	case again != pid:
		return fmt.Errorf("%w: pid %d, then pid %d", errUnproved, pid, again)
	}
	if err := signal(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}
	return b.awaitEnded(ctx, name, pid)
}

// sessionDaemon is the one process the process table proves the zmx daemon
// of the session name in the Backend's directory, as isSessionDaemon proves
// one.
func (b *Backend) sessionDaemon(name string) (int, error) {
	procs, err := b.processes.list()
	if err != nil {
		return 0, fmt.Errorf("list the processes: %w", err)
	}
	var daemons []int
	for _, p := range procs {
		switch is, err := b.isSessionDaemon(p, name); {
		case err != nil:
			return 0, err
		case is:
			daemons = append(daemons, p.PID)
		}
	}
	if len(daemons) != 1 {
		return 0, fmt.Errorf("%w: %d processes are", errUnproved, len(daemons))
	}
	return daemons[0], nil
}

// isSessionDaemon reports whether the listed process p is proved the zmx
// daemon of the session name in the Backend's directory: a child of launchd
// on no terminal, as zmx's daemon is once it has forked away from the command
// that started it, where a client of the session is neither, exec'd as one of
// startingVerbs on name, after its environment's ZMX_SESSION_PREFIX, with its
// directory the Backend's, compared canonically. A process gone as it is
// read is none, as is one whose environment the kernel doesn't show, which
// names no directory.
func (b *Backend) isSessionDaemon(p proctable.Process, name string) (bool, error) {
	if p.PPID != 1 || p.Terminal {
		return false, nil
	}
	args, err := b.processes.readArgs(p.PID)
	switch {
	case errors.Is(err, proctable.ErrGone):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read process %d: %w", p.PID, err)
	}
	started, ok := startedName(args.Argv)
	return ok && len(args.Env) > 0 && lookup(args.Env, envPrefix)+started == name && b.isDir(DirFor(args.Env)), nil
}

// awaitEnded waits, within the kill ceiling, for the session name's socket to
// go and pid, its daemon, signalled, to exit.
func (b *Backend) awaitEnded(ctx context.Context, name string, pid int) error {
	ctx, cancel := b.bound(ctx, b.limits.kill, "")
	defer cancel()
	for {
		switch ended, err := b.ended(name, pid); {
		case err != nil:
			return fmt.Errorf("look for pid %d's end: %w", pid, err)
		case ended:
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("pid %d outlived SIGTERM: %w", pid, context.Cause(ctx))
		case <-time.After(endPoll):
		}
	}
}

// ended reports whether the session name's socket has gone and pid, once its
// daemon, has exited: the table shows no such process, or one no longer
// proved the daemon, as a pid reused is not.
func (b *Backend) ended(name string, pid int) (bool, error) {
	if _, held, err := b.held(name); err != nil || held {
		return false, err
	}
	procs, err := b.processes.list()
	if err != nil {
		return false, fmt.Errorf("list the processes: %w", err)
	}
	i := slices.IndexFunc(procs, func(p proctable.Process) bool { return p.PID == pid })
	if i < 0 {
		return true, nil
	}
	is, err := b.isSessionDaemon(procs[i], name)
	return !is, err
}
