TASK: killed-sessions-resurrect-on-restart-1-8 — Integration test asserting daemon captureAndCommit resumes for previously-stuck-marker panes (AC4)

ACCEPTANCE CRITERIA:
- AC4 (spec): Scrollback save resumes for previously-stuck-marker panes — daemon `captureAndCommit` no longer indefinitely skips any live pane.
- Edge cases: sub-test extends task 1-6's file under //go:build integration; capture tick must run post-Clear @portal-restoring; expose state.RunCaptureOnce as a test seam if not present.

STATUS: Complete

SPEC CONTEXT: AC4 covers Symptom C — timed-out helper leaves marker set, daemon's skip-save guard suppresses scrollback save. AC4 satisfied by Fix 1. AC8 invariant: eager step runs while @portal-restoring still set; capture tick must happen after step 7 Clear.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:260-400 — `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4`. File carries //go:build integration — extends task 1-6's file.
- Daemon-tick simulation: `runDaemonTick` at /Users/leeovery/Code/portal/cmd/bootstrap/daemon_tick_test_helpers_test.go:86-149, mirroring production captureAndCommit at cmd/state_daemon.go:115-158.
- Eager-signal wiring is real: `buildIntegrationOrchestrator` auto-defaults EagerSignaler to real *EagerSignalCore when real Restore adapter is supplied.
- Ordering: test calls runDaemonTick only after o.Run returns; step 7 Clear runs inside Run — capture tick post-Clear, satisfying edge case.

TESTS:
- Status: Adequate
- Coverage:
  - N=2 fixture (alpha, beta) — beta is the deterministic-bug-scope non-attached session.
  - Restore-sanity guard prevents vacuous skip-save-free pass.
  - Waits for marker clearance via WaitForSkeletonMarkersCleared (2s, 50ms tick).
  - Forces \n-terminated record into beta via SendKeys + waitForPaneText.
  - Single daemon-equivalent tick with production-shape skip-guard ON.
  - Assertion via `state.TailScrollback(...) → (nil, nil)` contract: pre-fix shape (file absent/empty/unterminated) converges on tail==nil; post-fix yields non-nil bytes.
- Diagnostics: dumpPortalLogOnFailure distinguishes CI-load flake from AC4 regression.

CODE QUALITY:
- Project conventions: Followed — //go:build integration, no t.Parallel, real-tmux fixture.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Excellent. Docstring at lines 260-292 enumerates pre-fix vs post-fix shapes.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The plan's edge case "expose state.RunCaptureOnce as a test seam if not present" was not taken. Instead `runDaemonTick` mirrors production captureAndCommit byte-for-byte. Drift risk: any future change to production captureAndCommit must be mirrored in helper or AC4 could pass under broken production tick.
- [idea] Consider adding an inline negative-control sub-test that wires NoOpEagerHydrateSignaler{} and asserts beta's scrollback file is absent — symmetric with TestScrollbackResumption_WithoutCleanupScrollbackNotSaved.
- [quickfix] Unrelated comment-leader typos in /Users/leeovery/Code/portal/internal/state/scrollback_tail.go lines 73 and 80 use `/` instead of `//`.
