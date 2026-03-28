package cmd

import (
	"github.com/leeovery/portal/internal/hooks"
)

// HookExecutorFunc executes resume hooks for a given session.
type HookExecutorFunc func(sessionName string)

// buildHookExecutor creates a HookExecutorFunc that loads the hook store
// and delegates to hooks.ExecuteHooks. The tmux client satisfies all
// executor interfaces (PaneLister, KeySender, OptionChecker).
func buildHookExecutor(client interface {
	hooks.PaneLister
	hooks.KeySender
	hooks.OptionChecker
}) HookExecutorFunc {
	return func(sessionName string) {
		store, err := loadHookStore()
		if err != nil {
			return
		}
		hooks.ExecuteHooks(sessionName, client, store, client, client)
	}
}
