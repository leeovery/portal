//go:build integration

package cmd

import (
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
	"github.com/spf13/cobra"
)

func setupAbridgedEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string, []string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ensurePortalOnPATH(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-abridged-")
	client := ts.Client()

	// Cleanup is LIFO, so this fires before tmuxtest's kill-server. It blocks
	// until the daemon is gone, releasing its state-dir fds before RemoveAll.
	t.Cleanup(func() {
		reapSaverDaemon(t, ts, client, stateDir)
	})

	resetBootstrapOnce(t)

	return ts, client, stateDir, envSlice
}

// bootstrap.NewWithDefaults does not populate Latch/Version, so an
// orchestrator built through it would never stamp @portal-bootstrapped —
// they are set directly here, as buildProductionOrchestrator does.
func buildLatchingFullOrchestrator(t *testing.T, client *tmux.Client, stateDir string) *bootstrap.Orchestrator {
	t.Helper()
	orch := buildConcurrentColdBootOrchestrator(t, client, stateDir)
	orch.Latch = client
	orch.Version = version
	return orch
}

// Blocks until the recorded daemon pid is dead, not merely until the session
// is gone: the revived daemon's flock acquire pid pre-check would correctly
// refuse while the predecessor still lives. Unlike reapSaverDaemon this
// leaves the rest of the server intact.
func killSaverSessionAndWait(t *testing.T, client *tmux.Client, stateDir string) {
	t.Helper()
	oldPID, _ := state.ReadPIDFile(stateDir)
	if err := client.KillSession(tmux.PortalSaverName); err != nil {
		t.Fatalf("kill _portal-saver: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, present, perr := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
		saverGone := perr != nil || !present
		daemonDead := oldPID <= 0 || !pidIsAlive(oldPID)
		if saverGone && daemonDead {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("_portal-saver / daemon (pid %d) still observable after kill+wait budget", oldPID)
}

func warmSatisfiedServer(t *testing.T, client *tmux.Client, stateDir string) {
	t.Helper()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer (warm the server): %v", err)
	}
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("pre-bootstrap _portal-saver: %v", err)
	}
	if err := client.SetServerOption(state.BootstrappedMarkerName, version); err != nil {
		t.Fatalf("stamp satisfied latch: %v", err)
	}
	if !state.BootstrappedLatchSatisfied(client, version) {
		t.Fatalf("latch not satisfied after warmSatisfiedServer; expected @portal-bootstrapped == %q", version)
	}
}

func TestAbridged_SatisfiedSkipsRestoreRevivesKilledSaver(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)

	ghosts := []string{"ab-ghost-alpha", "ab-ghost-bravo"}
	restoretest.SeedSessionsJSON(t, stateDir, ghosts...)

	orch := buildLatchingFullOrchestrator(t, client, stateDir)
	_, res := driveConcurrentColdBoot(t, orch, stateDir)
	if res.sawFatal || !res.sawComplete {
		t.Fatalf("full bootstrap did not complete cleanly (fatal=%v complete=%v)\n--- portal.log ---\n%s",
			res.sawFatal, res.sawComplete, portaltest.ReadPortalLogSafe(stateDir))
	}

	if !state.BootstrappedLatchSatisfied(client, version) {
		t.Fatalf("latch NOT satisfied after a full bootstrap; expected @portal-bootstrapped == %q\n--- portal.log ---\n%s",
			version, portaltest.ReadPortalLogSafe(stateDir))
	}

	for _, name := range ghosts {
		if !client.HasSession(name) {
			t.Fatalf("full bootstrap did not restore seeded session %q — cannot assert the abridged skip", name)
		}
	}

	for _, name := range ghosts {
		if err := client.KillSession(name); err != nil {
			t.Fatalf("kill ghost %q: %v", name, err)
		}
	}
	for _, name := range ghosts {
		if client.HasSession(name) {
			t.Fatalf("ghost %q still present after KillSession", name)
		}
	}

	killSaverSessionAndWait(t, client, stateDir)
	if _, present, _ := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName); present {
		t.Fatal("_portal-saver still present after killSaverSessionAndWait")
	}

	resetBootstrapWarnings(t)
	ensureSaverLiveness(client, stateDir)

	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	for _, name := range ghosts {
		if client.HasSession(name) {
			t.Errorf("ghost %q is live after the abridged path — restore must NOT run on a satisfied latch (skip-restore contract violated)", name)
		}
	}

	t.Logf("abridged self-heal OK: saver revived (pane PID=%d), restore skipped (ghosts stay dead)", panePID)
}

func TestAbridged_VersionMismatchTriggersFullRebootstrapReStamp(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)

	prev := version
	version = "test-1.2.3"
	t.Cleanup(func() { version = prev })

	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	if err := client.SetServerOption(state.BootstrappedMarkerName, "v-old"); err != nil {
		t.Fatalf("stamp stale latch: %v", err)
	}

	if state.BootstrappedLatchSatisfied(client, version) {
		t.Fatalf("latch unexpectedly satisfied: stale %q must not match running %q", "v-old", version)
	}

	orch := buildLatchingFullOrchestrator(t, client, stateDir)
	_, res := driveConcurrentColdBoot(t, orch, stateDir)
	if res.sawFatal || !res.sawComplete {
		t.Fatalf("full re-bootstrap did not complete cleanly (fatal=%v complete=%v)\n--- portal.log ---\n%s",
			res.sawFatal, res.sawComplete, portaltest.ReadPortalLogSafe(stateDir))
	}

	val, found, err := client.TryGetServerOption(state.BootstrappedMarkerName)
	if err != nil {
		t.Fatalf("read @portal-bootstrapped post-rebootstrap: %v", err)
	}
	if !found || val != version {
		t.Fatalf("latch not re-stamped: got (val=%q found=%v), want %q", val, found, version)
	}
	if !state.BootstrappedLatchSatisfied(client, version) {
		t.Error("latch not satisfied after the re-stamp")
	}

	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	t.Logf("version-mismatch re-bootstrap OK: latch re-stamped %q, daemon singleton pane PID=%d", version, panePID)
}

func TestAbridged_OutcomeMatrix_OpenSatisfied_AbridgedInstantPicker(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)
	warmSatisfiedServer(t, client, stateDir)
	resetBootstrapWarnings(t)

	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var deferredSeen, serverStarted bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, started bool) error {
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		serverStarted = started
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute open (satisfied): %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("open + satisfied: orchestrator calls = %d, want 0 (abridged, never full)", runner.calls)
	}
	if deferredSeen {
		t.Error("open + satisfied stashed a deferred bootstrap; want none (serverStarted=false must survive to the instant-picker gate)")
	}
	if serverStarted {
		t.Error("open + satisfied threaded serverStarted=true; want false (no loading page — instant picker)")
	}
}

func TestAbridged_OutcomeMatrix_OpenNotSatisfied_ConcurrentDeferred(t *testing.T) {
	_, client, _, _ := setupAbridgedEnv(t)

	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	var deferredSeen bool
	withFuncSeam(t, &openTUIFunc, func(cmd *cobra.Command, _ pickerLanding, _ []string, _ bool) error {
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the concurrent route; want deferred", runner.calls)
		}
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		return nil
	})

	resetRootCmd()
	rootCmd.SetArgs([]string{"open"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute open (not-satisfied): %v", err)
	}

	if !deferredSeen {
		t.Error("open + not-satisfied did not stash a deferred bootstrap; the concurrent + loading route is expected")
	}
	if runner.calls != 0 {
		t.Errorf("open + not-satisfied: orchestrator calls = %d, want 0 (deferred to openTUI's goroutine)", runner.calls)
	}
}

func TestAbridged_OutcomeMatrix_CLISatisfied_AbridgedSync(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)
	warmSatisfiedServer(t, client, stateDir)
	resetBootstrapWarnings(t)

	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	installMockList(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute list (satisfied): %v", err)
	}

	if runner.calls != 0 {
		t.Errorf("CLI + satisfied: orchestrator calls = %d, want 0 (abridged sync, orchestrator not run)", runner.calls)
	}
}

func TestAbridged_OutcomeMatrix_CLINotSatisfied_SynchronousFull(t *testing.T) {
	_, client, _, _ := setupAbridgedEnv(t)

	runner := &recordingRunner{started: false}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, Client: client})

	installMockList(t)

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute list (not-satisfied): %v", err)
	}

	if runner.calls != 1 {
		t.Errorf("CLI + not-satisfied: orchestrator calls = %d, want 1 (synchronous full bootstrap)", runner.calls)
	}
}
