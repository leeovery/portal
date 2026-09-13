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
	targets := args
	if dash := cmd.ArgsLenAtDash(); dash >= 0 {
		targets = args[:dash]
	}

	var forms []string
	for _, arg := range targets {
		if resolver.IsSearchSigil(arg) {
			forms = append(forms, arg)
		}
	}
	return forms
}

// pickerLanding is how the picker was reached: the filter text it opens with,
// and whether that text is a search form's term. A search form declares the
// session domain, so the picker lands differently for it than for -f's text.
type pickerLanding struct {
	filter string
	search bool
}
