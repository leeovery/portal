package cmd

import (
	"github.com/leeovery/portal/internal/resolver"
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
