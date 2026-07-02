---
status: complete
created: 2026-07-02
cycle: 1
phase: Plan Integrity Review
topic: Skip Bootstrap When Warm
---

# Review Tracking: Skip Bootstrap When Warm - Integrity

## Findings

### 1. Task 1-2 latch-write insertion point mislabels SweepOrphanFIFOs as "the last soft step" while CleanStale still exists pre-1-3

**Severity**: Minor
**Plan Reference**: Phase 1, task `skip-bootstrap-when-warm-1-2` (Set the latch as the final action of a successful Orchestrator.Run) — the second bullet of the **Do** section.
**Category**: Task Self-Containment / Dependencies and Ordering
**Change Type**: update-task

**Details**:
Task 1-2's **Do** bullet instructs the implementer to insert the latch write "after the `emitStep(10, stepSweepOrphanFIFOs)` call (the last soft step) and before the `o.Logger.Info("orchestration complete", ...)` summary line." The parenthetical "(the last soft step)" is only true *after* task 1-3 removes step 11 (CleanStale). Tasks in this phase execute in natural (creation-date) order — 1-1 → 1-2 → 1-3 → 1-4 — so at the moment 1-2 runs, the live `cmd/bootstrap/bootstrap.go` still contains Step 11 (CleanStale) at lines ~458-466, running *after* `emitStep(10, stepSweepOrphanFIFOs)` (line 456) and before the Return boundary. An implementer following the literal instruction while 1-2 executes first would place the latch write between SweepOrphanFIFOs and the still-present CleanStale step — not as `Run`'s genuine final pre-return action.

Impact is low and self-correcting: CleanStale is a best-effort, non-aborting step, so a latch written just before it is harmless in the intermediate state, and once task 1-3 deletes CleanStale the latch write becomes correctly terminal with no further edit. The task already anticipates the co-evolution with 1-3 in one place (its Context notes "the `steps=10` in the orchestration-complete summary is task 1-3's change; this task's ordering assertion should tolerate whichever `totalSteps` value is live when it runs"), but the **Do** insertion-point bullet does not carry the same acknowledgement — it asserts SweepOrphanFIFOs is "the last soft step" as if step 11 were already gone. The fix is a wording clarification that (a) drops the inaccurate "(the last soft step)" absolute and (b) instructs placing the write immediately before the Return / orchestration-complete summary, explicitly tolerating a still-present CleanStale step if 1-2 lands first.

This is the only structural finding; the rest of the plan meets the integrity bar (every task carries a full template with concrete pass/fail acceptance criteria and edge-case tests, slices are vertical and independently testable, phase progression is logical, and the single cross-phase dependency 3-3 blocked_by 1-3 is present and correct).

**Current**:
```
- In `Run`, after the `emitStep(10, stepSweepOrphanFIFOs)` call (the last soft step) and before the `o.Logger.Info("orchestration complete", ...)` summary line, add the latch write. Because all fatal steps (`EnsureServer`, `RegisterPortalHooks`, `SetRestoring`, `ClearRestoring`) `return` early via `o.fatalf`, execution only reaches this point on a non-fatal run — so no extra error gate is needed; add a one-line comment stating this. Guard the call `if o.Latch != nil { ... }` so tests / fallbacks may leave it nil.
```

**Proposed**:
```
- In `Run`, insert the latch write as the final action before the `o.Logger.Info("orchestration complete", ...)` summary line and the `return` — i.e. after the last best-effort step and after the fatal-error gate. **Ordering note (co-evolution with task 1-3):** if this task lands before 1-3, the live `Run` still contains Step 11 (`CleanStale`) after `emitStep(10, stepSweepOrphanFIFOs)`; place the latch write *after* that CleanStale block (immediately before the Return boundary), NOT immediately after `emitStep(10, …)` — the write must be `Run`'s last pre-return action so it never precedes a soft step that is still present. Once 1-3 removes CleanStale, the write is already correctly terminal at `emitStep(10, …)`'s tail with no further edit needed. Because all fatal steps (`EnsureServer`, `RegisterPortalHooks`, `SetRestoring`, `ClearRestoring`) `return` early via `o.fatalf`, execution only reaches this point on a non-fatal run — so no extra error gate is needed; add a one-line comment stating this. Guard the call `if o.Latch != nil { ... }` so tests / fallbacks may leave it nil.
```

**Resolution**: Fixed
**Notes**: Applied verbatim (auto mode) to both the phase-1 task detail file and the Tick task tick-f09675.

---
