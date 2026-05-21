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

		// Some errors are intentional silent-exit sentinels — e.g.
		// cmd.ErrStatusUnhealthy (status output already rendered to
		// stdout) and the wrapped errCommitNowFailed returned by
		// `portal state commit-now` (a tmux hook subprocess with
		// nowhere meaningful to surface stderr). cmd.IsSilentExitError
		// compile-time-links the suppression contract across the cmd
		// and main packages so neither side drifts on a brittle
		// string-compare. The wrapped commit-now error still preserves
		// the underlying cause via errors.Unwrap for any consumer
		// inspecting the chain.
		if !cmd.IsSilentExitError(err) {
			fmt.Fprintln(os.Stderr, err)
		}

		var usageErr *cmd.UsageError
		if errors.As(err, &usageErr) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
