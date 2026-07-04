TASK: skip-bootstrap-when-warm-1-3 — Remove the CleanStale step from the orchestrator (11 → 10 steps)

ACCEPTANCE CRITERIA:
- bootstrap.go has no StaleCleaner interface, Clean field, stepCleanStale const, o.Clean.CleanStale() call, or emitStep(11, ...).
- const totalSteps == 10; orchestration-complete summary emits steps=10.
- Package doc + Run/Orchestrator godoc enumerate ten steps ending at SweepOrphanFIFOs; no "eleven"/"step 11" residue.
- NoOpStaleCleaner gone from noop.go; WithClean/clean/Clean: gone from defaults.go; orchestratorOpts.Clean/WithClean gone from orchestrator_builder_test.go.
- cleanStaleAdapter + CleanStale method + _ AllPaneLister assertion + NoOpStaleCleaner fallback gone from bootstrap_production.go; Clean: gone from production literal; unused imports removed.
- run_hook_stale_cleanup.go + test unchanged; internal/hooks CleanStale store method untouched.
- Marker sweep (step 9) and FIFO sweep (step 10) keep their indices (drop, not renumber).
- go build / go test ./... / golangci-lint clean.

STATUS: Complete

SPEC CONTEXT:
The single-abridged-path constraint (spec § The Two Bootstrap Paths → Single-abridged constraint; § Daemon-Owned Hooks Cleanup → Decision) forces hooks stale-cleanup out of the orchestrator entirely — a command-classified cleanup would be the rejected multi-variant design, and keeping it in the one abridged path would run it under the 20× attach reopen burst (and re-expose the bootstrap-cleanstale-wipes-hooks transient bug). Marker/FIFO sweeps (steps 9/10) stay for cold-boot leftovers; only hooks CleanStale is removed here and re-homed on the daemon in Phase 3. This task is a mechanical drop-step-11 (11 → 10 steps), keeping the surviving sweep indices at 9 and 10. runHookStaleCleanup is retained for Phase 3.

IMPLEMENTATION:
- Status: Implemented (matches all acceptance criteria; no drift)
- Location:
  - cmd/bootstrap/bootstrap.go — Orchestrator struct fields are exactly {Server, Hooks, Restoring, OrphanSweeper, Saver, Restore, EagerSignaler, StaleMarkers, Sweeper, Latch, Version, Logger}; no Clean field, no StaleCleaner interface. const totalSteps = 10 (line 59), threaded into the "orchestration complete" summary (line 507). emitStep calls run 1..10 ending at emitStep(10, stepSweepOrphanFIFOs) (line 483); no emitStep(11). Package doc block (lines 1-33) enumerates 1..10 ending at SweepOrphanFIFOs; Run/Orchestrator godoc say "ten-step"/"ten bootstrap steps"/"steps=10". No "eleven"/"step 11" residue anywhere in the file. stepCleanStale* matches only the legitimate stepCleanStaleMarkers (step 9).
  - cmd/bootstrap/noop.go — no NoOpStaleCleaner; package doc degradable list is {Hooks, OrphanSweeper, Saver, Restore, EagerHydrateSignaler, MarkerCleaner, FIFOSweeper}.
  - cmd/bootstrap/defaults.go — no clean field / WithClean / Clean: default; defaultsConfig + defaulting policy cover the 7 remaining seams.
  - cmd/bootstrap_production.go — no cleanStaleAdapter, no CleanStale method, no _ AllPaneLister assertion, no NoOpStaleCleaner fallback, no Clean: in the &bootstrap.Orchestrator{} literal; internal/hooks import removed.
  - Retained (unchanged): cmd/run_hook_stale_cleanup.go (8 CleanStale references intact), internal/hooks/store.go CleanStale.
- Notes: Repo-wide greps for StaleCleaner, NoOpStaleCleaner, WithClean, cleanStaleAdapter (code), and .Clean/Clean: orchestrator-field references all return zero production hits. The named integration sites (cmd/reattach_integration_test.go, cmd/state_commit_now_symptom_integration_test.go) carry only unrelated t.Cleanup references — no orphaned orchestrator Clean wiring.

TESTS:
- Status: Adequate (well-retuned; includes a strong structural guard)
- Coverage:
  - cmd/bootstrap/bootstrap_test.go — stepRecorder no longer has a CleanStale method (only CleanStaleMarkers, the retained step 9). All happy-path/ordering/soft-warning expected-call slices terminate at "Sweep" (10 entries). Micro-acceptance "runs exactly ten steps ending at SweepOrphanFIFOs" is covered by TestOrchestratorRun_executesStepsInSpecOrder. Ordering tests renamed and retuned to assert Clear < CleanStaleMarkers < Sweep with Sweep as the final recorded step (TestOrchestratorRun_runsSweepAsFinalStepAfterClearAndCleanStaleMarkers, TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep). "reports steps=10" covered by TestOrchestratorRun_emitsOrchestrationCompleteOnCleanBootstrap (asserts steps=10) + closedStepNames (10 names) driving the step-complete count. The "CleanStale must still run after X fails" resilience tests are gone.
  - cmd/bootstrap/defaults_test.go — no Clean assertion / stubStaleCleaner / WithClean; default-wiring assertions cover the 7 remaining degradable seams.
  - cmd/bootstrap/orchestrator_builder_test.go — orchestratorOpts drops the Clean field; the opts.Clean forwarding block is gone.
  - cmd/bootstrap/progress_emitter_test.go — StepEvent expectations run 1..10 ending at stepSweepOrphanFIFOs.
  - cmd/bootstrap_production_test.go — TestCleanStaleAdapter / cleanStaleAdapterT mirror removed (doc block records the removal); retained helpers feed run_hook_stale_cleanup_test.go.
  - cmd/hooks_cleanstale_single_caller_guard_test.go — NEW structural guard directly satisfying AC5 ("the daemon's throttled cleanup is the only hooks-CleanStale caller left"): walks cmd/bootstrap production source and fails on any \bCleanStale\b / \brunHookStaleCleanup\b reference, with a word-boundary that correctly excludes CleanStaleMarkers. Vacuity-guarded (fails if zero files scanned).
- Notes: Not over-tested — assertions are focused; the removed resilience tests were correctly dropped. Not under-tested — ten-step ordering, steps=10 summary, and the single-caller guard are all present. The steps=10 summary test uses the in-package RecordingLogger rather than logtest.Sink named in the plan; functionally equivalent (it pins the steps=10 attr on the single orchestration-complete INFO), so this is a benign substitution, not a gap.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel in the cmd package tests; interface-segregation seam pattern preserved (the StaleCleaner seam removed cleanly, no dangling nil-guards); NewWithDefaults centralisation intact.
- SOLID principles: Good. Orchestrator struct and defaulting policy stay coherent after the seam removal.
- Complexity: Low. Pure deletion + doc/index bookkeeping; surviving indices unchanged.
- Modern idioms: Yes.
- Readability: Good. Doc comments were carried to a consistent ten-step story.
- Issues: One stale doc reference (see non-blocking notes) — cosmetic only.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/bootstrap_production.go:5,7 — the file-level doc block still lists "internal/hooks" in both the "wire ... across internal/tmux, internal/restore, internal/state, and internal/hooks" sentence and the "stays free of dependencies on internal/restore, internal/state, and internal/hooks" sentence. cleanStaleAdapter was the only internal/hooks consumer in this file; with it removed the file no longer imports or references internal/hooks (grep confirms both matches are comment-only). Drop "and internal/hooks" from both sentences so the doc matches the file's actual dependency set. Pure doc edit, single file, zero logic impact.
