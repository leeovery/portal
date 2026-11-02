// Package backend defines the terminal-session backend Portal drives, in
// Portal's own terms: a session is a persistent terminal with a name, a
// directory and a command. Windows, panes, servers and options exist only
// inside a driver. internal/backend/zmx is the driver.
package backend

import (
	"errors"
	"strings"
	"time"
)

var (
	// ErrNoSuchSession is returned, possibly wrapped, by an operation naming a
	// session the backend does not hold.
	ErrNoSuchSession = errors.New("no such session")

	// ErrSessionExists is returned, possibly wrapped, by Create for a name the
	// backend already holds.
	ErrSessionExists = errors.New("session already exists")

	// ErrInvalidName is returned, wrapped with the reason, for a name the
	// backend cannot hold.
	ErrInvalidName = errors.New("invalid session name")
)

// portalPrefix marks Portal's own sessions, such as _portal-saver.
const portalPrefix = "_"

// Session is one live session as the backend reports it.
type Session struct {
	Name string
	// Dir is the session's working directory as the backend reports it, empty
	// when the backend reports none it can name.
	Dir     string
	Clients int
	PID     int
	// Created is zero when the backend reports no creation time.
	Created time.Time
	// Command is the command the session was created with, for display; empty
	// for the user's shell.
	Command string
	// Unreachable marks a session that exists but did not answer the listing,
	// so only its Name is known.
	Unreachable bool
}

// Attached reports whether any client is attached.
func (s Session) Attached() bool { return s.Clients > 0 }

// PortalOwned reports whether the session is one of Portal's own. Listings
// shown to the user hide these.
func (s Session) PortalOwned() bool { return strings.HasPrefix(s.Name, portalPrefix) }

// Command is a process to run: its argv, with Argv[0] the program as found on
// PATH, its complete environment, and its working directory, empty to inherit
// the caller's.
type Command struct {
	Argv []string
	Env  []string
	Dir  string
}

// Backend is the session backend.
type Backend interface {
	// List returns every session, Portal's own included. A failed read is an
	// error, never an empty list.
	List() ([]Session, error)
	Exists(name string) (bool, error)
	// Create starts a detached session running command in dir, with no
	// terminal attached. An empty command runs the user's shell.
	Create(name, dir string, command []string) error
	// AttachCommand is the process that attaches the caller's terminal to the
	// session.
	AttachCommand(name string) Command
	Kill(name string) error
	// Detach detaches every client of the session.
	Detach(name string) error
	// History returns the session's screen and scrollback with colours, as
	// terminal escape sequences, verbatim.
	History(name string) ([]byte, error)
	// Current returns the session this process runs inside, "" outside one.
	Current() string
	// ValidateName refuses a name the backend cannot hold, wrapping
	// ErrInvalidName.
	ValidateName(name string) error
}
