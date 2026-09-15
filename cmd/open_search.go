package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// completingPreDashPositional reports whether the word being completed is one
// the `--` separator leaves ahead of the trailing command's own — the bound
// preDashPositionals applies, so the completer offers a search form only where
// the parser would read one. The word being completed is the next positional,
// at index len(args).
//
// The `<=` is load-bearing and must not be tightened to `<`: cobra probe-parses
// the line with an appended `--` before it calls a completer, and pflag never
// resets its dash index between parses, so a line carrying no separator reports
// a dash index of len(args) rather than -1.
func completingPreDashPositional(cmd *cobra.Command, args []string) bool {
	dash := cmd.ArgsLenAtDash()
	return dash < 0 || len(args) <= dash
}

// pickerLanding is how the picker was reached: the filter text it opens with,
// or the search form that opened it. A search form declares the session domain,
// so the picker lands differently for it than for -f's text; a nil search is a
// picker no search form reached.
type pickerLanding struct {
	filter string
	search *tui.SearchForm
}

// validateOpenArgs refuses a search form beside anything else, a command scoped
// both ways or scoped emptily, and a filter beside a target or with no text. It
// runs as open's Args validator because each of those refusals is decided from
// the arguments alone, and cobra validates args before PersistentPreRunE — so a
// refused line starts no tmux server, restores nothing and paints no frame.
//
// The refusals are tested in a fixed order, so a line colliding several ways
// always names the same one.
func validateOpenArgs(cmd *cobra.Command, args []string) error {
	if err := validateSearchFormCollisions(cmd, args); err != nil {
		return err
	}
	return validateCommandScopeAndFilter(cmd, args)
}

// validateSearchFormCollisions refuses a line carrying a search form beside
// anything else. A line carrying no search form collides with nothing here.
func validateSearchFormCollisions(cmd *cobra.Command, args []string) error {
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

	// The named arms above cover open's registered flags by hand, so each refusal
	// names what collided in its own words. This arm reads the rest of the rule
	// from the registration: reaching it means every named flag is unset, so any
	// flag the line still set is one no arm covers — a flag registered on open
	// after these arms were written is refused the day it is registered.
	if name := firstSetLocalFlag(cmd); name != "" {
		return NewUsageError(fmt.Sprintf("cannot use a /term search with --%s", name))
	}
	return nil
}

// firstSetLocalFlag returns the name of the first flag the line set among the
// command's own, and the empty string when it set none. Inherited persistent
// flags are outside the set: the rule exempts them.
//
// The walk is VisitAll filtered on Changed rather than Visit: pflag records what
// a line set on the flag set the value was parsed through, which is the command's
// own Flags() — LocalFlags() is a second set holding the same *pflag.Flag values,
// so Visit over it reports nothing while the shared Changed field is accurate.
// pflag visits lexically with no early exit, so the first name visited is kept.
func firstSetLocalFlag(cmd *cobra.Command) string {
	var first string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if first == "" && f.Changed {
			first = f.Name
		}
	})
	return first
}

// validateCommandScopeAndFilter refuses the non-search collisions, through the
// same parse RunE takes the command and destination from, so the rules have one
// home.
func validateCommandScopeAndFilter(cmd *cobra.Command, args []string) error {
	_, destination, err := parseCommandArgs(cmd, args)
	if err != nil {
		return err
	}
	return validateFilterFlag(cmd, destination)
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

// currentSessionReader names the session the caller is attached to.
type currentSessionReader interface {
	CurrentSessionName() (string, error)
}

// currentPickerSession returns the session the caller is attached to, and the
// empty string outside tmux or when the read fails or answers empty — so a
// surface keying off it holds nothing back on a failed read.
func currentPickerSession(src currentSessionReader) string {
	if !tmux.InsideTmux() {
		return ""
	}
	current, err := src.CurrentSessionName()
	if err != nil {
		return ""
	}
	return current
}

// searchCandidates returns the sessions a term is counted over: the enumeration
// less the session the caller is already in, which the picker omits too.
func searchCandidates(src SearchSessionSource) ([]tmux.Session, error) {
	sessions, err := src.ListSessionsProbe()
	if err != nil {
		return nil, err
	}
	return tui.PickerSessions(sessions, currentPickerSession(src)), nil
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
// page has no settled session list to count against yet — restore has not run — so
// it hands the count to the picker, which takes it once that bootstrap completes.
func runSearchForm(cmd *cobra.Command, term string) error {
	if term == "" {
		return openTUIFunc(cmd, pickerLanding{search: &tui.SearchForm{}}, nil, serverWasStarted(cmd))
	}

	decide := searchDecision(buildSearchSessionSource(cmd), term)
	landing := pickerLanding{search: &tui.SearchForm{Term: term}}

	if deferredBootstrapFromContext(cmd) != nil {
		landing.search.Decide = decide
		return openTUIFunc(cmd, landing, nil, serverWasStarted(cmd))
	}

	name, err := decide()
	if err != nil {
		// This route paints no picker, so the buffered warnings go to the
		// terminal.
		bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
		return err
	}
	if name != "" {
		// No picker is painted on this route, so the notice band never surfaces
		// what the bootstrap accumulated: it goes to the terminal instead, before
		// the attach hands that terminal to tmux.
		bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
		return openSessionFunc(cmd, name)
	}
	return openTUIFunc(cmd, landing, nil, serverWasStarted(cmd))
}
