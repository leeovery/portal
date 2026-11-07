//go:build integration

package spawn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAdapterSandbox(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()

	t.Run("it refuses every program while a test allows none", func(t *testing.T) {
		t.Setenv(AdapterSandboxEnv, "")
		if err := adapterSandbox([]string{"/usr/bin/osascript", "-e", "x"}); !errors.Is(err, errSandboxed) {
			t.Errorf("adapterSandbox(osascript) = %v, want refused", err)
		}
	})

	t.Run("it runs only a program in a directory a test allows", func(t *testing.T) {
		t.Setenv(AdapterSandboxEnv, other+string(os.PathListSeparator)+allowed)
		for argv0, want := range map[string]bool{
			filepath.Join(allowed, "open-window.sh"):  true,
			filepath.Join(allowed, "sub", "open.sh"):  true,
			"/usr/bin/osascript":                      false,
			allowed + "-sibling/open-window.sh":       false,
			filepath.Join(allowed, "..", "escape.sh"): false,
			"open-window.sh":                          false,
		} {
			if err := adapterSandbox([]string{argv0}); (err == nil) != want {
				t.Errorf("adapterSandbox(%s) = %v, want allowed %v", argv0, err, want)
			}
		}
	})

	t.Run("a refused program makes the adapter fail to open, running nothing", func(t *testing.T) {
		t.Setenv(AdapterSandboxEnv, "")
		poisoned := runAdapterArgv
		runAdapterArgv = runSandboxedArgv
		t.Cleanup(func() { runAdapterArgv = poisoned })
		marker := filepath.Join(t.TempDir(), "ran")
		adapter := &argvRecipeAdapter{template: []string{"/usr/bin/touch", marker, "{command}"}, runner: execRecipeRunner{}}

		if result := adapter.OpenWindow([]string{"/abs/portal"}); result.OK() {
			t.Errorf("OpenWindow = %+v, want a failure", result)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("the refused program ran")
		}
	})
}
