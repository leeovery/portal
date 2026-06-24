TASK: spectrum-tui-design-5-4 — Step mapping: 11 real bootstrap steps → 5 friendly labels (§10.4)

ACCEPTANCE CRITERIA:
- All 11 real steps map to exactly one of the 5 friendly labels per the §10.4 table
- The bar advances on every real step (11 increments), reaching 100% only after step 11
- The active label is the friendly group of the current step; completed=done, current=active, future=pending
- Only "Restoring sessions" carries an (N/M) counter
- M=0 suppresses (N/M) and ticks "Restoring sessions" done immediately without stalling
- "Running resume commands" ticks done with no per-item work; cleanup steps 8-11 fold under it
- Non-visual plumbing/contract — vhs-exempt; verification is behavioural

STATUS: Complete

SPEC CONTEXT:
§10.4 mandates collapsing 11 cryptic bootstrap steps into 5 user-readable labels, advancing the
bar on every real step while the active label is the group the current step falls under. The §10.4
table: Started tmux server (1 EnsureServer) / Registered hooks (2-5) / Restoring sessions (N/M)
(6 skeleton per-session loop) / Replaying scrollback (6 geometry+replay, 7 EagerSignalHydrate) /
Running resume commands (on-resume hooks, 8 ClearRestoring, 9-11 cleanup). Only Restoring sessions
carries N/M (the restore loop is the one real per-item source). Empty restore (M=0): suppress
(N/M), tick ✓ immediately; Running resume commands likewise ticks ✓ with no per-item work; bar
still advances through every real step — a zero-item label is "done" not "stalled". §10.4 explicitly
permits the impl to choose which fast cleanup step (8-11) sits under which label.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/loading_progress.go (full file, the single source of truth)
- Verified the §10.4 table (stepLabelTable, loading_progress.go:84-96) matches spec §10.4 lines
  423-427 EXACTLY: step 1→Started tmux server; 2-5→Registered hooks; 6→Replaying scrollback (static
  completion mapping) with the runtime RestoreM>0 discriminator routing step-6 skeleton events to
  Restoring sessions; 7→Replaying scrollback; 8-11→Running resume commands.
- Bar advance is per DISTINCT completed step index (completedSteps set, len/11), idempotent against
  duplicate/out-of-order events (loading_progress.go:146-205, 213-226). Reaches 1.0 only after the
  11th distinct step.
- Dual-mapping of step 6 is handled correctly and the producer contract is honoured: cmd/bootstrap/
  bootstrap.go:375-400 fires every per-session callback (StepEvent Index 6, RestoreM>0) from INSIDE
  the restore loop, then the trailing emitStep(6) completion tick (zero N/M) AFTER Restore() returns.
  So a RestoreM>0 event is a reliable "step 6 not yet complete" signal — Apply updates the counter
  only and does NOT mark step 6 done; the trailing RestoreM==0 tick flips Restoring sessions to done
  and advances the bar through step 6. M=0 (no skeleton callbacks) means the only step-6 event is the
  RestoreM==0 completion tick → counter suppressed (counterFor, :343-348), label ticks done at once.
- SINGLE SOURCE OF TRUTH confirmed (the task 8-1 collapse claim verified against current code): grep
  for the five label strings across non-test .go finds the mapping encoded ONLY in loading_progress.go.
  Other hits are comments (bootstrap.go, restore.go, progress_emitter.go) and capture fixtures
  (internal/capture/fixtures.go), which drive the accumulator via BootstrapProgressMsg events rather
  than re-encoding the table. The wire message (BootstrapProgressMsg, model.go:180-184) and the
  StepEvent→msg bridge (cmd/bootstrap_progress.go:249-253) carry ONLY Index/RestoreN/RestoreM — no
  duplicate friendly Label or raw StepName rides the wire.
- Import direction is clean: internal/tui keys off the numeric Index (1..11), never imports
  cmd/bootstrap (no cycle).
- Notes: FailedView / LabelForStepIndex / LabelFailed co-located here serve the §10.5 fatal frame
  (consumed by model.go:3800), not strictly 5-4, but their co-location in the mapping file is correct
  (same single authority). Not dead code.

TESTS:
- Status: Adequate
- Location: internal/tui/loading_progress_test.go (11 test functions)
- Coverage maps 1:1 to acceptance criteria and the task's required test list:
  - TestStepMapsToFriendlyLabel — all 11 steps → §10.4 label incl. step-6 RestoreM discrimination
  - TestBarAdvancesEveryStep — 11 increments of 1/11, 100% only after step 11, initial 0
  - TestLabelStateTransitions — pending→active→done frontier progression, no active after step 11
  - TestMultiStepLabelStaysActiveUntilLastStep — Registered hooks active through step 4, done at 5
  - TestRestoringSessionsCounter — (N/M) only on Restoring sessions; mid-flight active+counter, bar
    NOT advanced by skeleton events; sticky N/M into done; no other label carries a counter
  - TestEmptyRestoreSuppressesCounterAndTicksDone — M=0 suppresses (N/M), ticks done, bar→6/11
  - TestRunningResumeCommandsTicksDoneWithNoItems — done at step 11, no counter, no stall
  - TestIdempotentPerStepIndex — duplicate (3× step 1 → 1/11) and out-of-order (3 then 2 → 2/11)
  - TestMappingCoversAllElevenStepsNoGaps — drift guard: 1..11 all map to a valid label, out-of-range
    (0/12/99) map to "" and do not advance the bar, exactly 5 labels in stable order
  - TestBootstrapProgressMsgCarriesOnlyConsumedFields — reflect-based guard that the wire message
    carries ONLY Index/RestoreN/RestoreM (pins the no-second-encoding invariant in the type itself)
- Tests are pure/transport-free (feed helper folds events through Apply); float comparisons use an
  epsilon (correct for fraction arithmetic). Not over-tested — each test targets a distinct contract
  clause; the reflect-based field guard is justified (it pins a structural invariant a behavioural
  test cannot). Not under-tested — every edge case from the task (M=0, zero-resume, multi-step label
  stays active, idempotency, out-of-range) is covered.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(); pure value-receiver accumulator (no package-level
  mutable state); exported constants mirror the labels so call sites never re-type them.
- SOLID principles: Good. Single responsibility (pure mapping accumulator, decoupled from transport
  task 5-2 and render task 5-5); table-driven mapping is open for extension; the RestoreProgressSink
  optional seam keeps the synchronous path untouched.
- Complexity: Low. labelDone/activeLabel/labelState/counterFor are each small, linear, and obvious.
- Modern idioms: Yes. Immutable Apply (returns new value via clone), map-set for idempotent distinct
  tracking, zero value is usable.
- Readability: Good — arguably exemplary. Doc comments precisely explain the step-6 dual-mapping, the
  producer ordering contract it relies on, and every degenerate case; intent is self-evident.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
