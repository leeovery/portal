// Portal is a Go CLI that provides an interactive session picker for tmux.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/leeovery/portal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		var usageErr *cmd.UsageError
		if errors.As(err, &usageErr) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
