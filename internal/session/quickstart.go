package session

import (
	"fmt"
	"path/filepath"
)

// SessionChecker reports whether a tmux session exists by name.
type SessionChecker interface {
	HasSession(name string) bool
}

// QuickStartResult contains the result of a quick-start session creation,
// including information needed for the exec handoff.
type QuickStartResult struct {
	// SessionName is the generated tmux session name.
	SessionName string
	// Dir is the resolved directory (git root) where the session was created.
	Dir string
	// ExecArgs are the arguments for syscall.Exec to replace the process with tmux.
	ExecArgs []string
}

// QuickStart orchestrates the quick-start session creation pipeline:
// git root resolution, project registration, session name generation,
// and returns exec args for atomic tmux create-or-attach via process handoff.
type QuickStart struct {
	git     GitResolver
	store   ProjectStore
	checker SessionChecker
	gen     IDGenerator
}

// NewQuickStart creates a QuickStart with the given dependencies.
func NewQuickStart(git GitResolver, store ProjectStore, checker SessionChecker, gen IDGenerator) *QuickStart {
	return &QuickStart{
		git:     git,
		store:   store,
		checker: checker,
		gen:     gen,
	}
}

// Run executes the quick-start pipeline for the given path.
// It resolves the git root, registers the project, generates a session name,
// and returns the result with exec args for atomic tmux create-or-attach handoff.
func (qs *QuickStart) Run(path string) (*QuickStartResult, error) {
	resolvedDir, err := qs.git.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory: %w", err)
	}

	projectName := filepath.Base(resolvedDir)

	exists := func(name string) bool {
		return qs.checker.HasSession(name)
	}

	sessionName, err := GenerateSessionName(projectName, qs.gen, exists)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session name: %w", err)
	}

	if err := qs.store.Upsert(resolvedDir, projectName); err != nil {
		return nil, fmt.Errorf("failed to upsert project: %w", err)
	}

	return &QuickStartResult{
		SessionName: sessionName,
		Dir:         resolvedDir,
		ExecArgs:    []string{"tmux", "new-session", "-A", "-s", sessionName, "-c", resolvedDir},
	}, nil
}
