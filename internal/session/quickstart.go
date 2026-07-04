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
// and returns the result with exec args for the tmux create-stamp-attach
// handoff. When command is non-nil and non-empty, a shell-command is appended
// to the new-session step.
//
// The exec args are a single chained tmux invocation (";"-separated commands,
// passed as literal ";" argv elements):
//
//	new-session -d -s <name> -c <dir> [<cmd>] ; set-option -t <name> @portal-dir <dir> ; set-option -t <name> @portal-id <token> ; attach-session -t <name>
//
// Creating detached first gives an in-server point at which to stamp
// @portal-dir and @portal-id BEFORE attaching (attach-session blocks the
// chain), so every quick-started session is anchored to its origin directory
// and its immutable identity at creation. Anchoring @portal-dir keeps grouping
// stable after the user cd's the pane elsewhere — the previous design left
// QuickStart sessions un-stamped and relied on a lazy pane-cwd guess, which
// mis-grouped a session whose pane had drifted (e.g. .dotfiles showing under
// portal).
//
// The @portal-id stamp is best-effort: its token is generated in Go here (a
// second qs.gen call, independent of the name suffix) before ExecArgs is
// assembled, because there is no error seam inside the argv chain. On a
// generation failure the set-option step is omitted (session still created,
// un-stamped → the hook key falls back to the session name), mirroring
// CreateFromDir's swallowed stamp failure. When generated, the step is
// interpolated as a literal argv element (the opaque alphanumeric token needs
// no shell-escaping) between the @portal-dir stamp and attach-session — stamped
// while detached, before attach blocks the chain.
//
// GenerateSessionName already guarantees <name> does not exist, so plain
// new-session -d (no -A create-or-attach) always creates; the former -A was a
// belt-and-suspenders attach-to-existing that the uniqueness guarantee makes
// unreachable, and -A -d on an existing session would attach immediately and
// break the stamp-before-attach ordering.
func (qs *QuickStart) Run(path string, command []string) (*QuickStartResult, error) {
	prepared, err := PrepareSession(path, command, qs.git, qs.store, qs.checker, qs.gen, qs.shell)
	if err != nil {
		return nil, err
	}

	// Generate the @portal-id stamp token in Go before assembling the chain —
	// there is no error-return point inside the argv chain. Independent of the
	// name suffix (PrepareSession already consumed one qs.gen call for that);
	// the id is name-independent. A generation failure omits the stamp step.
	idToken, idGenErr := qs.gen()

	execArgs := []string{"tmux", "new-session", "-d", "-s", prepared.SessionName, "-c", prepared.ResolvedDir}
	if prepared.ShellCmd != "" {
		execArgs = append(execArgs, prepared.ShellCmd)
	}
	execArgs = append(execArgs,
		";", "set-option", "-t", prepared.SessionName, PortalDirOption, prepared.ResolvedDir,
	)
	if idGenErr == nil && idToken != "" {
		execArgs = append(execArgs,
			";", "set-option", "-t", prepared.SessionName, PortalIDOption, idToken,
		)
	}
	execArgs = append(execArgs,
		";", "attach-session", "-t", prepared.SessionName,
	)

	return &QuickStartResult{
		SessionName: prepared.SessionName,
		Dir:         prepared.ResolvedDir,
		ExecArgs:    execArgs,
	}, nil
}
