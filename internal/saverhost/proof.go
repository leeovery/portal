package saverhost

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/state"
)

// Every failed proof wraps exactly one of these.
var (
	// ErrGone is a process that has exited.
	ErrGone = errors.New("the process has exited")
	// ErrNotSaverDaemon is a live process that is not the saver's daemon:
	// another program, or a portal state daemon whose environment lacks the
	// saver session's marks, such as another Portal's.
	ErrNotSaverDaemon = errors.New("not the saver's daemon")
	// ErrUnreadable is a process the proof could not read, which proves
	// nothing either way.
	ErrUnreadable = errors.New("the process could not be read")
)

// proof proves a process the saver's daemon of one backend's sessions: a
// portal state daemon by its program and argv, carrying every mark in the
// environment it was exec'd with. A daemon on argv alone, such as the
// installed tmux Portal's, carries none of them.
type proof struct {
	marks    []string
	identify func(pid int) (state.IdentifyResult, error)
	environ  func(pid int) ([]string, error)
}

func (p proof) prove(pid int) error {
	verdict, err := p.identify(pid)
	switch {
	case err != nil:
		return fmt.Errorf("identify pid %d: %w: %w", pid, ErrUnreadable, err)
	case verdict == state.IdentifyDead:
		return fmt.Errorf("pid %d: %w", pid, ErrGone)
	case verdict != state.IdentifyIsPortalDaemon:
		return fmt.Errorf("pid %d is no portal state daemon: %w", pid, ErrNotSaverDaemon)
	}
	environ, err := p.environ(pid)
	switch {
	case errors.Is(err, proctable.ErrGone):
		return fmt.Errorf("pid %d: %w", pid, ErrGone)
	case err != nil:
		return fmt.Errorf("read the environment of pid %d: %w: %w", pid, ErrUnreadable, err)
	}
	for _, mark := range p.marks {
		if !carries(environ, mark) {
			return fmt.Errorf("pid %d's environment lacks %s: %w", pid, mark, ErrNotSaverDaemon)
		}
	}
	return nil
}

// carries reports whether environ sets entry's variable, and to entry's value
// every time it sets it: a process reading the first and one reading the last
// of a repeated variable must agree.
func carries(environ []string, entry string) bool {
	key, _, _ := strings.Cut(entry, "=")
	set := false
	for _, kv := range environ {
		if k, _, _ := strings.Cut(kv, "="); k != key {
			continue
		}
		if kv != entry {
			return false
		}
		set = true
	}
	return set
}

// checkMarks refuses marks that could prove any daemon: none at all, or one
// that names no variable.
func checkMarks(marks []string) error {
	if len(marks) == 0 {
		return errors.New("no marks to prove the saver's daemon by")
	}
	for _, mark := range marks {
		if key, _, ok := strings.Cut(mark, "="); !ok || key == "" {
			return fmt.Errorf("mark %q is no KEY=value entry", mark)
		}
	}
	return nil
}

// kernelEnviron is the environment pid was exec'd with, as the kernel keeps
// it; it can't be read off darwin.
func kernelEnviron(pid int) ([]string, error) {
	args, err := proctable.ReadArgs(pid)
	return args.Env, err
}
