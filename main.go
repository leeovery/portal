// Portal is a Go CLI that provides an interactive session picker for tmux.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/cmd/bootstrap"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var fatal *bootstrap.FatalError
		if errors.As(err, &fatal) {
			// cmd.Execute already wrote fatal.UserMessage to stderr.
			// Avoid duplicating it; just exit non-zero.
			os.Exit(1)
		}

		// Some errors are intentional empty-message sentinels (e.g.
		// cmd.ErrStatusUnhealthy) used solely to drive a non-zero exit
		// after the command has already rendered its output. Skip the
		// stderr write so the user does not see a bare trailing newline.
		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, err)
		}

		var usageErr *cmd.UsageError
		if errors.As(err, &usageErr) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
