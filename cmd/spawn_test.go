package cmd

// Tests in this file mutate package-level state (bootstrapDeps, spawnDeps) and MUST NOT use t.Parallel.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
)

// fakeTerminalDetector is a fake TerminalDetector that returns a fixed
// Identity, letting the spawn command's --detect branch be Executed without
// real tmux, ps, or defaults reads.
type fakeTerminalDetector struct {
	id spawn.Identity
}

func (f fakeTerminalDetector) Detect() spawn.Identity {
	return f.id
}

func TestSpawnCommand(t *testing.T) {
	// nopRunner short-circuits PersistentPreRunE so no real tmux server is
	// dialed. spawn is intentionally NOT in skipTmuxCheck, so this bootstrap
	// injection is load-bearing (TestMain poisons TMUX; a missed injection
	// would fail loudly instead of reaching the developer's real server).
	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("it prints the friendly name and exact bundle id on --detect for a resolved terminal", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{
			id: spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"},
		}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"spawn", "--detect"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Ghostty") {
			t.Errorf("output %q missing friendly name %q", out, "Ghostty")
		}
		if !strings.Contains(out, "com.mitchellh.ghostty") {
			t.Errorf("output %q missing bundle id %q", out, "com.mitchellh.ghostty")
		}
	})

	t.Run("it prints the honest no-host-local-terminal line on --detect for a NULL identity", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{id: spawn.Identity{}}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"spawn", "--detect"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "no host-local terminal") {
			t.Errorf("output %q missing honest NULL line containing %q", out, "no host-local terminal")
		}
	})

	t.Run("it returns a UsageError when no sessions and no --detect are given", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a UsageError, got nil")
		}
		var usageErr *UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
		}
	})

	t.Run("it returns a UsageError for an unknown flag", func(t *testing.T) {
		spawnDeps = &SpawnDeps{Detector: fakeTerminalDetector{}}
		t.Cleanup(func() { spawnDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"spawn", "--bogus"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected a UsageError, got nil")
		}
		var usageErr *UsageError
		if !errors.As(err, &usageErr) {
			t.Errorf("error %v (%T) does not match *cmd.UsageError", err, err)
		}
	})
}
