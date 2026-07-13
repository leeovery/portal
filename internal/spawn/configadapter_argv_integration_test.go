//go:build integration

package spawn

import "testing"

// TestArgvRecipeAdapterOpenWindow_RealExec is the real-exec inch off the unit
// lane: it drives argvRecipeAdapter through the production execRecipeRunner
// against trivial real programs (/usr/bin/true, /usr/bin/false) — no tmux, no
// daemon, no built portal binary — to confirm the exit status maps to
// Success / SpawnFailed. The {command} token substitutes into a real argv
// element and the program ignores it.
func TestArgvRecipeAdapterOpenWindow_RealExec(t *testing.T) {
	command := []string{"/abs/portal", "attach", "proj-abc123"}

	t.Run("integration: it opens via a real argv recipe and maps a clean exit to success", func(t *testing.T) {
		adapter := &argvRecipeAdapter{
			template: []string{"/usr/bin/true", "{command}"},
			runner:   execRecipeRunner{},
		}

		result := adapter.OpenWindow(command)

		if result.Outcome != OutcomeSuccess {
			t.Errorf("Outcome = %v, want OutcomeSuccess for a clean real exit (Detail=%q)", result.Outcome, result.Detail)
		}
	})

	t.Run("integration: it maps a non-zero real exit to spawn-failed", func(t *testing.T) {
		adapter := &argvRecipeAdapter{
			template: []string{"/usr/bin/false", "{command}"},
			runner:   execRecipeRunner{},
		}

		result := adapter.OpenWindow(command)

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed for a non-zero real exit (Detail=%q)", result.Outcome, result.Detail)
		}
	})
}
