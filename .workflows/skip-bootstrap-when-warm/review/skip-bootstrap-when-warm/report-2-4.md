TASK: skip-bootstrap-when-warm-2-4 — Integration coverage: abridged self-heal + version-mismatch re-bootstrap (real tmux)

ACCEPTANCE CRITERIA:
- Every new test is //go:build integration, calls portaltest.IsolateStateForTest(t), applies the env to all subprocesses, reaps the saver daemon in t.Cleanup before the server kill; NO t.Parallel.
- Abridged self-heal: full bootstrap stamps a satisfied latch + kills the live _portal-saver → abridged path revives it (present + alive, singleton, no zombie) AND seeded saved-only sessions are NOT re-restored (restore skipped).
- Version-mismatch: stale-valued latch + different running version → BootstrappedLatchSatisfied == false; after driving the full bootstrap the stored @portal-bootstrapped == the new running version (re-stamped); daemon recreated on the new binary (singleton, no zombie).
- Outcome-matrix rows hold against real tmux: open-satisfied → abridged instant picker (serverStarted==false, no deferred, orchestrator not run); open-not-satisfied → concurrent + loading (deferred stashed); attach/CLI satisfied → abridged sync (orchestrator not run); attach/CLI not-satisfied → synchronous full bootstrap (orchestrator runs).
- assertDaemonSingletonNoZombie / assertNoExtraDaemons pass in the self-heal and version-mismatch tests.
- go build passes; go test -tags=integration ./cmd/... passes with tmux; go test ./... (default) stays green; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
The version-stamped latch (@portal-bootstrapped) gates a full vs abridged bootstrap. The abridged path (spec § Abridged EnsureSaver) runs a liveness-only EnsureSaver that revives a dead _portal-saver daemon without re-running restore — the "keep the fail-safe" crash-recovery thread. A version mismatch (spec § Why version-stamped) reads not-satisfied → the next command full-bootstraps and re-stamps the latch with the new version (self-healing upgrade path). Test Strategy explicitly mandates the integration coverage under IsolateStateForTest: warm+satisfied skips restore but revives a killed saver, and a version-mismatch latch triggers a re-stamping full re-bootstrap. Outcome matrix (spec § Latch-Check Placement) enumerates the four routing rows exercised end-to-end.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/abridged_integration_test.go (new, //go:build integration, 437 lines). Reuses scaffolding from cmd/concurrent_coldboot_integration_test.go (setupConcurrentColdBootEnv-parity setup, buildConcurrentColdBootOrchestrator, driveConcurrentColdBoot, assertDaemonSingletonNoZombie, assertNoExtraDaemons, reapSaverDaemon, pidIsAlive) and the openTUIFunc / deferredBootstrapFromContext / recordingRunner / installMockList unit-route helpers.
- Notes:
  * All six task-named tests present: TestAbridged_SatisfiedSkipsRestoreRevivesKilledSaver, TestAbridged_VersionMismatchTriggersFullRebootstrapReStamp, and the four TestAbridged_OutcomeMatrix_* rows.
  * Self-heal test drives ensureSaverLiveness(client, stateDir) directly — the "unit-of-integration" option the task DO explicitly sanctions ("either call ensureSaverLiveness directly OR drive through PersistentPreRunE"). This is the exact function PersistentPreRunE's abridged gate invokes (cmd/root.go:188), so it is faithful. The gate→ensureSaverLiveness routing is separately proven by the OpenSatisfied/CLISatisfied matrix rows.
  * The satisfied↔abridged routing matrix rows drive the REAL PersistentPreRunE branch through rootCmd.Execute() with a real client + real server-option latch; verified against root.go:173-196 (latchSatisfied verdict) and open.go:136 (serverWasStarted → openTUIFunc started param). Assertions are meaningful against the real control flow.
  * Version-mismatch test injects version via the package-var swap under t.Cleanup (design-for-test), stamps a stale "v-old", asserts not-satisfied, drives the full bootstrap through the production progress pipe, and asserts the re-stamp equals the new version. buildLatchingFullOrchestrator sets orch.Version = version AFTER the swap, so the ordering is correct and the re-stamp assertion is sound.
  * Isolation discipline is intact and matches the sibling suite exactly: IsolateStateForTest scrubs the ambient env (HOME→tempdir, XDG_CONFIG_HOME="") which the tmux-server-spawned daemon inherits, plus t.Setenv("PORTAL_STATE_DIR", stateDir) pins state. The returned envSlice is unused because no test hand-spawns a subprocess with cmd.Env — the daemon is spawned via the tmux server which inherits the scrubbed ambient env (verbatim parity with setupConcurrentColdBootEnv). No dev-install leakage path.
  * No production code changed by this task — all new code is under //go:build integration, so the default `go test ./...` is unaffected (the tag excludes it).

TESTS:
- Status: Adequate
- Coverage:
  * Self-heal: positive control (full bootstrap DID restore the ghosts) makes the negative (ghosts stay dead after abridged) meaningful — proves restore was genuinely skipped, not merely absent. killSaverSessionAndWait blocks until BOTH the saver session is gone AND the recorded daemon PID is dead (load-bearing: the Component-C pid pre-check on revive must observe a dead predecessor). assertDaemonSingletonNoZombie asserts the revived daemon is a singleton with no leak.
  * Version-mismatch: asserts not-satisfied on mismatch, re-stamp == new version, latch reads satisfied post-restamp, and daemon singleton on the new binary.
  * Matrix rows map 1:1 to spec § Outcome matrix; each asserts the load-bearing signal (runner.calls 0/1, deferredSeen, serverStarted). Row differences (open vs CLI, satisfied vs not) are genuinely distinct routing outcomes — not redundant.
  * Edge cases from the task/spec covered: warm+satisfied skips heavy steps (ghost-absence control), saver-dead revival (crash-recovery guard), version-mismatch re-stamp (upgrade path), full matrix, daemon-spawning discipline.
- Notes:
  * Not over-tested: no redundant assertions; the positive controls are necessary to make negatives meaningful. Matrix split into focused cases is explicitly sanctioned by the task ("TestAbridged_OutcomeMatrix or split into focused cases").
  * Not under-tested: no single test drives a REAL satisfied command through PersistentPreRunE AND revives a dead saver in one flow (self-heal calls ensureSaverLiveness directly; matrix satisfied rows pre-warm the saver so the probe is a no-op). This composition gap is closed by construction (the gate calls ensureSaverLiveness verbatim) and by the task's explicit sanction of the direct-call approach — not a coverage defect.
  * Determinism: the daemon captures but never restores, so killed ghosts cannot resurrect; the ghost-absence assertion is race-free. No flakiness patterns observed.

CODE QUALITY:
- Project conventions: Followed. //go:build integration ✓; NO t.Parallel ✓ (cmd package-level mutable seam state); IsolateStateForTest + PORTAL_STATE_DIR + reap-before-server-kill (LIFO) ✓; explicit singleton assertion (not relying on the fingerprint backstop) ✓; isolated ptl-abridged- socket ✓.
- SOLID principles: Good. Helpers are single-purpose (setupAbridgedEnv, buildLatchingFullOrchestrator, killSaverSessionAndWait, warmSatisfiedServer). buildLatchingFullOrchestrator is the minimal test-helper extension the task anticipated (Latch/Version fields set directly, mirroring buildProductionOrchestrator) with no production change.
- Complexity: Low. Linear test bodies, clear numbered-step structure mirroring the task DO.
- Modern idioms: Yes. Signal-0 liveness probe, deadline-poll loops, package-var injection.
- Readability: Excellent. Thorough doc comments explaining the load-bearing reap ordering, the kill-session-only-vs-whole-server distinction, and the design-for-test version swap.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/abridged_integration_test.go:135-145 & cmd/concurrent_coldboot_integration_test.go:166-178 — the daemon-death poll loop (snapshot oldPID → deadline loop of SaverPanePIDOrAbsent + pidIsAlive → return when both gone) is duplicated between killSaverSessionAndWait and reapSaverDaemon. Extract a shared `waitForDaemonGone(client, oldPID, budget) bool` helper; the two callers differ only in the preceding kill (session vs whole server) and fatal-vs-best-effort on timeout. Borderline given the author's documented Rule-of-Three stance on the setup duplication; low value.
- [quickfix] cmd/abridged_integration_test.go:64,97 — setupAbridgedEnv returns (*tmuxtest.Socket, *tmux.Client, string, []string) but every one of the six callers discards ts and envSlice with `_`. Narrow the signature to (client, stateDir) since neither returned value is consumed in this suite. Trade-off: the wider signature preserves byte-for-byte parity with setupConcurrentColdBootEnv (where ts/envSlice ARE used), which aids cross-suite readability — a defensible reason to keep it. Low value.
