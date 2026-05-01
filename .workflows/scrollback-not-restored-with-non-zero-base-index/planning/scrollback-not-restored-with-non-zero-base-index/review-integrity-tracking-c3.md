---
status: in-progress
created: 2026-04-30
cycle: 3
phase: Plan Integrity Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Integrity

Cycle 2 verification: both cycle-2 findings were applied correctly.

- Finding 1 (task 1-2 AC names `RegisterPortalHooksWithLogger` as aggregator): present at `phase-1-tasks.md` line 109. ✓
- Finding 2 (task 2-2 Tests lists `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning` by name): present at `phase-2-tasks.md` line 99. ✓

Cycle 3 sweep surfaces two remaining propagation gaps in task 1-2 where cycle-1 / cycle-2 changes did not flow into the Tests section. Both stem from the same root: the Option-B lock-in renamed the logger-aware caller to `RegisterPortalHooksWithLogger`, but several Tests bullets still call `RegisterPortalHooks` (the no-op-logger wrapper) while asserting that a captured logger received output. Without resolution, an implementer must guess which function the test invokes.

## Findings

### 1. Task 1-2 Tests bullets call `RegisterPortalHooks` but assert captured-logger INFO/WARN output

**Severity**: Important
**Plan Reference**: `phase-1-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-1-2` → Tests section, bullets 1, 2, and 3 (`TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed`, `TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap`, `TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp`)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
After cycle 1's Option-B lock-in, `RegisterPortalHooks` is the no-op-logger wrapper that delegates to `RegisterPortalHooksWithLogger`. Three Tests bullets still call `RegisterPortalHooks` directly while simultaneously asserting INFO / WARN lines were written "to a captured logger" or that the captured logger has zero entries. With a no-op logger, no log lines can ever be captured — these assertions are unreachable as written. The implementer must decide: (a) call `RegisterPortalHooksWithLogger` with a capturing logger, (b) call `migrateHydrationHooks` directly, or (c) leave the test paths that assert log content as no-ops. This is a design call the plan should have already settled. Aligning the bullets with the Option-B shape removes the ambiguity and matches the AC contract that the no-op wrapper inherits behaviour by delegation.

The fix is to invoke `RegisterPortalHooksWithLogger` with an injected capturing `MigrationLogger` in each of the three bullets; the existing `Do` step already mandates the sibling exists, so this is a Tests-side propagation only.

**Current**:
```markdown
- `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed` (real-tmux fixture preferred via `internal/tmuxtest`):
  - Set up a tmux server.
  - For each event in `hydrationTriggerEvents`, append the legacy un-separated command verbatim: `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"` (no `--`).
  - Call `RegisterPortalHooks` once.
  - Assert that for each event, `ShowGlobalHooks` returns exactly one entry containing `portal state signal-hydrate`, and that entry contains `portal state signal-hydrate --`.
  - Assert one INFO line `evicted N stale signal-hydrate hook(s) lacking '--' separator` was written to a captured logger, with N equal to `len(hydrationTriggerEvents)`.
- `TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap`:
  - Run the same fixture; call `RegisterPortalHooks` twice in a row.
  - Assert hook state is unchanged after the second call.
  - Assert the second call's logger captured zero INFO lines and zero WARN lines.
- `TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp`:
  - Fresh server with no pre-existing entries; call `RegisterPortalHooks` once.
  - Assert install proceeds normally, eviction count is 0, no INFO and no WARN lines emitted.
```

**Proposed**:
```markdown
- `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed` (real-tmux fixture preferred via `internal/tmuxtest`):
  - Set up a tmux server.
  - For each event in `hydrationTriggerEvents`, append the legacy un-separated command verbatim: `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"` (no `--`).
  - Construct a capturing `MigrationLogger` (test-local stub recording INFO/WARN calls) and call `RegisterPortalHooksWithLogger(c, capturingLogger)` once. The no-op-logger wrapper `RegisterPortalHooks` is exercised separately by the bootstrap-adapter wiring; this test must use the logger-aware sibling so the captured-output assertions are reachable.
  - Assert that for each event, `ShowGlobalHooks` returns exactly one entry containing `portal state signal-hydrate`, and that entry contains `portal state signal-hydrate --`.
  - Assert one INFO line `evicted N stale signal-hydrate hook(s) lacking '--' separator` was written to the capturing logger, with N equal to `len(hydrationTriggerEvents)`.
- `TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap`:
  - Run the same fixture; call `RegisterPortalHooksWithLogger(c, capturingLogger)` twice in a row, resetting the capturing logger between calls.
  - Assert hook state is unchanged after the second call.
  - Assert the second call's capturing logger recorded zero INFO lines and zero WARN lines.
- `TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp`:
  - Fresh server with no pre-existing entries; call `RegisterPortalHooksWithLogger(c, capturingLogger)` once.
  - Assert install proceeds normally, eviction count is 0, and the capturing logger recorded zero INFO and zero WARN lines.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 1-2 Tests bullet still references "Option A/B shape" deliberation that was resolved by cycle 1

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-1-2` → Tests section, bullet `TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime`
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Cycle 1 finding 2 locked in Option B (the new `RegisterPortalHooksWithLogger` sibling with `MigrationLogger`). The Do section was updated, but this Tests bullet still tells the implementer to "choose whichever fits the chosen Option A/B shape" — a deliberation that has already been resolved. Reading this in isolation, an implementer would either re-litigate the choice or skip the bullet thinking the design is unsettled. Replacing the parenthetical with the locked-in shape removes the dangling deliberation and keeps the task self-contained.

**Current**:
```markdown
- `TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime`:
  - Stub or temporarily extend `hydrationTriggerEvents` (via a test-only export or by exercising a function that takes the slice as a parameter — choose whichever fits the chosen Option A/B shape).
  - Append un-separated entries on every listed event.
  - Assert all are evicted. Confirms the migration loop reads the slice rather than hard-coding events.
```

**Proposed**:
```markdown
- `TestMigrateHydrationHooks_HydrationTriggerEventsSliceIsRespectedAtRuntime`:
  - Stub or temporarily extend `hydrationTriggerEvents` (via a test-only package-internal export, since `migrateHydrationHooks` reads the package-scoped slice directly per the locked-in Option-B shape; do not introduce a parameterised public surface for this test alone).
  - Append un-separated entries on every listed event.
  - Assert all are evicted. Confirms the migration loop reads the slice rather than hard-coding events.
```

**Resolution**: Pending
**Notes**:

---
