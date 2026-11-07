//go:build integration

package spawn

import "testing"

func TestArgvRecipeAdapterOpenWindow_RealExec(t *testing.T) {
	realAdapterExec(t)
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
