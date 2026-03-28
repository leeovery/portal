package cmd

import (
	"github.com/leeovery/portal/internal/hooks"
)

// HookExecutorFunc executes resume hooks for a given session.
type HookExecutorFunc func(sessionName string)

// buildHookExecutor creates a HookExecutorFunc that loads the hook store
// and delegates to hooks.ExecuteHooks. The tmux client satisfies
// TmuxOperator; the hook store satisfies HookRepository.
func buildHookExecutor(client hooks.TmuxOperator) HookExecutorFunc {
	return func(sessionName string) {
		store, err := loadHookStore()
		if err != nil {
			return
		}
		hooks.ExecuteHooks(sessionName, client, store)
	}
}
