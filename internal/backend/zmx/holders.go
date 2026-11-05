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
// be read proves nothing, nor does a process it can't read once args gives up
// on it whose kernel name contains zmx. Any other process it can't read is
// skipped (decision 70): under load a scan often catches one mid-exec, and
// some stay unreadable for seconds, so only a renamed zmx, unreadable that
// long while its daemon's backlog is full, is miscounted.
func (b *Backend) unheld(entry string) error {
	procs, err := b.processes.list()
	if err != nil {
		return fmt.Errorf("list the processes that may hold it: %w", err)
	}
	for _, p := range procs {
		args, read, err := b.candidateArgs(p)
		switch {
		case err != nil:
			return fmt.Errorf("read process %d, which may hold it: %w", p.PID, err)
		case !read:
			continue
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

// candidateArgs reads the listed process p's arguments for a proof, through
// args: read, once they are; unread, with no error, for a process gone
// meanwhile, or for one still unreadable whose kernel name has no zmx in it,
// which is skipped (decision 70); and the read's failure for one whose name
// has, as a zmx renamed or wrapped may be.
func (b *Backend) candidateArgs(p proctable.Process) (args proctable.Args, read bool, err error) {
	args, err = b.processes.args(p.PID)
	switch {
	case err == nil:
		return args, true, nil
	case errors.Is(err, proctable.ErrGone) || !strings.Contains(p.Command, program):
		return proctable.Args{}, false, nil
	}
	return proctable.Args{}, false, err
}

// mayHold reports whether a process exec'd with args, starting the session
// started after its prefix, may hold the entry: its environment's prefix and
// directory make the entry its session in the Backend's directory. One whose
// environment the kernel doesn't show, as macOS hides a platform or hardened
// binary's, may hold any entry that ends with started, its prefix and
// directory unknown, and no other: a hardened program of that argv shape on
// another name, such as tart run on a VM's, keeps no refusing socket from
// being proved stale. With started empty, naming the prefix alone, it holds
// none, since every entry ends with it: zmx as Portal runs it shows its
// environment, so such a process is no zmx.
func (b *Backend) mayHold(args proctable.Args, started, entry string) bool {
	if len(args.Env) == 0 {
		return started != "" && strings.HasSuffix(entry, started)
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
	return filepath.IsAbs(dir) && CanonicalDir(dir) == CanonicalDir(b.dir)
}

// CanonicalDir is dir with its symbolic links resolved. Two spellings of one
// zmx directory, as /tmp and /private/tmp are on macOS, have one canonical
// form, whether or not it exists: an absolute dir that doesn't resolve has its
// deepest ancestor that does resolved, and the rest appended, cleaned. A path
// that doesn't exist so never compares equal to one that does, which keeps the
// holder proof as strict as resolving existing paths alone. A relative dir that
// doesn't resolve is as written, cleaned.
func CanonicalDir(dir string) string {
	if path, err := filepath.EvalSymlinks(dir); err == nil {
		return path
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return dir
	}
	var missing []string
	for p := dir; p != filepath.Dir(p); p = filepath.Dir(p) {
		missing = append(missing, filepath.Base(p))
		if path, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
			slices.Reverse(missing)
			return filepath.Join(append([]string{path}, missing...)...)
		}
	}
	return dir
}

// errUnproved is a session's daemon the process table doesn't prove.
var errUnproved = errors.New("no one process is proved the session's zmx daemon")

// endPoll spaces the looks for a signalled daemon's end.
const endPoll = 20 * time.Millisecond

// EndDaemon ends the zmx daemon of the session name, whatever protocol it
// speaks: SIGTERM, sent through signal to the one process the process table
// proves that daemon, read again alone just before the signal, as a pid may
// be reused between a proof and the signal. A zmx daemon answers SIGTERM by
// unlinking its socket, then hanging up its program, SIGKILLing it half a
// second later and exiting once it has reaped it, so the end never waits on
// the program's own shutdown. EndDaemon waits, within the kill ceiling, for
// the socket and the daemon both to go: once it returns, nothing the session
// ran holds anything. A session whose socket has already gone is ended, and a
// daemon gone or exiting by the second read, as another's SIGTERM leaves one,
// is waited for as one signalled. It signals nothing without the proof: no
// such process or more than one, or a table it can't read, as off darwin.
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
	d, err := b.sessionDaemon(name)
	if err != nil {
		return err
	}
	switch err := b.stillDaemon(d); {
	case errors.Is(err, proctable.ErrGone):
		return b.awaitEnded(ctx, name, d.PID)
	case err != nil:
		return err
	}
	if err := signal(d.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", d.PID, err)
	}
	return b.awaitEnded(ctx, name, d.PID)
}

// daemon is a process proved a session's zmx daemon, as the proof read it.
type daemon struct {
	proctable.Process
	args proctable.Args
}

// sessionDaemon is the one process the process table proves the zmx daemon
// of the session name in the Backend's directory: detached, and running it as
// runsDaemonOf reads its arguments. A detached process it can't read is
// judged as candidateArgs judges it: one whose kernel name contains zmx
// proves nothing, and any other is skipped (decision 70), so the proof
// signals only a process it read whole.
func (b *Backend) sessionDaemon(name string) (daemon, error) {
	procs, err := b.processes.list()
	if err != nil {
		return daemon{}, fmt.Errorf("list the processes: %w", err)
	}
	var daemons []daemon
	for _, p := range procs {
		if !detached(p) {
			continue
		}
		switch args, read, err := b.candidateArgs(p); {
		case err != nil:
			return daemon{}, fmt.Errorf("read process %d: %w", p.PID, err)
		case read && b.runsDaemonOf(args, name):
			daemons = append(daemons, daemon{Process: p, args: args})
		}
	}
	if len(daemons) != 1 {
		return daemon{}, fmt.Errorf("%w: %d processes are", errUnproved, len(daemons))
	}
	return daemons[0], nil
}

// stillDaemon reads the proved daemon d again, alone, and fails unless it
// reads as it did when proved: its parent, its terminal, its argv and its
// environment. A pid reused since reads otherwise. One gone or exiting fails
// with proctable.ErrGone.
func (b *Backend) stillDaemon(d daemon) error {
	p, err := b.processes.read(d.PID)
	if err != nil {
		return fmt.Errorf("read pid %d again: %w", d.PID, err)
	}
	args, err := b.processes.args(d.PID)
	if err != nil {
		return fmt.Errorf("read pid %d again: %w", d.PID, err)
	}
	if p.PPID != d.PPID || p.Terminal != d.Terminal || !slices.Equal(args.Argv, d.args.Argv) || !slices.Equal(args.Env, d.args.Env) {
		return fmt.Errorf("%w: pid %d reads otherwise than it did when proved", errUnproved, d.PID)
	}
	return nil
}

// detached reports whether p may be a zmx daemon: a child of launchd on no
// terminal, as zmx's daemon is once it has forked away from the command that
// started it, where a client of the session is neither.
func detached(p proctable.Process) bool { return p.PPID == 1 && !p.Terminal }

// runsDaemonOf reports whether a detached process exec'd with args is proved
// the zmx daemon of the session name in the Backend's directory: exec'd as
// one of startingVerbs on name, after its environment's ZMX_SESSION_PREFIX,
// with its directory the Backend's, compared canonically. One whose
// environment the kernel doesn't show names no directory, so is none.
func (b *Backend) runsDaemonOf(args proctable.Args, name string) bool {
	started, ok := startedName(args.Argv)
	return ok && len(args.Env) > 0 && lookup(args.Env, envPrefix)+started == name && b.isDir(DirFor(args.Env))
}

// awaitEnded waits, within the kill ceiling, for the session name's socket to
// go and pid, its daemon, to exit.
func (b *Backend) awaitEnded(ctx context.Context, name string, pid int) error {
	ctx, cancel := b.bound(ctx, b.limits.kill, "")
	defer cancel()
	for {
		left, err := b.left(name, pid)
		switch {
		case err != nil:
			return fmt.Errorf("look for pid %d's end: %w", pid, err)
		case left == "":
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", left, context.Cause(ctx))
		case <-time.After(endPoll):
		}
	}
}

// left says what is left of the session name's daemon, pid: its socket while
// it is held, then pid while the table shows it the daemon still; nothing
// once the table shows no such process, or one no longer proved the daemon,
// as a pid reused is not. Unlike the proof's scan, it fails on pid unreadable,
// whatever the kernel names it: that one pid was the daemon.
func (b *Backend) left(name string, pid int) (string, error) {
	if _, held, err := b.held(name); err != nil || held {
		return "its socket is still there", err
	}
	procs, err := b.processes.list()
	if err != nil {
		return "", fmt.Errorf("list the processes: %w", err)
	}
	i := slices.IndexFunc(procs, func(p proctable.Process) bool { return p.PID == pid })
	if i < 0 || !detached(procs[i]) {
		return "", nil
	}
	switch args, err := b.processes.args(pid); {
	case errors.Is(err, proctable.ErrGone):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("read process %d: %w", pid, err)
	case b.runsDaemonOf(args, name):
		return fmt.Sprintf("pid %d is still its daemon", pid), nil
	}
	return "", nil
}
