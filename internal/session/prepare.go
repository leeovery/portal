package session

import (
	"fmt"
	"path/filepath"
)

// PreparedSession holds the intermediate result of the shared session-preparation pipeline.
// Both SessionCreator and QuickStart consume this to perform their respective final steps.
type PreparedSession struct {
	// ResolvedDir is the git root directory resolved from the input path.
	ResolvedDir string
	// ProjectName is derived from filepath.Base of ResolvedDir.
	ProjectName string
	// SessionName is the generated tmux session name in {project}-{nanoid} format.
	SessionName string
	// ShellCmd is the constructed shell command string, empty when no command is provided.
	ShellCmd string
}

// PrepareSession executes the shared session-preparation pipeline:
// (1) resolve git root, (2) derive project name, (3) generate session name,
// (4) upsert project in store, (5) build shell command.
func PrepareSession(
	path string,
	command []string,
	git GitResolver,
	store ProjectStore,
	checker SessionChecker,
	gen IDGenerator,
	shell string,
) (*PreparedSession, error) {
	resolvedDir, err := git.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory: %w", err)
	}

	projectName := filepath.Base(resolvedDir)

	exists := func(name string) bool {
		return checker.HasSession(name)
	}

	sessionName, err := GenerateSessionName(projectName, gen, exists)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session name: %w", err)
	}

	// via=internal: the session-creation pipeline is a code-driven mutation,
	// not a user-facing config command.
	if err := store.Upsert(resolvedDir, projectName, "internal"); err != nil {
		return nil, fmt.Errorf("failed to upsert project: %w", err)
	}

	shellCmd := BuildShellCommand(command, shell)

	return &PreparedSession{
		ResolvedDir: resolvedDir,
		ProjectName: projectName,
		SessionName: sessionName,
		ShellCmd:    shellCmd,
	}, nil
}
