// Package backend defines the terminal-session backend Portal drives, in
// Portal's own terms: a session is a persistent terminal with a name, a
// directory and a command. Windows, panes, servers and options exist only
// inside a driver. internal/backend/zmx is the driver.
package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrNoSuchSession is returned, possibly wrapped, by an operation naming a
	// session the backend does not hold.
	ErrNoSuchSession = errors.New("no such session")

	// ErrSessionExists is matched by Create's refusal of a name the backend
	// already holds, a *SessionExistsError.
	ErrSessionExists = errors.New("session already exists")

	// ErrInvalidName is returned, wrapped with the reason, for a name the
	// backend cannot hold.
	ErrInvalidName = errors.New("invalid session name")

	// ErrTimeout is returned, wrapped, by an operation the backend did not
	// answer in time, such as a kill of a wedged session, and by any call on a
	// session that is held but not answering at all.
	ErrTimeout = errors.New("backend did not answer in time")

	// ErrHistoryUnavailable is returned, wrapped, by History when the backend
	// answered without the session's screen. It is never a blank transcript.
	ErrHistoryUnavailable = errors.New("session history unavailable")

	// ErrUnreadableClient is returned, wrapped, by a search for a session's
	// client processes that met a process it could not read, which might have
	// been one of them.
	ErrUnreadableClient = errors.New("a process that may be a client can't be read")
)

// SessionExistsError is Create's refusal of a name the backend already holds.
// It matches ErrSessionExists.
type SessionExistsError struct {
	// Name is the name Create was given.
	Name string
	// Held is the name as the backend holds it, which differs from Name when
	// the backend folds case or Unicode normalisation; empty when unknown.
	Held string
}

func (e *SessionExistsError) Error() string {
	if e.Held == "" || e.Held == e.Name {
		return fmt.Sprintf("create session %q: %v", e.Name, ErrSessionExists)
	}
	return fmt.Sprintf("create session %q: %v as %q", e.Name, ErrSessionExists, e.Held)
}

func (e *SessionExistsError) Unwrap() error { return ErrSessionExists }

// HeldAsError is a per-session call's refusal of a name the backend holds
// only under another spelling, as a filesystem that folds case or Unicode
// normalisation holds it. It matches ErrNoSuchSession.
type HeldAsError struct {
	// Held is the name as the backend holds it.
	Held string
}

func (e *HeldAsError) Error() string {
	return fmt.Sprintf("held as %q: %v", e.Held, ErrNoSuchSession)
}

func (e *HeldAsError) Unwrap() error { return ErrNoSuchSession }

// portalPrefix marks Portal's own sessions, such as _portal-saver.
const portalPrefix = "_"

// Session is one live session as the backend reports it.
type Session struct {
	Name string
	// Answered reports whether the session answered the listing. When it is
	// false the session exists but only Name is known, and every other field
	// is its zero value, not a reading. Such a session, silent, turning
	// connections away, or hanging up without a well-formed answer, fails
	// every call naming it with ErrTimeout until it answers again, and Create
	// refuses its name. Only Detach cannot tell a hangup from its own success.
	Answered bool
	// Dir is the session's working directory, empty when the backend reports
	// none it can name on this machine.
	Dir string
	// Clients counts the other clients connected to the session as it
	// answered: attached terminals, and any reader that was mid-call.
	Clients int
	// PID is the process the session runs: its command or shell, not the
	// backend's own process hosting it.
	PID int
	// Created is zero when the backend reports no creation time.
	Created time.Time
	// Command is the command the session was created with, for display; empty
	// for the user's shell.
	Command string
}

// Attached reports whether any client is attached.
func (s Session) Attached() bool { return s.Clients > 0 }

// PortalOwned reports whether the session is one of Portal's own. Listings
// shown to the user hide these.
func (s Session) PortalOwned() bool { return strings.HasPrefix(s.Name, portalPrefix) }

// Command is a process to run: its argv, with Argv[0] the program's path, its
// complete environment, and its working directory, empty to inherit the
// caller's.
type Command struct {
	Argv []string
	Env  []string
	Dir  string
	// Switches reports that the command, run inside a session, switches that
	// session's clients to another and exits at once, rather than attaching
	// the caller's terminal until it detaches.
	Switches bool
}

// ClientEnv is the environment a backend tracks for one client attached to a
// session: each variable it tracks for that client, in the backend's order,
// set or unset.
type ClientEnv []EnvVar

// EnvVar is one tracked variable.
type EnvVar struct {
	Name, Value string
	// Unset reports that the client's environment lacked the variable.
	Unset bool
}

// Lookup is name's value, and whether it is tracked and set.
func (e ClientEnv) Lookup(name string) (string, bool) {
	for _, v := range e {
		if v.Name == name && !v.Unset {
			return v.Value, true
		}
	}
	return "", false
}

// Matches reports whether a process exec'd with environ carries e: each
// variable e has set, set to the same value in environ, and each e has unset,
// absent from it. The last of a variable's entries in environ is its value.
func (e ClientEnv) Matches(environ []string) bool {
	for _, v := range e {
		value, set := lookupLast(environ, v.Name)
		if set == v.Unset || value != v.Value {
			return false
		}
	}
	return true
}

func lookupLast(environ []string, name string) (value string, set bool) {
	for _, kv := range environ {
		if k, v, _ := strings.Cut(kv, "="); k == name {
			value, set = v, true
		}
	}
	return value, set
}

// ClientProcess is a process attached to a session as one of its clients, as
// the process table shows it: its pid, and the environment it was exec'd with.
type ClientProcess struct {
	PID int
	Env []string
}

// Backend is the session backend. Every call that reaches the backend takes a
// context and fails with its cause once it ends; each also has a ceiling of
// its own, past which it fails with ErrTimeout. List differs on both counts.
type Backend interface {
	// List returns every session, Portal's own included, in name order. A
	// failed read is an error, never an empty list. Its ceiling bounds the
	// whole listing, and a session that has not answered by then is listed
	// with Answered false. It fails with the context's cause only when the
	// context ends before every session has answered or failed to.
	List(ctx context.Context) ([]Session, error)
	// Session reads the one live session of exactly this name, as List would
	// report it, without asking any other. A name held only under another
	// spelling is a *HeldAsError; a session that does not answer is
	// ErrTimeout, never a Session with only its name.
	Session(ctx context.Context, name string) (Session, error)
	// Exists reports whether the backend holds a live session of exactly this
	// name. A session it can tell neither live nor gone is ErrTimeout.
	Exists(ctx context.Context, name string) (bool, error)
	// Create starts a detached session running command in dir, with no
	// terminal attached. An empty command runs the user's shell. It succeeds
	// only once the backend reports it made the session, whatever the
	// command does next: a command that exits at once still made a session.
	// It returns the session as the new session answers, as List would
	// report it; one that has already ended, or does not answer in time, is
	// returned with only its name. A held name, including one held under
	// another case or normalisation, is refused with a *SessionExistsError.
	// The check and the creation are not atomic: a concurrent creator of the
	// same name can win, and this call then briefly attaches to its session
	// and returns the same refusal.
	Create(ctx context.Context, name, dir string, command []string) (Session, error)
	// AttachCommand is the process that attaches the caller's terminal to the
	// live session of exactly this name, or, run inside a session, switches
	// that session's clients to it, as its Switches says.
	AttachCommand(ctx context.Context, name string) (Command, error)
	// Track is command, an attach command, with name set to value in its
	// environment and tracked: LeaderEnv reports it while the client the
	// command attaches leads the session. The given command is unchanged.
	Track(command Command, name, value string) Command
	// LeaderEnv is the tracked environment of the client leading the
	// session: the one that last sent it keyboard input, or one that attached
	// while no client led it. It is empty when no client leads.
	LeaderEnv(ctx context.Context, name string) (ClientEnv, error)
	// Kill ends the session. On ErrTimeout the kill may still land later.
	Kill(ctx context.Context, name string) error
	// Detach detaches every client of the session. On ErrTimeout the detach
	// may still land later.
	Detach(ctx context.Context, name string) error
	// History returns the session's screen with up to rows lines of the
	// scrollback above it, all of it when rows is 0, with colours, as
	// terminal escape sequences, verbatim. A live session always has a
	// screen, so the result is never empty: an answer without one is
	// ErrHistoryUnavailable.
	History(ctx context.Context, name string, rows int) ([]byte, error)
	// Current returns the session this process runs inside, "" outside one.
	Current() string
	// ValidateName refuses a name Create refuses, wrapping ErrInvalidName.
	// The backend can hold sessions made elsewhere under names Create
	// refuses, and every call on a session List reports accepts its name,
	// but AttachCommand refuses one the backend's own attach would misread.
	ValidateName(name string) error
}
