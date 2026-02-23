// Portal is a Go CLI that provides an interactive session picker for tmux.
package main

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
