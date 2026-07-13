//go:build integration

package spawn

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScriptRecipeAdapterOpenWindow_RealExec is the real-exec inch off the unit
// lane: it drives the script recipe adapter through the production
// execRecipeRunner against REAL shebang scripts in t.TempDir() — no tmux, no
// daemon, no built portal binary. It confirms the exit status maps to
// Success / SpawnFailed AND that the script observed the composed command as its
// positional arg $1 (the ok script records $1 to a sibling file the test reads
// back). Constructed via newScriptRecipeAdapter so the real stat / exec-bit gate
// is exercised end-to-end against a real 0o755 file.
func TestScriptRecipeAdapterOpenWindow_RealExec(t *testing.T) {
	const key = "com.example.MyTerm"
	command := []string{"/abs/portal", "attach", "proj-abc123"}
	wantArg := renderCommandString(command)

	t.Run("integration: it execs a real shebang script, maps a clean exit to success, and observes $1", func(t *testing.T) {
		dir := t.TempDir()
		argFile := filepath.Join(dir, "arg.txt")
		okScript := filepath.Join(dir, "ok.sh")
		body := "#!/bin/sh\nprintf '%s' \"$1\" > \"" + argFile + "\"\nexit 0\n"
		if err := os.WriteFile(okScript, []byte(body), 0o755); err != nil {
			t.Fatalf("writing ok script: %v", err)
		}

		adapter, ok := newScriptRecipeAdapter(key, okScript, execRecipeRunner{})
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for a real executable script, want ok=true")
		}

		result := adapter.OpenWindow(command)

		if result.Outcome != OutcomeSuccess {
			t.Errorf("Outcome = %v, want OutcomeSuccess for a clean real exit (Detail=%q)", result.Outcome, result.Detail)
		}
		got, err := os.ReadFile(argFile)
		if err != nil {
			t.Fatalf("reading recorded $1: %v", err)
		}
		if string(got) != wantArg {
			t.Errorf("script observed $1 = %q, want the composed command as a single positional arg %q", string(got), wantArg)
		}
	})

	t.Run("integration: it maps a non-zero real exit to spawn-failed", func(t *testing.T) {
		dir := t.TempDir()
		failScript := filepath.Join(dir, "fail.sh")
		if err := os.WriteFile(failScript, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
			t.Fatalf("writing fail script: %v", err)
		}

		adapter, ok := newScriptRecipeAdapter(key, failScript, execRecipeRunner{})
		if !ok {
			t.Fatal("newScriptRecipeAdapter returned ok=false for a real executable script, want ok=true")
		}

		result := adapter.OpenWindow(command)

		if result.Outcome != OutcomeSpawnFailed {
			t.Errorf("Outcome = %v, want OutcomeSpawnFailed for a non-zero real exit (Detail=%q)", result.Outcome, result.Detail)
		}
	})
}
