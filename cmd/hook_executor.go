package cmd

import (
	"github.com/leeovery/portal/internal/hooks"
)

// HookExecutorFunc executes resume hooks for a given session.
type HookExecutorFunc func(sessionName string)

// buildHookExecutor creates a HookExecutorFunc that loads the hook store
// and delegates to hooks.ExecuteHooks. The tmux client satisfies all
// executor interfaces (PaneLister, KeySender, OptionChecker, AllPaneLister).
// The hook store satisfies HookLoader and HookCleaner.
func buildHookExecutor(client interface {
	hooks.PaneLister
	hooks.KeySender
	hooks.OptionChecker
	hooks.AllPaneLister
}) HookExecutorFunc {
	return func(sessionName string) {
		store, err := loadHookStore()
		if err != nil {
			return
		}
		hooks.ExecuteHooks(sessionName, client, store, client, client, client, store)
	}
}
