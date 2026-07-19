package cmd

// Tests in this file mutate package-level state (openBurstDeps, openPathFunc) and
// MUST NOT use t.Parallel. They exercise buildOpenBurstDeps — the open-burst DI
// defaulting builder — proving injected fields survive the shared-bundle
// defaulting and that every unset field (notably the NOVEL LocalMint → openPathFunc
// default) falls back to its production implementation. Mirrors
// TestBuildSpawnDeps_PartialInjectionKeepsInjectedFillsRest (spawn_seams_test.go).

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

func TestBuildOpenBurstDeps_PartialInjectionKeepsInjectedFillsRest(t *testing.T) {
	// Scenario 1: partially-injected deps. The injected fields (Resolve, LocalMint,
	// Logger) must win over the shared builder, and every unset field must be
	// defaulted from the shared production seams / CLI defaults.
	t.Run("injected fields win, unset fields defaulted", func(t *testing.T) {
		isolateTerminalsFile(t)

		client := tmux.NewClient(&recordingCommander{})
		cmd := cmdWithClient(client)

		// Distinguishable sentinels for the three injected fields.
		injectedAdapter := &spawntest.FakeAdapter{}
		injectedResolve := func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
			return injectedAdapter, spawn.ResolutionConfig
		}
		injectedMintCalled := false
		injectedMint := func(*cobra.Command, string, []string) error {
			injectedMintCalled = true
			return nil
		}
		injectedLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

		openBurstDeps = &OpenBurstDeps{
			Resolve:   injectedResolve,
			LocalMint: injectedMint,
			Logger:    injectedLogger,
		}
		t.Cleanup(func() { openBurstDeps = nil })

		// Guard: if the injected LocalMint were wrongly overwritten by the default,
		// invoking it would route to openPathFunc — record that so the assertion
		// catches the overwrite instead of executing the real openPath side effect.
		origOpenPath := openPathFunc
		openPathRouted := false
		openPathFunc = func(*cobra.Command, string, []string) error {
			openPathRouted = true
			return nil
		}
		t.Cleanup(func() { openPathFunc = origOpenPath })

		deps := buildOpenBurstDeps(cmd)

		// Injected Resolve must win over the shared builder.
		gotAdapter, gotResolution := deps.Resolve(spawn.Identity{})
		if gotAdapter != spawn.Adapter(injectedAdapter) || gotResolution != spawn.ResolutionConfig {
			t.Errorf("Resolve overwritten: got (%T, %q), want injected (*spawntest.FakeAdapter, %q)", gotAdapter, gotResolution, spawn.ResolutionConfig)
		}
		// Injected Logger must win.
		if deps.Logger != injectedLogger {
			t.Error("Logger overwritten: want the injected *slog.Logger instance")
		}
		// Injected LocalMint must win: invoking it hits the sentinel, NOT openPathFunc.
		if err := deps.LocalMint(cmd, "/some/dir", nil); err != nil {
			t.Fatalf("injected LocalMint returned error: %v", err)
		}
		if !injectedMintCalled {
			t.Error("LocalMint overwritten: injected sentinel was not invoked")
		}
		if openPathRouted {
			t.Error("LocalMint overwritten: routed to openPathFunc instead of the injected sentinel")
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

		// The non-shared burst defaults are populated.
		if deps.Detector == nil {
			t.Error("Detector default missing (should route through spawnDetector)")
		}
		if deps.Connector == nil {
			t.Error("Connector default missing")
		}
		if deps.NewBurster == nil {
			t.Error("NewBurster default missing (lazy closure)")
		}
	})

	// Scenario 2: LocalMint NOT injected, so it takes the NOVEL production default —
	// which must route through the openPathFunc package var at CALL time.
	t.Run("unset LocalMint defaults to openPathFunc", func(t *testing.T) {
		isolateTerminalsFile(t)

		client := tmux.NewClient(&recordingCommander{})
		cmd := cmdWithClient(client)

		// LocalMint deliberately absent so it takes the production default.
		openBurstDeps = &OpenBurstDeps{
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		t.Cleanup(func() { openBurstDeps = nil })

		origOpenPath := openPathFunc
		openPathRouted := false
		var recordedDir string
		openPathFunc = func(_ *cobra.Command, dir string, _ []string) error {
			openPathRouted = true
			recordedDir = dir
			return nil
		}
		t.Cleanup(func() { openPathFunc = origOpenPath })

		deps := buildOpenBurstDeps(cmd)

		if deps.LocalMint == nil {
			t.Fatal("LocalMint default missing")
		}
		if err := deps.LocalMint(cmd, "/some/dir", nil); err != nil {
			t.Fatalf("defaulted LocalMint returned error: %v", err)
		}
		if !openPathRouted {
			t.Error("defaulted LocalMint did not route to openPathFunc")
		}
		if recordedDir != "/some/dir" {
			t.Errorf("defaulted LocalMint passed dir %q to openPathFunc, want %q", recordedDir, "/some/dir")
		}
	})
}
