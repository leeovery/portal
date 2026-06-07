package session

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
	shell   string
}

// NewQuickStart creates a QuickStart with the given dependencies.
// The user's shell is resolved from $SHELL at construction time.
func NewQuickStart(git GitResolver, store ProjectStore, checker SessionChecker, gen IDGenerator) *QuickStart {
	return &QuickStart{
		git:     git,
		store:   store,
		checker: checker,
		gen:     gen,
		shell:   ShellFromEnv(),
	}
}

// Run executes the quick-start pipeline for the given path.
// It resolves the git root, registers the project, generates a session name,
// and returns the result with exec args for atomic tmux create-or-attach handoff.
// When command is non-nil and non-empty, a shell-command is appended to exec args.
//
// Unlike SessionCreator.CreateFromDir, this path deliberately does NOT stamp the
// @portal-dir session user-option. syscall.Exec replaces the portal process with
// tmux, so there is no in-process point after session creation at which to call
// SetSessionOption, and set-option must not be injected into the exec handoff.
// QuickStart-created sessions are covered by the lazy stamp-on-render fallback,
// which re-derives the directory and stamps @portal-dir on the first grouped render.
func (qs *QuickStart) Run(path string, command []string) (*QuickStartResult, error) {
	prepared, err := PrepareSession(path, command, qs.git, qs.store, qs.checker, qs.gen, qs.shell)
	if err != nil {
		return nil, err
	}

	execArgs := []string{"tmux", "new-session", "-A", "-s", prepared.SessionName, "-c", prepared.ResolvedDir}
	if prepared.ShellCmd != "" {
		execArgs = append(execArgs, prepared.ShellCmd)
	}

	return &QuickStartResult{
		SessionName: prepared.SessionName,
		Dir:         prepared.ResolvedDir,
		ExecArgs:    execArgs,
	}, nil
}
