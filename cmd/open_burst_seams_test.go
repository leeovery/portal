package cmd

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/spf13/cobra"
)

func TestBuildOpenBurstDeps_PartialInjectionKeepsInjectedFillsRest(t *testing.T) {
	t.Run("injected fields win, unset fields defaulted", func(t *testing.T) {
		isolateTerminalsFile(t)

		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
		cmd := &cobra.Command{}

		injectedAdapter := &spawntest.FakeAdapter{}
		injectedResolve := func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
			return injectedAdapter, spawn.ResolutionConfig
		}
		injectedMintCalled := false
		injectedMint := func(*cobra.Command, string, []string) error {
			injectedMintCalled = true
			return nil
		}
		injectedLogger := log.Discard()

		withOpenBurstDeps(t, OpenBurstDeps{
			Resolve:   injectedResolve,
			LocalMint: injectedMint,
			Logger:    injectedLogger,
		})

		// Records rather than executing the real openPath side effect, so an
		// overwritten LocalMint surfaces as a recorded call instead of a real open.
		openPathRouted := false
		withFuncSeam(t, &openPathFunc, func(*cobra.Command, string, []string) error {
			openPathRouted = true
			return nil
		})

		deps, err := buildOpenBurstDeps(cmd)
		if err != nil {
			t.Fatalf("buildOpenBurstDeps: %v", err)
		}

		gotAdapter, gotResolution := deps.Resolve(spawn.Identity{})
		if gotAdapter != spawn.Adapter(injectedAdapter) || gotResolution != spawn.ResolutionConfig {
			t.Errorf("Resolve overwritten: got (%T, %q), want injected (*spawntest.FakeAdapter, %q)", gotAdapter, gotResolution, spawn.ResolutionConfig)
		}
		if deps.Logger != injectedLogger {
			t.Error("Logger overwritten: want the injected *slog.Logger instance")
		}
		if err := deps.LocalMint(cmd, "/some/dir", nil); err != nil {
			t.Fatalf("injected LocalMint returned error: %v", err)
		}
		if !injectedMintCalled {
			t.Error("LocalMint overwritten: injected sentinel was not invoked")
		}
		if openPathRouted {
			t.Error("LocalMint overwritten: routed to openPathFunc instead of the injected sentinel")
		}

		if _, ok := deps.Ack.(stateAcks); !ok {
			t.Errorf("Ack not defaulted from shared builder: got %T, want the state directory's stateAcks", deps.Ack)
		}
		if deps.ExePath == nil {
			t.Error("ExePath not defaulted from shared builder")
		}
		if deps.Env == nil {
			t.Fatal("Env not defaulted from shared builder")
		}
		if env, err := deps.Env(); err != nil || len(env) == 0 || env[0] != "PATH="+os.Getenv("PATH") {
			t.Errorf("defaulted Env() = %q, %v; want the window environment, this process's PATH first", env, err)
		}

		if deps.Detector == nil {
			t.Error("Detector not defaulted from shared builder")
		}
		if deps.Connector == nil {
			t.Error("Connector default missing")
		}
		if deps.NewBurster == nil {
			t.Error("NewBurster default missing (lazy closure)")
		}
	})

	t.Run("unset LocalMint defaults to openPathFunc", func(t *testing.T) {
		isolateTerminalsFile(t)

		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
		cmd := &cobra.Command{}

		// LocalMint deliberately absent so it takes the production default.
		withOpenBurstDeps(t, OpenBurstDeps{
			Logger: log.Discard(),
		})

		openPathRouted := false
		var recordedDir string
		withFuncSeam(t, &openPathFunc, func(_ *cobra.Command, dir string, _ []string) error {
			openPathRouted = true
			recordedDir = dir
			return nil
		})

		deps, err := buildOpenBurstDeps(cmd)
		if err != nil {
			t.Fatalf("buildOpenBurstDeps: %v", err)
		}

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
