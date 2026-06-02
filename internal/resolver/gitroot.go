// Package resolver provides directory resolution utilities for Portal.
package resolver

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// CommandRunner defines the interface for executing shell commands.
type CommandRunner interface {
	Run(name string, args ...string) (string, error)
}

// RealCommandRunner executes commands via os/exec.
type RealCommandRunner struct{}

// Run executes a command with the given name and arguments and returns its output.
//
// Boundary class 1: on a non-zero exit (or PATH-lookup failure) the returned
// error embeds the binary path, argv, exit status/signal, and the child's
// trimmed stderr via the shared helper. ResolveGitRoot still swallows that
// error to (dir, nil) for the expected not-a-repo / git-missing classification;
// the enrichment only matters for any caller that surfaces the error.
func (r *RealCommandRunner) Run(name string, args ...string) (string, error) {
	out, err := log.CombinedOutputWithContext(exec.Command(name, args...))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ResolveGitRoot resolves a directory to its git repository root.
// If the directory is within a git repo, returns the repo root.
// If the directory is not a git repo or git is not installed, returns the original directory unchanged.
// Returns an error if the directory does not exist.
func ResolveGitRoot(dir string, runner CommandRunner) (string, error) {
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("directory does not exist: %w", err)
	}

	output, err := runner.Run("git", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return dir, nil
	}

	return strings.TrimSpace(output), nil
}
