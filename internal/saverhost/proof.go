package saverhost

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
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

// OtherStateDirError is a saver proved another state directory's: its
// daemon carries every mark of the Host's saver session but the state
// directory's, its environment naming the state directory it saves instead.
// It is a failed proof, wrapping ErrNotSaverDaemon.
type OtherStateDirError struct {
	// StateDir is the Host's state directory.
	StateDir string
	// Other is the state directory the saver's daemon saves.
	Other string
	// PID is that daemon.
	PID int
}

func (e *OtherStateDirError) Error() string {
	return fmt.Sprintf("pid %d is the saver's daemon of the state in %s, not %s", e.PID, e.Other, e.StateDir)
}

func (e *OtherStateDirError) Unwrap() error { return ErrNotSaverDaemon }

// StateDirMark is the mark of the saver of the state directory dir: the
// entry, PORTAL_STATE_DIR=dir, that the Host creates the saver with, so every
// process its session runs carries it, and that keeps its daemon's state in
// dir.
func StateDirMark(dir string) string {
	return xdg.StateDir.EnvVar + "=" + dir
}

// proof proves a process the saver's daemon of one backend's sessions and one
// state directory: a portal state daemon by its program and argv, carrying
// every mark in the environment it was exec'd with. A daemon on argv alone,
// such as another install's, lacks a mark: the saver session's name, the
// backend's directory, or the state directory. The argv is read with the
// environment, from the one read, so a pid reused since the identity's read
// proves nothing. A process that has finished exiting is gone, asked before
// it is identified and again once ps names it no daemon, as ps shows a
// zombie.
type proof struct {
	// marks are the backend's marks of the saver session. stateDir is the
	// Host's state directory, and state its mark, which another state
	// directory's saver lacks.
	marks     []string
	stateDir  string
	state     string
	canonical map[string]func(dir string) string
	running   func(pid int) bool
	identify  func(pid int) (state.IdentifyResult, error)
	args      func(pid int) (proctable.Args, error)
}

// prove proves pid the saver's daemon. A daemon carrying every mark but the
// state directory's, its environment naming another state directory in that
// mark's place, one directory, absolute, every time it sets the variable,
// fails as an *OtherStateDirError: the saver of that state directory.
func (p proof) prove(pid int) error {
	args, err := p.daemonArgs(pid)
	if err != nil {
		return err
	}
	if err := p.marked(pid, args.Env); err != nil {
		return err
	}
	if carries(args.Env, p.state, p.canonical) {
		return nil
	}
	key, _, _ := strings.Cut(p.state, "=")
	if other, ok := soleValue(args.Env, key); ok && filepath.IsAbs(other) {
		return &OtherStateDirError{StateDir: p.stateDir, Other: other, PID: pid}
	}
	return fmt.Errorf("pid %d's environment lacks %s: %w", pid, p.state, ErrNotSaverDaemon)
}

// daemonArgs proves pid a portal state daemon by its program and argv, and is
// the argv and environment it was exec'd with, from the read that proves its
// argv.
func (p proof) daemonArgs(pid int) (proctable.Args, error) {
	if !p.running(pid) {
		return proctable.Args{}, fmt.Errorf("pid %d: %w", pid, ErrGone)
	}
	verdict, err := p.identify(pid)
	switch {
	case err != nil:
		return proctable.Args{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	case verdict == state.IdentifyDead:
		return proctable.Args{}, fmt.Errorf("pid %d: %w", pid, ErrGone)
	case verdict != state.IdentifyIsPortalDaemon && !p.running(pid):
		return proctable.Args{}, fmt.Errorf("pid %d: %w", pid, ErrGone)
	case verdict != state.IdentifyIsPortalDaemon:
		return proctable.Args{}, fmt.Errorf("pid %d is no portal state daemon: %w", pid, ErrNotSaverDaemon)
	}
	args, err := p.args(pid)
	switch {
	case errors.Is(err, proctable.ErrGone):
		return proctable.Args{}, fmt.Errorf("pid %d: %w", pid, ErrGone)
	case err != nil:
		return proctable.Args{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	case !state.IsDaemonArgv(args.Argv):
		return proctable.Args{}, fmt.Errorf("pid %d runs %q, no portal state daemon: %w", pid, args.Argv, ErrNotSaverDaemon)
	}
	return args, nil
}

// marked fails unless environ, pid's, carries every one of the backend's
// marks.
func (p proof) marked(pid int, environ []string) error {
	for _, mark := range p.marks {
		if !carries(environ, mark, p.canonical) {
			return fmt.Errorf("pid %d's environment lacks %s: %w", pid, mark, ErrNotSaverDaemon)
		}
	}
	return nil
}

// carries reports whether environ sets entry's variable, and to entry's value
// every time it sets it: a process reading the first and one reading the last
// of a repeated variable must agree. A variable canonical keys names a
// directory, whose absolute spellings of entry's directory count as its value.
func carries(environ []string, entry string, canonical map[string]func(dir string) string) bool {
	key, want, _ := strings.Cut(entry, "=")
	set := false
	for _, kv := range environ {
		k, got, _ := strings.Cut(kv, "=")
		if k != key {
			continue
		}
		if !sameValue(got, want, canonical[key]) {
			return false
		}
		set = true
	}
	return set
}

// soleValue is the value environ sets key to, and whether it sets it, and to
// that one value every time.
func soleValue(environ []string, key string) (string, bool) {
	value, set := "", false
	for _, kv := range environ {
		k, got, _ := strings.Cut(kv, "=")
		switch {
		case k != key:
			continue
		case set && got != value:
			return "", false
		}
		value, set = got, true
	}
	return value, set
}

// sameValue reports whether got, a process's value, is want, the mark's: the
// same string, or, given the canonical spelling of the directories they name,
// one directory once canonical. A relative got is only ever itself: it
// resolves against the process's working directory, not this one.
func sameValue(got, want string, canonical func(dir string) string) bool {
	if got == want {
		return true
	}
	return canonical != nil && filepath.IsAbs(got) && canonical(got) == canonical(want)
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
