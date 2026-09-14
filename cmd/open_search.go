package cmd

import (
	"slices"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// searchFormPositionals returns, in argv order, the positionals carrying the
// search form. The scan stops at a `--` separator: the words after it are the
// trailing command's own, so a `/word` among them is that command's argument
// rather than a search over sessions.
func searchFormPositionals(cmd *cobra.Command, args []string) []string {
	var forms []string
	for _, arg := range preDashPositionals(cmd, args) {
		if resolver.IsSearchSigil(arg) {
			forms = append(forms, arg)
		}
	}
	return forms
}

// preDashPositionals returns the positionals a `--` separator, when present,
// leaves ahead of the trailing command's own words.
func preDashPositionals(cmd *cobra.Command, args []string) []string {
	if dash := cmd.ArgsLenAtDash(); dash >= 0 {
		return args[:dash]
	}
	return args
}

// pickerLanding is how the picker was reached: the filter text it opens with,
// and whether that text is a search form's term. A search form declares the
// session domain, so the picker lands differently for it than for -f's text.
type pickerLanding struct {
	filter string
	search bool
	// decide classifies a search term against the live session list from inside
	// the picker, for an invocation whose bootstrap has not run yet. Nil means
	// the classification was already taken, or there was none to take.
	decide func() (string, error)
}

// validateOpenArgs refuses a line carrying a search form beside anything else.
// It runs as open's Args validator so the refusal is decided from the arguments
// alone: cobra validates args before PersistentPreRunE, so a refused line starts
// no tmux server, restores nothing and paints no frame.
//
// The collisions are tested in a fixed order, so a line colliding several ways
// always names the same one.
func validateOpenArgs(cmd *cobra.Command, args []string) error {
	forms := searchFormPositionals(cmd, args)
	if len(forms) == 0 {
		return nil
	}

	switch {
	case len(forms) > 1:
		return NewUsageError("cannot use a /term search with another /term search")
	case len(preDashPositionals(cmd, args)) > 1:
		return NewUsageError("cannot use a /term search with another target")
	case cmd.ArgsLenAtDash() >= 0 || cmd.Flags().Changed("exec"):
		return NewUsageError("cannot use a /term search with a command (-e/--)")
	case cmd.Flags().Changed("filter"):
		return NewUsageError("cannot use a /term search with -f/--filter")
	case anyOpenDomainPin(cmd):
		return NewUsageError("cannot use a /term search with a domain pin (-s/-p/-z/-a)")
	case cmd.Flags().Changed("ack"):
		return NewUsageError("cannot use a /term search with --ack")
	}
	return nil
}

// SearchSessionSource enumerates the live sessions a search term is counted
// against, and names the session the caller is attached to so the count is taken
// over the set the picker would list. The enumeration discriminates a failed
// read from an empty server: a list that could not be read has searched nothing,
// so it must not be counted as no matches.
type SearchSessionSource interface {
	ListSessionsProbe() ([]tmux.Session, error)
	CurrentSessionName() (string, error)
}

func buildSearchSessionSource(cmd *cobra.Command) SearchSessionSource {
	if openDeps != nil && openDeps.SearchSessions != nil {
		return openDeps.SearchSessions
	}
	return tmuxClient(cmd)
}

// searchCandidates returns the sessions a term is counted over: the enumeration
// less the session the caller is already in, which the picker omits too. A
// current-session read that fails or answers empty drops nothing, so a session
// is never counted out on a failed read.
func searchCandidates(src SearchSessionSource) ([]tmux.Session, error) {
	sessions, err := src.ListSessionsProbe()
	if err != nil {
		return nil, err
	}
	if !tmux.InsideTmux() {
		return sessions, nil
	}
	current, err := src.CurrentSessionName()
	if err != nil || current == "" {
		return sessions, nil
	}
	return slices.DeleteFunc(sessions, func(s tmux.Session) bool { return s.Name == current }), nil
}

// searchMatches returns, in enumeration order, the sessions the term matches.
func searchMatches(term string, sessions []tmux.Session) []tmux.Session {
	var matches []tmux.Session
	for _, s := range sessions {
		if resolver.MatchesSearchTerm(term, s.Name, s.Dir) {
			matches = append(matches, s)
		}
	}
	return matches
}

// searchDecision classifies term against the live session list: a single match
// is the session to attach, any other count is ("", nil) and leaves the picker
// to it, and a failed enumeration is returned unchanged. It is a closure because
// the moment it runs differs by route — here, or from inside the picker.
func searchDecision(src SearchSessionSource, term string) func() (string, error) {
	return func() (string, error) {
		sessions, err := searchCandidates(src)
		if err != nil {
			return "", err
		}
		if matches := searchMatches(term, sessions); len(matches) == 1 {
			return matches[0].Name, nil
		}
		return "", nil
	}
}

// runSearchForm dispatches a search form by the number of live sessions its term
// matches: exactly one attaches directly, any other count opens the picker
// pre-filtered by the term. A term-less form has nothing to count, so it opens
// the picker on the whole live list without reading the session set at all.
//
// An invocation whose bootstrap runs in a goroutine behind the picker's loading
// page has no server to count against yet — every term would answer zero — so it
// hands the count to the picker instead of taking it here.
func runSearchForm(cmd *cobra.Command, term string) error {
	if term == "" {
		return openTUIFunc(cmd, pickerLanding{search: true}, nil, serverWasStarted(cmd))
	}

	decide := searchDecision(buildSearchSessionSource(cmd), term)
	landing := pickerLanding{filter: term, search: true}

	if deferredBootstrapFromContext(cmd) != nil {
		landing.decide = decide
		return openTUIFunc(cmd, landing, nil, serverWasStarted(cmd))
	}

	name, err := decide()
	if err != nil {
		return err
	}
	if name != "" {
		return openSessionFunc(cmd, name)
	}
	return openTUIFunc(cmd, landing, nil, serverWasStarted(cmd))
}
