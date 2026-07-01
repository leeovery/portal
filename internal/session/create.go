package session

import (
	"fmt"
	"os"
	"strings"
)

// PortalDirOption is the tmux session user-option that stamps a session with
// its resolved directory (the git-root computed in PrepareSession). It is the
// fast path for mapping a live session back to its directory at grouped render
// time — a freely-renamed session name cannot do this. It rides the session
// object, not its name, so it survives rename without a re-stamp.
const PortalDirOption = "@portal-dir"

// PortalIDOption is the tmux session user-option that stamps a session with its
// immutable, rename-immune Portal identity: a fresh opaque token frozen at
// creation. Like @portal-dir it rides the session object, not its name, so it
// survives a rename without a re-stamp — but resume hooks key on it precisely so
// a rename cannot orphan them (the mutable session name cannot serve as that
// anchor). Parallel to PortalDirOption in shape, it diverges in lifecycle:
// @portal-dir lazy-re-derives when absent, whereas @portal-id must be persisted
// across reboots and re-stamped on restore (a Phase 3 concern) because there is
// no way to re-derive a frozen identity.
//
// The literal MUST stay byte-identical to the "@portal-id" embedded in
// tmux.HookKeyFormat so every key-producing site agrees; consistency is achieved
// by both using the identical literal.
const PortalIDOption = "@portal-id"

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
// Single quotes in the command are escaped using the '\” pattern.
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
	Upsert(path, name, via string) error
}

// TmuxClient provides tmux session operations.
type TmuxClient interface {
	HasSession(name string) bool
	NewSession(name, dir, shellCommand string) error
	SetSessionOption(session, name, value string) error
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
	prepared, err := PrepareSession(dir, command, sc.git, sc.store, sc.tmux, sc.gen, sc.shell)
	if err != nil {
		return "", err
	}

	if err := sc.tmux.NewSession(prepared.SessionName, prepared.ResolvedDir, prepared.ShellCmd); err != nil {
		return "", fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Stamp @portal-dir and @portal-id onto the freshly-created session. Both are
	// best-effort at this point: a stamp failure must never fail session creation.
	// Swallowed silently — the session package has no log component and the closed
	// component vocabulary does not include one.
	//
	// @portal-dir is the fast-path for directory resolution at grouped render
	// time; an un-stamped session is re-derived and re-stamped by the lazy
	// stamp-on-render fallback on its first grouped render.
	//
	// @portal-id is the immutable rename-immune identity resume hooks key on. It
	// is frozen here from a fresh generator token; a generation error skips the
	// stamp entirely (session left un-stamped, hook keys fall back to the session
	// name). Fire-and-forget: no uniqueness check, so correctness relies on the
	// generator's width making a birthday collision negligible — the accepted
	// residual is hook-key cross-talk between two panes that happen to share an id.
	_ = sc.tmux.SetSessionOption(prepared.SessionName, PortalDirOption, prepared.ResolvedDir)

	if token, genErr := sc.gen(); genErr == nil {
		_ = sc.tmux.SetSessionOption(prepared.SessionName, PortalIDOption, token)
	}

	return prepared.SessionName, nil
}
