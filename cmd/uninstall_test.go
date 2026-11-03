package cmd

import (
	"errors"
	"testing"
)

func TestUninstall_RefusesUntilTheZmxSaver(t *testing.T) {
	t.Run("it refuses with one line, touching no tmux server and running no bootstrap", func(t *testing.T) {
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: panicRunner{}})

		stdout, _, err := runRootCmd(t, "uninstall")
		if !errors.Is(err, errUninstallUnavailable) {
			t.Fatalf("error = %v, want %v", err, errUninstallUnavailable)
		}
		if want := "portal uninstall returns with the zmx saver"; err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})
}

func TestUninstall_IsBootstrapExempt(t *testing.T) {
	if !bootstrapExempt["uninstall"] {
		t.Fatal("uninstall must be registered in bootstrapExempt")
	}
}
