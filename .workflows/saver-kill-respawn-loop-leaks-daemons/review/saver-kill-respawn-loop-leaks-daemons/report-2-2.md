TASK: Add ctx.Done() check at captureAndCommit entry (observation point 1: pre-enumeration) with cancel-before-first unit test

ACCEPTANCE CRITERIA:
- captureAndCommit checks ctx.Done() at its first statement before any other work.
- On a pre-cancelled context, returns nil (not an error).
- state.ListSkeletonMarkers is not invoked when ctx is already cancelled at entry.
- state.CaptureStructure is not invoked.
- No state.Commit invocation occurs.
- deps.PrevIndex is not mutated.
- deps.LastSaveAt is not mutated (asserted indirectly via the function's no-write contract).
- Unit test named to describe "cancel before first per-pane iteration" pins the behaviour.
- Existing tests remain green.

STATUS: Complete

SPEC CONTEXT: Spec § Change 2 mandates three ctx.Done() observation points inside captureAndCommit; this task installs point 1 at function entry, before state.ListSkeletonMarkers. Cancellation must be a clean abandon: return nil so tick does not emit a WARN, no partial commit of sessions.json, no per-pane work fired.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:132-143 — `select { case <-ctx.Done(): return nil; default: }` placed as the first statement inside captureAndCommit, ahead of state.ListSkeletonMarkers at line 144.
- Notes:
  - Uses canonical non-blocking select { default: } pattern as required (not `if ctx.Err() != nil`).
  - Returns nil on cancellation, suppressing tick's WARN log path (line 110-112).
  - Documentation comment above the select cites spec § Change 2 and explains the nil-return rationale.

TESTS:
- Status: Adequate
- Coverage: cmd/state_daemon_run_test.go:965 TestCaptureAndCommit_PreCancelledCtxReturnsImmediately
  - Seeds a multi-pane fixture (sessionsOut/panesOut/captureByTarget) so any leak past the cancel check would produce observable commander activity.
  - Seeds deps.PrevIndex with sentinelIndex("sentinel-must-be-preserved") to detect pointer replacement.
  - Cancels context before calling captureAndCommit.
  - Asserts: nil return; zero show-options (ListSkeletonMarkers), zero list-sessions / list-panes (CaptureStructure), zero capture-pane calls; assertNoCommit verifies PrevIndex pointer unchanged and sessions.json absent.
  - Live-ctx regression guard present at line 882.
- Notes:
  - Test landed in cmd/state_daemon_run_test.go rather than cmd/state_daemon_test.go — correct, matches where the daemon-tick fake commander harness lives.
  - LastSaveAt non-mutation not asserted directly; spec task-2-2 edge-case explicitly accepts this.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel (matches CLAUDE.md). DI via daemonFakeCommander matches package idiom.
- SOLID: Good. Localised non-blocking guard, no abstraction churn.
- Complexity: Low.
- Modern idioms: Yes. Canonical Go non-blocking ctx-cancellation pattern.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Plan text references cmd/state_daemon_test.go as test home; tests correctly landed in cmd/state_daemon_run_test.go. Plan template could be updated.
