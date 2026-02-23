package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellFromEnv returns the user's shell from $SHELL, falling back to /bin/sh.
func ShellFromEnv() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}

// BuildShellCommand constructs a tmux shell-command string from a command slice.
// Returns empty string when command is nil or empty.
// The format is: $SHELL -ic '<joined_cmd>; exec $SHELL'
// Single quotes in the command are escaped using the '\'' pattern.
func BuildShellCommand(command []string, shell string) string {
	if len(command) == 0 {
		return ""
	}
	joined := strings.Join(command, " ")
	escaped := strings.ReplaceAll(joined, "'", "'\\''")
	return fmt.Sprintf("%s -ic '%s; exec %s'", shell, escaped, shell)
}

// GitResolver resolves a directory to its git repository root.
type GitResolver interface {
	Resolve(dir string) (string, error)
}

// ProjectStore persists project data.
type ProjectStore interface {
	Upsert(path, name string) error
}

// TmuxClient provides tmux session operations.
type TmuxClient interface {
	HasSession(name string) bool
	NewSession(name, dir, shellCommand string) error
}

// SessionCreator orchestrates the creation of a new tmux session from a directory.
type SessionCreator struct {
	git   GitResolver
	store ProjectStore
	tmux  TmuxClient
	gen   IDGenerator
	shell string
}

// NewSessionCreator creates a SessionCreator with the given dependencies.
// The user's shell is resolved from $SHELL at construction time.
func NewSessionCreator(git GitResolver, store ProjectStore, tmux TmuxClient, gen IDGenerator) *SessionCreator {
	return &SessionCreator{
		git:   git,
		store: store,
		tmux:  tmux,
		gen:   gen,
		shell: ShellFromEnv(),
	}
}

// CreateFromDir resolves the directory to a git root, generates a session name,
// upserts the project in the store, and creates a tmux session.
// When command is non-nil and non-empty, constructs a shell-command for tmux.
// Returns the generated session name.
func (sc *SessionCreator) CreateFromDir(dir string, command []string) (string, error) {
	resolvedDir, err := sc.git.Resolve(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve directory: %w", err)
	}

	projectName := filepath.Base(resolvedDir)

	exists := func(name string) bool {
		return sc.tmux.HasSession(name)
	}

	sessionName, err := GenerateSessionName(projectName, sc.gen, exists)
	if err != nil {
		return "", fmt.Errorf("failed to generate session name: %w", err)
	}

	if err := sc.store.Upsert(resolvedDir, projectName); err != nil {
		return "", fmt.Errorf("failed to upsert project: %w", err)
	}

	shellCmd := BuildShellCommand(command, sc.shell)

	if err := sc.tmux.NewSession(sessionName, resolvedDir, shellCmd); err != nil {
		return "", fmt.Errorf("failed to create tmux session: %w", err)
	}

	return sessionName, nil
}
