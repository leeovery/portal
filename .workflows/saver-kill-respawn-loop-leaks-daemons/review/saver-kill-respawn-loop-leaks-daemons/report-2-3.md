TASK: Add ctx.Done() check post-enumeration, pre-first-iteration (observation point 2 of 3)

ACCEPTANCE CRITERIA:
- captureAndCommit checks ctx.Done() immediately after state.CaptureStructure returns successfully and before the per-pane loop begins
- On cancellation observed at this point, returns nil
- No iteration of for _, sess := range idx.Sessions begins
- state.CaptureAndHashPane / WriteScrollbackIfChanged / state.Commit not invoked
- deps.PrevIndex not replaced with freshly captured idx
- Unit test pins behaviour; prior Phase 2 and existing tests remain green

STATUS: Complete

SPEC CONTEXT: Spec §Change 2 (Cancellation semantics) mandates three ctx.Done() observation points inside captureAndCommit. This task installs point #2 between CaptureStructure returning and the first per-pane iteration, so cancellations during the (fast but non-instant) CaptureStructure subprocess call cause an early return without per-pane work or Commit.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go lines 154-161 (select block) preceded by comment at lines 154-156
- Sits exactly between CaptureStructure's err-check (lines 149-152) and the anyScrollbackChanged := false / for-loop (lines 163-164)
- Returns nil (consistent with point 1; avoids spurious WARN line in tick())
- PrevIndex / anyScrollbackChanged untouched on this path; assignment `deps.PrevIndex = &idx` happens only post-Commit at line 205 (unreachable)
- Notes: Placement matches plan's "Do" instruction verbatim. Comment cites spec §Change 2.

TESTS:
- Status: Adequate
- Test: TestCaptureAndCommit_CancelDuringCaptureStructureReturnsBeforePerPaneWork at cmd/state_daemon_run_test.go lines 1034-1109
- Mechanism: dispatchHook in daemonFakeCommander fires cancel() when CaptureStructure's terminal subcall (show-environment) is dispatched, so CaptureStructure returns successfully with a populated multi-pane idx, and the post-enumeration check then observes ctx.Done() before the loop
- Multi-pane fixture (2 sessions × 3 panes) guarantees a meaningful leak surface — without the check, three capture-pane calls would land
- Assertions cover every required invariant:
  1. returns nil
  2. CaptureStructure ran to completion — list-sessions, list-panes, show-environment all observed
  3. capture-pane call count == 0 → CaptureAndHashPane not invoked
  4. zero scrollback files written for all 3 fixture panes → WriteScrollbackIfChanged not invoked
  5. assertNoCommit covers PrevIndex pointer preservation + sessions.json absence (proves Commit not invoked)

CODE QUALITY:
- Project conventions: Followed. Pattern mirrors observation point 1 exactly. File-level "MUST NOT use t.Parallel" honoured.
- SOLID: Good. Check sits at the natural seam.
- Complexity: Low. Three-line idiomatic non-blocking select.
- Modern idioms: Yes.
- Readability: Good. Four-line comment explains what / why / timing-distinction from point 1.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Plan's "Do" says add the test in cmd/state_daemon_test.go; in practice it landed in cmd/state_daemon_run_test.go alongside the daemonFakeCommander / sentinelIndex / assertNoCommit infrastructure (same home as the Task 2-1 happy-path regression). This is the better location.
- [idea] The dispatchHook fires cancel() on every show-environment call (one per session in the fixture, so 2 invocations). cancel() is idempotent so this is harmless. Very low value.
