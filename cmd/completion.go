package cmd

import (
	"strings"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// completionSessions builds its own client rather than reaching for the
// context-injected one: completion runs on the bootstrap-exempt __complete
// path, where cmd.Context() carries no client and tmuxClient would panic.
var completionSessions = func() []tmux.Session {
	sessions, err := tmux.DefaultClient().ListSessions()
	if err != nil {
		return nil
	}
	return sessions
}

// completionCurrentSession builds its own client for the same reason as its
// neighbour: the bootstrap-exempt __complete path carries no context client.
var completionCurrentSession = func() string {
	return currentPickerSession(tmux.DefaultClient())
}

// completeSessionNames filters by prefix itself, because cobra does not
// prefix-filter a dynamic completion func's returns, and suppresses file
// completion so the shell never merges paths into the session-name list.
func completeSessionNames(toComplete string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, s := range completionSessions() {
		if strings.HasPrefix(s.Name, toComplete) {
			matches = append(matches, s.Name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// Reads only the aliases config file, so it needs no tmux client on the
// bootstrap-exempt __complete path. Any error yields nil, degrading to zero
// suggestions rather than a failure.
var completionAliasKeys = func() []string {
	store, err := loadAliasStore()
	if err != nil {
		return nil
	}
	return store.Keys()
}

func completeAliasKeys(toComplete string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, key := range completionAliasKeys() {
		if strings.HasPrefix(key, toComplete) {
			matches = append(matches, key)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// completeSearchTerm completes a search-form word against the sessions the sigil
// searches, matching on the term after the slash and offering each candidate
// with the slash still on the front: the shell discards any candidate that is
// not an extension of the word being completed. The searched set comes from
// tui.PickerSessions, and whether a completed name still composes a search word
// is asked of resolver.IsSearchSigil — a name that fails it would compose a
// second slash, which the parser reads as a path rather than a search.
func completeSearchTerm(toComplete string) ([]string, cobra.ShellCompDirective) {
	term := resolver.SearchTerm(toComplete)

	var matches []string
	for _, s := range tui.PickerSessions(completionSessions(), completionCurrentSession()) {
		if !resolver.IsSearchSigil("/" + s.Name) {
			continue
		}
		if strings.HasPrefix(s.Name, term) {
			matches = append(matches, "/"+s.Name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// completeOpenPositional completes open's positional target: the search form
// against the term after its slash, every other word against session names.
func completeOpenPositional(toComplete string) ([]string, cobra.ShellCompDirective) {
	if resolver.IsSearchSigil(toComplete) {
		return completeSearchTerm(toComplete)
	}
	return completeSessionNames(toComplete)
}
