package zmx

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/leeovery/portal/internal/proctable"
)

// startingVerbs are the spellings of zmx's commands that fork a session's
// daemon when its socket is missing or refuses: attach, run and write.
var startingVerbs = map[string]bool{"attach": true, "a": true, "run": true, "r": true, "write": true, "wr": true}

// unheld proves that no process holds the session whose socket is the
// directory entry, or says why it can't. A process may hold it when the
// kernel names it zmx and it was exec'd as one of startingVerbs on the
// entry's name, after its environment's ZMX_SESSION_PREFIX, in the Backend's
// directory as its environment resolves it. A session's daemon keeps the argv
// and environment of the command it forked from, so it is one; a client of
// the session may be counted too, and a client whose daemon has died exits on
// the closed socket. A table that can't be read, or a process of that
// command whose arguments or environment the kernel doesn't show, proves
// nothing.
func (b *Backend) unheld(entry string) error {
	procs, err := b.processes.list()
	if err != nil {
		return fmt.Errorf("list the processes that may hold it: %w", err)
	}
	for _, p := range procs {
		if p.Command != program {
			continue
		}
		args, err := b.processes.readArgs(p.PID)
		if errors.Is(err, proctable.ErrGone) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read zmx process %d, which may hold it: %w", p.PID, err)
		}
		started, ok := startedName(args.Argv)
		switch {
		case !ok:
		case len(args.Env) == 0:
			return fmt.Errorf("zmx process %d, which may hold it, runs %q with no environment the kernel shows", p.PID, args.Argv)
		case lookup(args.Env, envPrefix)+started == entry && b.mayBeDir(DirFor(args.Env)):
			return fmt.Errorf("zmx process %d may hold it: it runs %q", p.PID, args.Argv)
		}
	}
	return nil
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
	return !filepath.IsAbs(dir) || resolved(dir) == resolved(b.dir)
}

// resolved is dir with its symbolic links resolved, or as written when they
// can't be: a directory that doesn't resolve is not one that does.
func resolved(dir string) string {
	if path, err := filepath.EvalSymlinks(dir); err == nil {
		return path
	}
	return filepath.Clean(dir)
}
