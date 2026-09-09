//go:build integration

package restoretest

import (
	"path/filepath"
	"testing"
)

func TestNewRestoreOrchestrator(t *testing.T) {
	t.Run("it pins the staged binary on the orchestrator it returns", func(t *testing.T) {
		binDir := t.TempDir()
		stateDir := t.TempDir()

		o := NewRestoreOrchestrator(t, nil, stateDir, binDir)

		if o.Exe == nil {
			t.Fatal("orchestrator Exe is nil; want the staged binary pinned")
		}
		got, err := o.Exe()
		if err != nil {
			t.Fatalf("Exe() returned err = %v; want nil", err)
		}
		if want := filepath.Join(binDir, "portal"); got != want {
			t.Errorf("Exe() = %q; want %q", got, want)
		}
		if o.StateDir != stateDir {
			t.Errorf("StateDir = %q; want %q", o.StateDir, stateDir)
		}
		if o.Logger == nil {
			t.Error("Logger is nil; want the state dir's test logger")
		}
	})
}
