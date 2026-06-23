package cmd

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// serverStartedKey is the context key for the serverStarted boolean.
const serverStartedKey contextKey = "serverStarted"

// tmuxClientKey is the context key for the shared *tmux.Client.
const tmuxClientKey contextKey = "tmuxClient"

// deferredBootstrapKey is the context key for the §10.2 deferred bootstrap.
// On the cold + TUI path PersistentPreRunE does NOT run the orchestrator
// synchronously — it stashes the runner here so openTUI runs it in a goroutine,
// streaming progress over the channel. Absent on every other path (warm + CLI),
// where serverStartedKey carries the synchronous orchestrator's return.
const deferredBootstrapKey contextKey = "deferredBootstrap"

// deferredBootstrap carries the runner PersistentPreRunE deferred to openTUI's
// concurrent goroutine on the cold + TUI path (§10.2). Only the runner is
// needed: the client is already threaded via tmuxClientKey.
type deferredBootstrap struct {
	runner bootstrap.Runner
}

// deferredBootstrapFromContext retrieves the deferred bootstrap stashed on the
// cold + TUI path. Returns nil on every synchronous path, which openTUI reads as
// "run synchronously already happened; keep today's behaviour."
func deferredBootstrapFromContext(cmd *cobra.Command) *deferredBootstrap {
	ctx := cmd.Context()
	if ctx == nil {
		return nil
	}
	d, _ := ctx.Value(deferredBootstrapKey).(*deferredBootstrap)
	return d
}

// serverWasStarted retrieves the serverStarted flag from the command's context.
// Returns false if the value was never set (e.g., skipTmuxCheck commands).
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

// tmuxClient retrieves the shared *tmux.Client from the command's context.
// Panics if no client is present — PersistentPreRunE must run before any
// command that uses tmux. Commands with skipTmuxCheck never call this.
func tmuxClient(cmd *cobra.Command) *tmux.Client {
	ctx := cmd.Context()
	if ctx != nil {
		if c, ok := ctx.Value(tmuxClientKey).(*tmux.Client); ok {
			return c
		}
	}
	panic("tmuxClient: no client in context — PersistentPreRunE must run before any command that uses tmux")
}
