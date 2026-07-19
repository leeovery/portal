package cmd

// Tests in this file exercise the shared production spawn-seam builder
// (buildProductionSpawnSeams) that both the open burst (buildOpenBurstDeps) and
// the picker (openTUI's tuiConfig population) read from. The file also houses the
// shared fakeTerminalDetector + cmdWithClient test doubles consumed across the
// open-burst / doctor / detection-seam tests. Per the cmd-package convention
// (package-level mutable seams), tests here MUST NOT use t.Parallel.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// fakeTerminalDetector is a fake TerminalDetector that returns a fixed
// Identity, letting host-terminal-aware command bodies (the open burst, doctor,
// the picker detection seam) be Executed without real tmux, ps, or defaults
// reads. It satisfies both cmd.TerminalDetector and tui.TerminalDetector (both
// declare Detect() spawn.Identity).
type fakeTerminalDetector struct {
	id spawn.Identity
}

func (f fakeTerminalDetector) Detect() spawn.Identity {
	return f.id
}

// isolateTerminalsFile points PORTAL_TERMINALS_FILE at a temp path so
// buildResolver (reached via buildProductionSpawnSeams) never reads the
// developer's real terminals.json. The file is absent, so TerminalsStore.Load
// tolerantly yields an empty (native-only) config.
func isolateTerminalsFile(t *testing.T) {
	t.Helper()
	t.Setenv("PORTAL_TERMINALS_FILE", filepath.Join(t.TempDir(), "terminals.json"))
}

// cmdWithClient returns a *cobra.Command carrying client under tmuxClientKey,
// exactly as PersistentPreRunE injects it — so a lazy tmuxClient(cmd) resolution
// (buildOpenBurstDeps, spawnDetector) finds a client instead of panicking.
func cmdWithClient(client *tmux.Client) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.WithValue(context.Background(), tmuxClientKey, client))
	return cmd
}

func TestBuildProductionSpawnSeams(t *testing.T) {
	isolateTerminalsFile(t)

	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	cmder := &recordingCommander{}
	client := tmux.NewClient(cmder)

	seams := buildProductionSpawnSeams(client)

	t.Run("Exists is the client's HasSession probe", func(t *testing.T) {
		if seams.Exists == nil {
			t.Fatal("Exists seam is nil")
		}
		if got := seams.Exists("mysession"); !got {
			t.Errorf("Exists returned false; recordingCommander defaults to no error, want true")
		}
		want := []string{"has-session", "-t", "=mysession"}
		if len(cmder.Calls) != 1 || !slices.Equal(cmder.Calls[0], want) {
			t.Errorf("Exists drove commander with %v, want exactly one %v call", cmder.Calls, want)
		}
	})

	t.Run("Ack is a server-option ack channel", func(t *testing.T) {
		if _, ok := seams.Ack.(*spawn.ServerOptionAckChannel); !ok {
			t.Errorf("Ack is %T, want *spawn.ServerOptionAckChannel", seams.Ack)
		}
	})

	t.Run("Logger is the spawn-component logger", func(t *testing.T) {
		if seams.Logger == nil {
			t.Fatal("Logger seam is nil")
		}
		seams.Logger.Info("seam probe")
		rec := sink.OnlyRecord(t)
		if got := rec.AttrString(t, "component"); got != "spawn" {
			t.Errorf("Logger component attr = %q, want %q (body: %q)", got, "spawn", sink.Body())
		}
	})

	t.Run("Detector, Resolve, Exe, Getenv are wired", func(t *testing.T) {
		if seams.Detector == nil {
			t.Error("Detector seam is nil")
		}
		if seams.Resolve == nil {
			t.Error("Resolve seam is nil")
		}
		if seams.Exe == nil {
			t.Error("Exe seam is nil")
		}
		if seams.Getenv == nil {
			t.Error("Getenv seam is nil")
		}
		// Getenv is os.Getenv: it reads the live process env.
		if got, want := seams.Getenv("PATH"), os.Getenv("PATH"); got != want {
			t.Errorf("Getenv(PATH) = %q, want os.Getenv value %q", got, want)
		}
	})
}
