TASK: Add ctx.Done() check between per-pane iterations (observation point 3) + cancel-mid-loop test

ACCEPTANCE CRITERIA:
- captureAndCommit checks ctx.Done() at the first statement inside the innermost pane loop body.
- On cancellation observed mid-loop, the function returns nil (not an error).
- state.Commit is not invoked when cancellation is observed mid-loop.
- deps.PrevIndex is not replaced when cancellation is observed mid-loop.
- Per-pane scrollback writes for completed iterations are NOT rolled back (atomic, by design).
- Unit test on multi-pane fixture pins mid-loop cancel; regression guard confirms uncancelled full run.
- Phase 2 prior-task tests remain green.

STATUS: Complete

SPEC CONTEXT: Spec § Change 2 (Cancellation semantics, point 3) — third ctx.Done() observation between per-pane iterations caps worst-case SIGHUP-to-exit latency at one pane's capture-pane wall time. Per-pane scrollback writes are atomic and not rolled back; the no-partial-commit invariant is about sessions.json only.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:167-178
- Non-blocking `select { case <-ctx.Done(): return nil; default: }` placed at the very first statement inside the innermost for _, pane := range win.Panes { loop (line 166).
- Returns nil (not an error) — consistent with observation points 1 and 2; avoids spurious WARN in tick.
- Comment (lines 167-173) documents non-rollback semantics for atomic per-pane writes and references spec § Change 2 as required.
- Placement precedes paneKey resolution, skipSet check, CaptureAndHashPane, and WriteScrollbackIfChanged — so the cancellation observation suppresses all per-pane side effects on the iteration that observes it.

TESTS:
- Status: Adequate
- Coverage:
  - TestCaptureAndCommit_CancelMidLoopAfterKofNPanesProcessed (cmd/state_daemon_run_test.go:1130): 1 session × 1 window × 3 panes; dispatchHook calls cancel() after the first capture-pane invocation; asserts nil return, capture-pane called ≥1 and <3 times, PrevIndex pointer preserved (via assertNoCommit), sessions.json not written.
  - TestCaptureAndCommit_UncancelledMultiPaneFixtureProcessesAllPanesAndCommits (line 1197): same fixture without cancellation; asserts nil return, capture-pane called exactly 3 times, sessions.json written and decodable, PrevIndex pointer replaced via assertCommitReplacedPrev.
- Notes: Tests use shared makeDeps/assertNoCommit helpers consistent with Tasks 2-2/2-3.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(), uses the package-level fake commander seam (dispatchHook) per the cmd-package mock injection pattern noted in CLAUDE.md.
- SOLID principles: Good. captureAndCommit single responsibility preserved.
- Complexity: Low. Three identical select/default patterns across the three observation points — consistent and easy to read.
- Modern idioms: Yes. Canonical Go non-blocking ctx check via select/default.
- Readability: Good. Each observation point has a numbered comment ("observation point N of 3"), and the mid-loop comment explicitly documents the non-rollback semantics.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The cancel-mid-loop test's capture-call count uses `< 3` (allows 1 or 2). In practice the dispatch-hook timing yields exactly 1 call. A tighter `== 1` assertion would more precisely pin the contract.
- [idea] The mid-loop test does not include an explicit filesystem assertion that per-pane scrollback files MAY remain on disk for completed iterations. The non-rollback invariant is captured in prose but not pinned by an assertion. Low priority.
