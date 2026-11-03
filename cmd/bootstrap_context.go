package cmd

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

type contextKey string

const serverStartedKey contextKey = "serverStarted"

const tmuxClientKey contextKey = "tmuxClient"

// Set only on the picker path with a bootstrap to run, where PersistentPreRunE
// defers the orchestrator to openTUI's goroutine instead of running it
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

func serverWasStarted(cmd *cobra.Command) bool {
	ctx := cmd.Context()
	if ctx == nil {
		return false
	}
	val, ok := ctx.Value(serverStartedKey).(bool)
	if !ok {
		return false
	}
	return val
}

// tmuxClient serves the commands not yet moved to the backend. It starts no
// server. Production puts no client in the context; tests stage theirs there.
func tmuxClient(cmd *cobra.Command) *tmux.Client {
	if ctx := cmd.Context(); ctx != nil {
		if c, ok := ctx.Value(tmuxClientKey).(*tmux.Client); ok {
			return c
		}
	}
	return tmux.DefaultClient()
}
