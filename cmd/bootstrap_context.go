package cmd

import "github.com/spf13/cobra"

// contextKey is an unexported type for context keys in this package.
type contextKey string

// serverStartedKey is the context key for the serverStarted boolean.
const serverStartedKey contextKey = "serverStarted"

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
