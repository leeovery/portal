package cmd

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/spf13/cobra"
)

type contextKey string

// Set only on the picker path with a bootstrap to run, where PersistentPreRunE
// defers the bootstrap to openTUI's goroutine instead of running it
// synchronously.
const deferredBootstrapKey contextKey = "deferredBootstrap"

type deferredBootstrap struct {
	runner bootstrap.Runner
}

func deferredBootstrapFromContext(cmd *cobra.Command) *deferredBootstrap {
	ctx := cmd.Context()
	if ctx == nil {
		return nil
	}
	d, _ := ctx.Value(deferredBootstrapKey).(*deferredBootstrap)
	return d
}
