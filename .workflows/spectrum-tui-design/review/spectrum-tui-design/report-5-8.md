TASK: spectrum-tui-design-5-8 — Restore/daemon race review against the live event loop + startup-ordering integration-test updates (prior-incident surface)

ACCEPTANCE CRITERIA (from tick-28b9cb / phase-5 plan):
- Restore/daemon interaction against the live event loop reviewed; analysis recorded durably (race-safe, or any found race fixed)
- Ordering invariants (@portal-restoring window, sweep-before-saver, daemon.lock singleton, self-supervision) confirmed to hold under the concurrent route
- Live event loop performs no tmux/state mutation while the orchestrator runs (TUI inert during loading), asserted by a test
- Startup-ordering integration tests pass for the concurrent cold boot (step ordering preserved, daemon singleton, no zombie/leaked daemon, no slow-open regression)
- Warm-path synchronous-ordering parity assertion exists and passes
- Every daemon-spawning test uses IsolateStateForTest(t) with env applied to every subprocess; no t.Parallel()
- vhs-exempt (non-visual risk-review/test task); kill-barrier flake re-run in isolation before treating any timing failure as a regression

STATUS: Complete

SPEC CONTEXT:
§10.2 (the startup flip) runs the eleven-step orchestrator (Restore, EnsureSaver/daemon bootstrap, EagerSignalHydrate, FIFO/marker cleanup) in a goroutine while the Bubble Tea event loop is live — the exact interaction surface behind the prior slow-open / empty-previews / zombie-session incident. The spec's stated containment property: "the TUI is inert during loading (animation only) — this contains the race surface." The ordering invariants the synchronous bootstrap guaranteed (the @portal-restoring Set step 3 / Clear step 8 suppression window, SweepOrphanDaemons step 4 before EnsureSaver step 5, the daemon.lock flock singleton + Component C pre-check, daemon self-supervision hysteresis) must survive the concurrent route. CLAUDE.md mandates IsolateStateForTest for every daemon-spawning test and forbids t.Parallel() in the cmd package (package-level mutable mock state).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/bootstrap_progress.go:17-65 — the durable in-source Part A race-review finding: all four ordering invariants traced against the concurrent route, each documented HOLDS with the reason (every invariant is internal to Orchestrator.Run's single goroutine; none depended on TUI absence; the containing property is TUI inert-during-loading).
  - cmd/bootstrap/bootstrap.go:274-286 — Run resolves the §10.2 emitter once; emit is a no-op on the synchronous route, so the step body is byte-for-byte identical on both routes (substantiates the "ordering unchanged" finding).
  - internal/tui/model.go:1807-1828 — refetchSessionsAfterRestore (the one REAL fix, Part B): post-complete session re-enumeration scoped to the concurrent route (progressReceiver != nil); nil/no-op on the warm route.
  - internal/tui/model.go:2002-2058 — BootstrapCompleteMsg / LoadingMinElapsedMsg arms dispatch the re-fetch only when the LATER of the two gates closes; fatalActive guards prevent any flip into a half-restored picker.
  - internal/tui/model.go:2201-2213 — the PageLoading key arm: only Ctrl+C quits unconditionally; q/Esc quit only when fatalActive; every other key returns (m, nil) — the inert property in code.
- Notes: The Part A analysis is accurate. I independently verified the byte-for-byte-identical-Run claim (Run differs across routes only by a nil-vs-non-nil emitter callback at each step site) and the inert-key-arm claim. No genuine race exists; the only fix needed (cold-boot session staleness) is implemented and correctly route-scoped.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/concurrent_coldboot_integration_test.go (Part D, build-tagged integration, REAL tmux + REAL saver daemon, drives the production bootstrapProgressPipe):
    * TestConcurrentColdBoot_StepOrderingAndDaemonSingleton — slow-restore shape (seeded sessions.json): 11 steps in canonical order, serverStarted=true, @portal-restoring cleared, daemon singleton (pgrep + saver-pane PID agree, no zombie), restored sessions live (empty-previews surface proven absent).
    * TestConcurrentColdBoot_FastEmptyRestore_NoZombie — M=0 fast shape: orchestrator finishes near first render, singleton intact. Covers the spec's "very fast cold boot, M=0" edge case.
    * TestConcurrentColdBoot_RestoringWindowSetBeforeRestore — wraps the RestoreAdapter to observe @portal-restoring SET at the instant step 6 runs (the Set-before-restore half), paired with assertRestoringCleared (the Clear-before-cleanup half).
    * TestConcurrentColdBoot_WarmParity_NoLoadingPageSynchronousOrdering — warm boot (pre-warmed server) takes the synchronous path: no deferred bootstrap stashed, Run called once, serverStarted=false threaded (no loading page), 11-step canonical order.
  - internal/tui/inert_during_loading_test.go (Part C): a key + progress storm pumped through Update while parked on PageLoading drives every mutating seam (kill/rename/create/attach/enum) as a recording mock; asserts ZERO mutations and ZERO ListSessions calls from the loading-page key arm. Directly asserts the race-containment property.
  - internal/tui/coldboot_session_refetch_test.go (Part B): post-complete re-fetch reflects restored sessions (not the empty Init snapshot); fast-boot ordering (complete-before-min-elapsed → re-fetch fires when the later gate closes); warm route does NOT re-fetch (parity).
  - cmd/concurrent_bootstrap_gate_test.go / concurrent_bootstrap_route_test.go — non-integration siblings pinning shouldRunConcurrentBootstrap classification and the cold-defers / warm-synchronous / cold-CLI-synchronous routing.
- Notes: Coverage maps 1:1 onto the task's six named tests and the spec edge cases (M=0 fast; slow restore; complete-before-min-elapsed; no slow-open regression; no leaked daemon). The drain budget (15s) fails loudly on a wedged goroutine (the slow-open shape). assertNoExtraDaemons is the EXPLICIT no-leaked-daemon check, correctly noted as load-bearing over the IsolateStateForTest backstop (defence-in-depth). Not over-tested: each test isolates a distinct invariant; the slow/fast split is justified by the M=0 vs M>0 edge cases the spec calls out.

ISOLATION DISCIPLINE (load-bearing):
- cmd/concurrent_coldboot_integration_test.go:84-93 calls portaltest.IsolateStateForTest(t), pins PORTAL_STATE_DIR on every subprocess via t.Setenv (the tmux server — and thus the saver-pane daemon it spawns — inherits it; tmuxtest's exec.Command sets no cmd.Env so it inherits the modified parent env). HISTFILE→/dev/null routes shell-history writes away from the HOME tempdir (teardown-race mitigation). reapSaverDaemon (l.155-177) blocks until the daemon process is dead (pane absent AND daemon.pid PID dead) before t.TempDir RemoveAll — the tmux-pane analogue of the SIGKILL+Wait reap, load-bearing on macOS.
- No t.Parallel() anywhere in cmd/ or internal/tui/ (every grep hit is a "No t.Parallel()" comment, not a call).

CODE QUALITY:
- Project conventions: Followed. Small DI seams (Restorer, RestoreProgressSink, Runner); package-level mutable mock state restored via t.Cleanup; ForTest-suffixed export (ProgressEmitterFromContextForTest) is the idiomatic Go test-only seam over the unexported progressEmitterFromContext, with production using the unexported path. vhs-exempt per spec (non-visual task) — correctly carries no tape.
- SOLID principles: Good. refetchSessionsAfterRestore is a single-responsibility route-scoped helper; the progress pipe cleanly separates the channel/goroutine/terminal-return concerns.
- Complexity: Low. The drain loop, gate logic, and re-fetch scoping are each linear and well-commented.
- Modern idioms: Yes. ctx-guarded select for non-blocking send-on-cancel; signal-0 liveness probe; happens-after channel-close for the unsynchronised terminal-return reads (documented at bootstrap_progress.go:130-137).
- Readability: Good. The Part A finding is exemplary durable-analysis-in-source; test headers state the invariant each file pins.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [bug] cmd/state_daemon_integration_test.go:178-179,254-256 — this pre-existing daemon-spawning integration test isolates via raw t.TempDir() + t.Setenv("PORTAL_STATE_DIR") + daemon.Env=append(os.Environ(), "PORTAL_STATE_DIR="+stateDir) rather than portaltest.IsolateStateForTest(t). State writes are redirected to the tempdir (so the dev install is not corrupted), but this bypasses both the fingerprint-diff backstop AND the XDG_CONFIG_HOME scrub that CLAUDE.md mandates for daemon-spawning tests — a latent gap (a daemon path that reads XDG_CONFIG_HOME rather than PORTAL_STATE_DIR could still touch the dev config). Pre-existing, not introduced by 5-8, and out of this task's "updated for concurrent boot" scope; flagging because the task's discipline criterion says "every daemon-spawning test." reattach_integration_test.go uses the same raw-env pattern (it does not spawn `portal state daemon` directly — its saver step is NoOp — so the risk is lower there).
- [quickfix] cmd/concurrent_coldboot_integration_test.go:236-238 — driveConcurrentColdBoot spawns a fresh goroutine per receive (go func(){ got <- receiver() }()); on the deadline t.Fatalf path the in-flight goroutine stays blocked on receiver() until the pipe goroutine's deferred close(p.ch) unblocks it. Benign in a failing test (the process is exiting), but a single long-lived receiver goroutine draining into a channel the select reads would avoid the per-iteration goroutine churn and the blocked-on-fatal leak. Optional tidy.
