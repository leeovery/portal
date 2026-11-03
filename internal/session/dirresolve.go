package session

import (
	"strings"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
)

// GroupingDir derives the directory a session with no recorded one groups
// under: the git root of its working directory, reduced to the project store's
// canonical key. ok is false when there is none to derive: an empty cwd, or
// one gone from disk.
func GroupingDir(cwd string, runner resolver.CommandRunner) (string, bool) {
	// Guard before ResolveGitRoot, which would os.Stat("") and error.
	if strings.TrimSpace(cwd) == "" {
		return "", false
	}
	gitRoot, err := resolver.ResolveGitRoot(cwd, runner)
	if err != nil {
		return "", false
	}
	return project.CanonicalDirKey(gitRoot), true
}
