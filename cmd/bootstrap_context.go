package cmd

import (
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// serverStartedKey is the context key for the serverStarted boolean.
const serverStartedKey contextKey = "serverStarted"

// tmuxClientKey is the context key for the shared *tmux.Client.
const tmuxClientKey contextKey = "tmuxClient"

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
