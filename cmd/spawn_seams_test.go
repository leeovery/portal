package cmd

// Tests in this file mutate package-level state (spawnDeps) and MUST NOT use
// t.Parallel. They exercise the shared production spawn-seam builder that both
// the CLI (buildSpawnDeps) and the picker (openTUI's tuiConfig population) read
// from, plus buildSpawnDeps' injected-field precedence over it.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// isolateTerminalsFile points PORTAL_TERMINALS_FILE at a temp path so
// buildResolver (reached via buildProductionSpawnSeams) never reads the
// developer's real terminals.json. The file is absent, so TerminalsStore.Load
// tolerantly yields an empty (native-only) config.
func isolateTerminalsFile(t *testing.T) {
	t.Helper()
	t.Setenv("PORTAL_TERMINALS_FILE", filepath.Join(t.TempDir(), "terminals.json"))
}

// cmdWithClient returns a *cobra.Command carrying client under tmuxClientKey,
// exactly as PersistentPreRunE injects it — so buildSpawnDeps' lazy
// tmuxClient(cmd) resolution finds a client instead of panicking.
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

func TestBuildSpawnDeps_PartialInjectionKeepsInjectedFillsRest(t *testing.T) {
	isolateTerminalsFile(t)

	client := tmux.NewClient(&recordingCommander{})
	cmd := cmdWithClient(client)

	// Sentinels for the three injected fields. Each is behaviourally
	// distinguishable from its production default so we can prove the injected
	// value survived rather than being overwritten by the shared builder.
	injectedAdapter := &spawntest.FakeAdapter{}
	injectedResolve := func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return injectedAdapter, spawn.ResolutionConfig
	}
	injectedExists := func(string) bool { return false }
	injectedLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	spawnDeps = &SpawnDeps{
		Resolve: injectedResolve,
		Exists:  injectedExists,
		Logger:  injectedLogger,
	}
	t.Cleanup(func() { spawnDeps = nil })

	deps := buildSpawnDeps(cmd)

	// Injected fields must win over the shared builder.
	gotAdapter, gotResolution := deps.Resolve(spawn.Identity{})
	if gotAdapter != spawn.Adapter(injectedAdapter) || gotResolution != spawn.ResolutionConfig {
		t.Errorf("Resolve overwritten: got (%T, %q), want injected (*spawntest.FakeAdapter, %q)", gotAdapter, gotResolution, spawn.ResolutionConfig)
	}
	if deps.Exists("anything") {
		t.Error("Exists overwritten: injected predicate returns false for all names, got true")
	}
	if deps.Logger != injectedLogger {
		t.Error("Logger overwritten: want the injected *slog.Logger instance")
	}

	// Unset fields must be filled from the shared production builder.
	if _, ok := deps.Ack.(*spawn.ServerOptionAckChannel); !ok {
		t.Errorf("Ack not defaulted from shared builder: got %T, want *spawn.ServerOptionAckChannel", deps.Ack)
	}
	if deps.ExePath == nil {
		t.Error("ExePath not defaulted from shared builder")
	}
	if deps.Getenv == nil {
		t.Error("Getenv not defaulted from shared builder")
	}
	if got, want := deps.Getenv("PATH"), os.Getenv("PATH"); got != want {
		t.Errorf("defaulted Getenv(PATH) = %q, want os.Getenv value %q", got, want)
	}

	// The non-shared CLI defaults are still populated exactly as before.
	if deps.Detector == nil {
		t.Error("Detector default missing (should route through spawnDetector)")
	}
	if deps.Connector == nil {
		t.Error("Connector default missing (CLI-only)")
	}
	if deps.NewBurster == nil {
		t.Error("NewBurster default missing (CLI-only lazy closure)")
	}
}
