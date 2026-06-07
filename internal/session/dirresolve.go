package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
)

// PaneCurrentPathReader reads the active pane's current_path for a named
// session. It is the single-method seam that *tmux.Client satisfies via
// ActivePaneCurrentPath, kept narrow so the directory resolver can be unit-
// tested with a fake (and so the "active pane only" contract is structurally
// enforced — there is no all-panes method to call).
type PaneCurrentPathReader interface {
	ActivePaneCurrentPath(session string) (string, error)
}

// ResolveSessionDir is the lazy active-pane → git-root directory resolver for
// the grouped render's stamp-absent fallback. Given a session whose
// @portal-dir stamp is missing (post-reboot restore, or a session already live
// when the feature shipped), it derives the session's directory by reading the
// active pane's current_path, resolving that to a git-root, and reducing the
// result to the same canonical key the project store uses (project.CanonicalDirKey).
//
// It returns (canonicalDir, ok, err):
//
//   - ok==true, err==nil: a directory was derived. Per the adopted contract,
//     ANY readable current_path yields a directory: resolver.ResolveGitRoot
//     returns the pane's own cwd unchanged when the pane is not inside a git
//     repository (it does NOT error), so a non-repo pane still resolves to its
//     real cwd. The caller groups this under By-Project (its matching Project
//     if one is stored, else the Unknown bucket).
//   - ok==false, err==nil: the session is UNRESOLVABLE this pass — a non-fatal
//     result that must never abort the grouped render. Two cases collapse here:
//     (a) the session was killed mid-resolve (the active-pane read failed with a
//     no-such-session / no-pane class tmux error), and (b) the active pane has
//     no readable current_path at all (empty value), e.g. a dead pane. The TRUE
//     "no directory" case is precisely this absence of a readable current_path —
//     NOT the absence of a git repository.
//   - err!=nil: an unexpected, non-churn failure from the pane read (e.g. a
//     transport fault). The caller decides policy; routine session churn is
//     already absorbed into the ok==false path above and never surfaces here.
//
// The active pane only is read — never an enumeration of all panes.
func ResolveSessionDir(session string, reader PaneCurrentPathReader, runner resolver.CommandRunner) (string, bool, error) {
	paneCwd, err := reader.ActivePaneCurrentPath(session)
	if err != nil {
		// Session churn (killed mid-resolve, or a transient empty pane list) is
		// expected and non-fatal: the session is simply unresolvable this pass.
		// Classify via typed sentinels, never substring-matching tmux stderr.
		if errors.Is(err, tmux.ErrNoSuchSession) || errors.Is(err, tmux.ErrEmptyPaneList) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to read active pane path for session %q: %w", session, err)
	}

	// An empty current_path (dead pane / blank value) is the true no-directory
	// case. Guard before ResolveGitRoot, which would os.Stat("") and error.
	if strings.TrimSpace(paneCwd) == "" {
		return "", false, nil
	}

	// ResolveGitRoot returns paneCwd unchanged when it is not inside a repo, so
	// a real cwd always yields a directory. It errors only when paneCwd does not
	// exist on disk — treated as unresolvable this pass rather than fatal.
	gitRoot, err := resolver.ResolveGitRoot(paneCwd, runner)
	if err != nil {
		return "", false, nil
	}

	return project.CanonicalDirKey(gitRoot), true, nil
}
