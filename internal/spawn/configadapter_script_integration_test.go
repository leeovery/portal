//go:build integration

package spawn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/shellquote"
)

func TestScriptRecipeAdapterOpenWindow_RealExec(t *testing.T) {
	const key = "com.example.MyTerm"
	command := []string{"/abs/portal", "attach", "proj-abc123"}
	wantArg := shellquote.Join(command)

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
