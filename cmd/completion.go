package cmd

import (
	"strings"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// completionSessionNames is the injectable production seam that sources the
// candidate session names for shell completion. It builds its OWN
// tmux.DefaultClient() rather than reaching for the context-injected client:
// completion runs on the __complete path, which is bootstrap-exempt
// (skipTmuxCheck), so no client is present in cmd.Context() and tmuxClient(cmd)
// would panic.
//
// Names come ONLY from the user-visible ListSessionNames view — it delegates to
// ListSessions, which drops leading-underscore names (_portal-saver /
// _portal-bootstrap) and collapses a no-server error to an empty slice — so
// internal sessions are never suggested and a down server yields no completions
// rather than an error or panic. Tests override this seam (with t.Cleanup
// restore) to stay hermetic.
var completionSessionNames = func() []string {
	names, err := tmux.DefaultClient().ListSessionNames()
	if err != nil {
		return nil
	}
	return names
}

// completeSessionNames is the shared session-name completer for the open
// positional, open --session, and kill positional slots (spec § Tab
// Completion). It sources candidates from completionSessionNames, keeps those
// prefix-matching toComplete (cobra does NOT prefix-filter a dynamic
// ValidArgsFunction's returns), and returns ShellCompDirectiveNoFileComp so the
// shell never merges file/dir completion into the session-name list.
//
// It NEVER calls tmuxClient(cmd): the completer runs on the bootstrap-exempt
// __complete path where no client is in context.
func completeSessionNames(toComplete string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, name := range completionSessionNames() {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}
