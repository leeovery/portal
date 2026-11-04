package saverhost

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
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
// installed tmux Portal's, lacks the saver session's name. The argv is read
// with the environment, from the one read, so a pid reused since the
// identity's read proves nothing.
type proof struct {
	marks    []string
	identify func(pid int) (state.IdentifyResult, error)
	args     func(pid int) (proctable.Args, error)
}

func (p proof) prove(pid int) error {
	verdict, err := p.identify(pid)
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", ErrUnreadable, err)
	case verdict == state.IdentifyDead:
		return fmt.Errorf("pid %d: %w", pid, ErrGone)
	case verdict != state.IdentifyIsPortalDaemon:
		return fmt.Errorf("pid %d is no portal state daemon: %w", pid, ErrNotSaverDaemon)
	}
	args, err := p.args(pid)
	switch {
	case errors.Is(err, proctable.ErrGone):
		return fmt.Errorf("pid %d: %w", pid, ErrGone)
	case err != nil:
		return fmt.Errorf("%w: %w", ErrUnreadable, err)
	case !state.IsDaemonArgv(args.Argv):
		return fmt.Errorf("pid %d runs %q, no portal state daemon: %w", pid, args.Argv, ErrNotSaverDaemon)
	}
	for _, mark := range p.marks {
		if !carries(args.Env, mark) {
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

// checkMarks refuses marks that could prove a daemon other than this
// backend's saver's: none naming the saver session, none beside it saying
// where its session is, or one that isn't KEY=value with both halves.
func checkMarks(marks []string) error {
	named := 0
	for _, mark := range marks {
		key, value, _ := strings.Cut(mark, "=")
		if key == "" || value == "" {
			return fmt.Errorf("mark %q is no KEY=value entry", mark)
		}
		if value == saver.SessionName {
			named++
		}
	}
	switch {
	case named == 0:
		return fmt.Errorf("no mark names the %s session", saver.SessionName)
	case named == len(marks):
		return fmt.Errorf("no mark says where the %s session is", saver.SessionName)
	}
	return nil
}
