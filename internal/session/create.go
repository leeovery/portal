package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/shellquote"
)

// maxAttempts bounds the fresh nanoids a mint draws when names it composes
// are held already.
const maxAttempts = 10

func ShellFromEnv() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}

// ShellCommand is the argv that runs command in a new session the way tmux
// ran a command string: the words joined with spaces and read by the user's
// interactive shell as shell code, which then re-execs itself, so the session
// outlives the command. The re-exec takes a line of its own, so a command
// ending in "&", a comment or a newline still reaches it. The backend execs
// an argv directly and ends the session when it exits, so a nil argv, the
// backend's own login shell, stands for no command.
func ShellCommand(command []string, shell string) []string {
	if len(command) == 0 {
		return nil
	}
	return []string{shell, "-ic", strings.Join(command, " ") + "\nexec " + shellquote.Single(shell)}
}

type GitResolver interface {
	Resolve(dir string) (string, error)
}

type ProjectStore interface {
	Upsert(path, name, via string) error
}

// Backend is what minting needs of the session backend.
type Backend interface {
	Exists(ctx context.Context, name string) (bool, error)
	ValidateName(name string) error
	Create(ctx context.Context, name, dir string, command []string) (backend.Session, error)
}

// Created is a session a SessionCreator made.
type Created struct {
	Name string
	// Dir is the directory the session was made in: the git root of the one
	// asked for.
	Dir string
	// At is when the backend made it, and PID the process it runs, each zero
	// when the new session did not say.
	At  time.Time
	PID int
}

type SessionCreator struct {
	git     GitResolver
	store   ProjectStore
	backend Backend
	gen     IDGenerator
	shell   string
}

// NewSessionCreator resolves the user's shell from $SHELL at construction time.
func NewSessionCreator(git GitResolver, store ProjectStore, b Backend, gen IDGenerator) *SessionCreator {
	return &SessionCreator{
		git:     git,
		store:   store,
		backend: b,
		gen:     gen,
		shell:   ShellFromEnv(),
	}
}

// Create makes a detached session at dir's git root, registered as a project,
// named {project}-{nanoid}. A non-empty command runs in the session's shell.
func (sc *SessionCreator) Create(ctx context.Context, dir string, command []string) (Created, error) {
	root, err := sc.git.Resolve(dir)
	if err != nil {
		return Created{}, fmt.Errorf("failed to resolve directory: %w", err)
	}
	project := filepath.Base(root)
	if err := sc.store.Upsert(root, project, "internal"); err != nil {
		return Created{}, fmt.Errorf("failed to upsert project: %w", err)
	}
	made, err := sc.mint(ctx, project, root, ShellCommand(command, sc.shell))
	if err != nil {
		return Created{}, err
	}
	return Created{Name: made.Name, Dir: root, At: made.Created, PID: made.PID}, nil
}

// mint creates the session under a fresh name, drawing another nanoid for a
// name held already: one Exists reports, one that does not answer, or one
// Create refuses, as it refuses a name held under another case.
func (sc *SessionCreator) mint(ctx context.Context, project, dir string, argv []string) (backend.Session, error) {
	for range maxAttempts {
		suffix, err := sc.gen()
		if err != nil {
			return backend.Session{}, fmt.Errorf("failed to generate session ID: %w", err)
		}
		name, err := SessionName(project, suffix, sc.backend.ValidateName)
		if err != nil {
			return backend.Session{}, fmt.Errorf("failed to name a session for %q: %w", project, err)
		}
		held, err := sc.backend.Exists(ctx, name)
		if err != nil && !errors.Is(err, backend.ErrTimeout) {
			return backend.Session{}, fmt.Errorf("failed to check session %q: %w", name, err)
		}
		if held || err != nil {
			continue
		}
		made, err := sc.backend.Create(ctx, name, dir, argv)
		if errors.Is(err, backend.ErrSessionExists) {
			continue
		}
		if err != nil {
			return backend.Session{}, fmt.Errorf("failed to create session: %w", err)
		}
		return made, nil
	}
	return backend.Session{}, fmt.Errorf("failed to generate a unique session name after %d attempts", maxAttempts)
}
