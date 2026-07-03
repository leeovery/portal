//go:build integration

// skip-bootstrap-when-warm-2-4 — real-tmux integration coverage for the Phase 2
// abridged path.
//
// The unit tests (cmd/abridged_route_test.go, cmd/concurrent_bootstrap_route_test.go)
// prove branch SELECTION against fake commanders. These tests prove the two
// load-bearing guarantees that only hold end-to-end against a REAL isolated tmux
// server with a REAL `_portal-saver` daemon:
//
//   - Abridged self-heal (the "keep the fail-safe" crash-recovery thread): a
//     warm + satisfied command skips restore yet still revives a killed
//     `_portal-saver` daemon (TestAbridged_SatisfiedSkipsRestoreRevivesKilledSaver).
//   - Version-mismatch re-bootstrap: a stale-valued latch + a different running
//     version triggers a real full re-bootstrap that re-stamps
//     @portal-bootstrapped with the new version
//     (TestAbridged_VersionMismatchTriggersFullRebootstrapReStamp).
//   - The full outcome matrix (spec § Latch-Check Placement → Outcome matrix)
//     driven through PersistentPreRunE/Execute against a real client.
//
// Discipline (load-bearing — the prior incident was a leaked test daemon
// corrupting the dev install), mirrored verbatim from
// cmd/concurrent_coldboot_integration_test.go:
//   - portaltest.IsolateStateForTest(t) scrubs the developer XDG_CONFIG_HOME and
//     registers the fingerprint-diff backstop; PORTAL_STATE_DIR pins every
//     subprocess (the tmux-server-spawned saver daemon inherits it).
//   - the saver daemon is reaped by killing the isolated server in t.Cleanup
//     BEFORE tmuxtest's kill-server, so the daemon sees SIGHUP and exits — no
//     zombie (reapSaverDaemon, registered by setupAbridgedEnv).
//   - the singleton / no-leak assertion is EXPLICIT (assertDaemonSingletonNoZombie
//     / assertNoExtraDaemons); the IsolateStateForTest backstop is
//     defence-in-depth, not a substitute.
//   - NO t.Parallel (cmd-package convention; package-level mutable seam state).
//   - every tmux round-trip goes through the isolated `ptl-abridged-` socket —
//     NEVER the developer's default server.
//
// Build & run:  go test -tags=integration ./cmd/... -run Abridged

package cmd

import (
	"os"
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

// setupAbridgedEnv builds the per-test scaffolding for the abridged integration
// suite. It mirrors setupConcurrentColdBootEnv (cmd/concurrent_coldboot_integration_test.go)
// step-for-step — isolated state dir (PORTAL_STATE_DIR pinned, fingerprint
// backstop registered), portal binary on PATH (so a spawned saver daemon
// resolves), HISTFILE routed to /dev/null (teardown-race mitigation), an
// isolated tmux socket, and the reapSaverDaemon t.Cleanup — differing only in
// the `ptl-abridged-` socket prefix mandated by the task. The duplication is a
// deliberate two-instance copy (Rule of Three not yet reached) that keeps the
// two suites' socket namespaces distinct.
func setupAbridgedEnv(t *testing.T) (*tmuxtest.Socket, *tmux.Client, string, []string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ensurePortalOnPATH(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	// See setupConcurrentColdBootEnv: route shell-history writes away from the
	// HOME tempdir so they do not race the framework's RemoveAll on teardown.
	// The tmux server inherits this env. Orthogonal to the daemon/state
	// invariants under test.
	t.Setenv("HISTFILE", os.DevNull)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-abridged-")
	client := ts.Client()

	// Reap the saver daemon BEFORE tmuxtest's kill-server (t.Cleanup LIFO:
	// tmuxtest.New registered its kill-server first, so this fires first). This
	// blocks until the daemon process is gone so it releases the state-dir fds
	// before the framework's t.TempDir RemoveAll runs.
	t.Cleanup(func() {
		reapSaverDaemon(t, ts, client, stateDir)
	})

	resetBootstrapOnce(t)

	return ts, client, stateDir, envSlice
}

// buildLatchingFullOrchestrator returns the full-bootstrap orchestrator used by
// the coldboot suite (real Restoring / OrphanSweeper / Saver / Restore, NoOp
// cleanup + hooks) with the Latch/Version fields ADDED.
//
// buildConcurrentColdBootOrchestrator composes via bootstrap.NewWithDefaults,
// whose contract does NOT populate Latch/Version — so the returned orchestrator
// would never stamp @portal-bootstrapped. Setting the two exported fields
// directly mirrors buildProductionOrchestrator's `Latch: client, Version: version`
// wiring, so Run stamps the version-stamped latch as its final action exactly as
// production does. This is the minimal test-helper extension the task anticipated
// (no production change).
func buildLatchingFullOrchestrator(t *testing.T, client *tmux.Client, stateDir string) *bootstrap.Orchestrator {
	t.Helper()
	orch := buildConcurrentColdBootOrchestrator(t, client, stateDir)
	orch.Latch = client
	orch.Version = version
	return orch
}

// killSaverSessionAndWait kills the live `_portal-saver` session and BLOCKS
// until both the session is gone AND the recorded daemon PID is dead. The wait
// is load-bearing: the abridged revival's BootstrapPortalSaver spawns a fresh
// daemon whose flock acquire runs the Component C pid pre-check — if the prior
// daemon were still alive the acquire would (correctly) refuse, so the revive
// must observe a genuinely-dead predecessor to spawn a clean singleton. Unlike
// reapSaverDaemon this kills only the saver SESSION, leaving the rest of the
// server (and any restored sessions) intact.
func killSaverSessionAndWait(t *testing.T, client *tmux.Client, stateDir string) {
	t.Helper()
	// Snapshot the daemon PID before the kill so its process-exit (fd release,
	// which trails the pane vanishing) can be polled directly.
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

// warmSatisfiedServer brings a real server to the "already-bootstrapped, warm"
// end-state the abridged path expects: server up, `_portal-saver` daemon alive
// (so the abridged liveness probe is a no-op, not a revival), and the
// version-stamped latch stamped == the running version (so BootstrappedLatchSatisfied
// reads satisfied). Used by the SATISFIED outcome-matrix rows so the routing
// assertion runs against a stable warm state without triggering an in-Execute
// daemon spawn.
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

// TestAbridged_SatisfiedSkipsRestoreRevivesKilledSaver is the explicit
// crash-recovery regression guard (spec § Test Strategy → Abridged self-heal;
// § Abridged EnsureSaver → Two independent daemon safety nets). A full bootstrap
// stamps a satisfied latch and restores seeded sessions; after killing the live
// `_portal-saver`, the abridged liveness path (ensureSaverLiveness — exactly
// what PersistentPreRunE's abridged gate runs) must REVIVE the daemon while NOT
// re-running restore.
func TestAbridged_SatisfiedSkipsRestoreRevivesKilledSaver(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)

	// Seed sessions.json so the FULL bootstrap has real restore work. The ghosts
	// it restores are the CONTROL: a full bootstrap creates them; the abridged
	// path must not. (restoretest.SeedSessionsJSON plants single-window/pane
	// sessions.)
	ghosts := []string{"ab-ghost-alpha", "ab-ghost-bravo"}
	restoretest.SeedSessionsJSON(t, stateDir, ghosts...)

	// (1) FULL bootstrap through the production progress pipe: restores the
	// ghosts, spawns `_portal-saver`, and (via the added Latch/Version) stamps
	// the version latch as Run's final action. driveConcurrentColdBoot carries a
	// timeout budget so a wedged step fails loudly rather than hanging.
	orch := buildLatchingFullOrchestrator(t, client, stateDir)
	_, res := driveConcurrentColdBoot(t, orch, stateDir)
	if res.sawFatal || !res.sawComplete {
		t.Fatalf("full bootstrap did not complete cleanly (fatal=%v complete=%v)\n--- portal.log ---\n%s",
			res.sawFatal, res.sawComplete, portaltest.ReadPortalLogSafe(stateDir))
	}

	// (2) Latch satisfied for the running version.
	if !state.BootstrappedLatchSatisfied(client, version) {
		t.Fatalf("latch NOT satisfied after a full bootstrap; expected @portal-bootstrapped == %q\n--- portal.log ---\n%s",
			version, portaltest.ReadPortalLogSafe(stateDir))
	}

	// Positive control: the full bootstrap actually restored the ghosts (proves
	// a full bootstrap DOES create them, so their later absence is meaningful).
	for _, name := range ghosts {
		if !client.HasSession(name) {
			t.Fatalf("full bootstrap did not restore seeded session %q — cannot assert the abridged skip", name)
		}
	}

	// Kill the restored ghosts so their post-abridged absence proves restore was
	// skipped (a full re-run would recreate them; the abridged path cannot).
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

	// (3) Kill the live `_portal-saver` daemon and confirm it is gone.
	killSaverSessionAndWait(t, client, stateDir)
	if _, present, _ := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName); present {
		t.Fatal("_portal-saver still present after killSaverSessionAndWait")
	}

	// (4) Drive the ABRIDGED path — the liveness-only saver revival, no restore.
	resetBootstrapWarnings(t)
	ensureSaverLiveness(client, stateDir)

	// (5a) Saver REVIVED — present, alive, singleton, no zombie/leak.
	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	// (5b) Restore was SKIPPED — the killed ghosts are STILL absent. If the
	// abridged path had run restore (a full bootstrap would), they would be back.
	for _, name := range ghosts {
		if client.HasSession(name) {
			t.Errorf("ghost %q is live after the abridged path — restore must NOT run on a satisfied latch (skip-restore contract violated)", name)
		}
	}

	t.Logf("abridged self-heal OK: saver revived (pane PID=%d), restore skipped (ghosts stay dead)", panePID)
}

// TestAbridged_VersionMismatchTriggersFullRebootstrapReStamp proves the
// upgrade-invalidation path (spec § The Version-Stamped Latch → Why
// version-stamped; § Edge Cases → Upgrade invalidation). A stale-valued latch
// plus a different running version reads NOT satisfied; a full re-bootstrap
// re-stamps @portal-bootstrapped with the new running version and recreates the
// daemon (singleton, no zombie).
func TestAbridged_VersionMismatchTriggersFullRebootstrapReStamp(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)

	// (2) Pin a running version distinct from the stale latch value. Injected via
	// the package-var swap so the mismatch branch is exercised without rebuilding
	// the binary (spec § Test Strategy → Design-for-test).
	prev := version
	version = "test-1.2.3"
	t.Cleanup(func() { version = prev })

	// Bring the server up so the stale latch can be stamped before the bootstrap.
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// (1) Stamp a STALE latch value (a prior binary's version).
	if err := client.SetServerOption(state.BootstrappedMarkerName, "v-old"); err != nil {
		t.Fatalf("stamp stale latch: %v", err)
	}

	// (3) Stale value != running version -> NOT satisfied.
	if state.BootstrappedLatchSatisfied(client, version) {
		t.Fatalf("latch unexpectedly satisfied: stale %q must not match running %q", "v-old", version)
	}

	// (4) A full bootstrap runs (the not-satisfied verdict routes here) and
	// re-stamps the latch with the new running version as Run's final action.
	orch := buildLatchingFullOrchestrator(t, client, stateDir)
	_, res := driveConcurrentColdBoot(t, orch, stateDir)
	if res.sawFatal || !res.sawComplete {
		t.Fatalf("full re-bootstrap did not complete cleanly (fatal=%v complete=%v)\n--- portal.log ---\n%s",
			res.sawFatal, res.sawComplete, portaltest.ReadPortalLogSafe(stateDir))
	}

	// (5) The stored @portal-bootstrapped value now equals the new running version.
	val, found, err := client.TryGetServerOption(state.BootstrappedMarkerName)
	if err != nil {
		t.Fatalf("read @portal-bootstrapped post-rebootstrap: %v", err)
	}
	if !found || val != version {
		t.Fatalf("latch not re-stamped: got (val=%q found=%v), want %q", val, found, version)
	}
	// And the re-converge is complete — the latch now reads satisfied.
	if !state.BootstrappedLatchSatisfied(client, version) {
		t.Error("latch not satisfied after the re-stamp")
	}

	// The daemon was recreated on the full bootstrap — singleton, no zombie.
	panePID := assertDaemonSingletonNoZombie(t, client, stateDir)

	t.Logf("version-mismatch re-bootstrap OK: latch re-stamped %q, daemon singleton pane PID=%d", version, panePID)
}

// TestAbridged_OutcomeMatrix_OpenSatisfied_AbridgedInstantPicker pins outcome
// row 1: `open` (no args) on a SATISFIED latch takes the abridged path — the
// orchestrator never runs, serverStarted=false is threaded to openTUI (instant
// picker, no loading page), and NO deferred bootstrap is stashed. Driven through
// Execute with a REAL client whose latch is a REAL server option; the saver is
// pre-warmed so the abridged liveness probe is a no-op.
func TestAbridged_OutcomeMatrix_OpenSatisfied_AbridgedInstantPicker(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)
	warmSatisfiedServer(t, client, stateDir)
	resetBootstrapWarnings(t)

	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var deferredSeen, serverStarted bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, started bool) error {
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		serverStarted = started
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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

// TestAbridged_OutcomeMatrix_OpenNotSatisfied_ConcurrentDeferred pins outcome
// row 2: `open` (no args) on a NOT-satisfied latch takes the concurrent route —
// the orchestrator is DEFERRED (a deferredBootstrap is stashed for openTUI's
// goroutine), not run synchronously in PersistentPreRunE. No server is started,
// so the latch read fails gracefully into not-satisfied.
func TestAbridged_OutcomeMatrix_OpenNotSatisfied_ConcurrentDeferred(t *testing.T) {
	_, client, _, _ := setupAbridgedEnv(t)

	runner := &recordingRunner{started: true}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

	var deferredSeen bool
	origFunc := openTUIFunc
	openTUIFunc = func(cmd *cobra.Command, _ string, _ []string, _ bool) error {
		// On the deferred route the orchestrator has NOT run inside PersistentPreRunE.
		if runner.calls != 0 {
			t.Errorf("orchestrator ran synchronously (%d calls) on the concurrent route; want deferred", runner.calls)
		}
		deferredSeen = deferredBootstrapFromContext(cmd) != nil
		return nil
	}
	t.Cleanup(func() { openTUIFunc = origFunc })

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

// TestAbridged_OutcomeMatrix_CLISatisfied_AbridgedSync pins outcome row 3: a CLI
// command (`list`) on a SATISFIED latch takes the abridged sync path — the
// orchestrator never runs. Real client, real satisfied latch, pre-warmed saver.
func TestAbridged_OutcomeMatrix_CLISatisfied_AbridgedSync(t *testing.T) {
	_, client, stateDir, _ := setupAbridgedEnv(t)
	warmSatisfiedServer(t, client, stateDir)
	resetBootstrapWarnings(t)

	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

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

// TestAbridged_OutcomeMatrix_CLINotSatisfied_SynchronousFull pins outcome row 4:
// a CLI command (`list`) on a NOT-satisfied latch takes the SYNCHRONOUS full
// bootstrap — the orchestrator runs exactly once in PersistentPreRunE (the
// concurrent flip is scoped to the TUI path only). No server is started, so the
// latch read folds into not-satisfied.
func TestAbridged_OutcomeMatrix_CLINotSatisfied_SynchronousFull(t *testing.T) {
	_, client, _, _ := setupAbridgedEnv(t)

	runner := &recordingRunner{started: false}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, Client: client}
	t.Cleanup(func() { bootstrapDeps = nil })

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
