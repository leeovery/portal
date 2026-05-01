---
status: complete
created: 2026-04-30
cycle: 1
phase: Plan Integrity Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Integrity

## Findings

### 1. `savedPanePos` symbol omitted from Phase 2 plan-level pre-deletion grep AC

**Severity**: Minor
**Plan Reference**: `planning.md` Phase 2 → Acceptance, first checkbox
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 2-1's `Do` step lists `savedPanePos` as a deletion target (the struct paired with `flattenSavedPanePositions`), and task 2-1's first AC repeats this. However Phase 2's plan-level acceptance criteria item 1 (the pre-deletion grep gate) only lists `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, and `readIndexOption` — `savedPanePos` is missing. Because the phase-level AC is what reviewers and the implementer use as the canonical pre-flight check, the omission risks the struct surviving the deletion (it would still compile if its only consumer `flattenSavedPanePositions` is also gone, but a leftover unused-type definition is exactly the dead-code residue this phase aims to eliminate). Aligning the phase-level grep list with the task's deletion list keeps the two consistent.

**Current**:
```markdown
- [ ] Pre-deletion repo-wide grep confirms zero remaining references to `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, and `readIndexOption` outside the deletion list; any unexpected reference is surfaced for review rather than silently deleted.
```

**Proposed**:
```markdown
- [ ] Pre-deletion repo-wide grep confirms zero remaining references to `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, `savedPanePos`, and `readIndexOption` outside the deletion list; any unexpected reference is surfaced for review rather than silently deleted.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 1-2 leaves Option A/B logger-wiring choice unresolved at plan time

**Severity**: Important
**Plan Reference**: `phase-1-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-1-2` → `Do` section
**Category**: Task Self-Containment / Scope and Granularity
**Change Type**: update-task

**Details**:
Task 1-2's `Do` section presents two alternative shapes for threading a logger into `RegisterPortalHooks` (Option A: extend the existing signature; Option B: add a sibling `RegisterPortalHooksWithLogger`). The choice criterion ("minimises blast radius") and the deciding question ("does `*state.Logger` import cleanly into `internal/tmux`") both push a non-trivial design decision onto the implementer mid-task — exactly the situation `task-design.md` warns against ("each task contains all context needed for execution... no task requires the implementer to make design decisions").

The import boundary question is decidable now: `internal/tmux` already imports `internal/state` indirectly via existing helpers (or the codebase's small-interface DI pattern would dictate Option B). Either way, the plan should commit to one shape so the task is a single TDD cycle rather than a design-then-build cycle. If the import is clean, pick Option A. If a cycle exists, mandate Option B with the named interface (`MigrationLogger`) defined at a specified location.

**Current**:
```markdown
- Update `RegisterPortalHooks(c *Client)` to accept (or be wrapped to provide) the logger seam needed by the migration. Two acceptable shapes — choose the one that minimises blast radius:
  - **Option A**: extend `RegisterPortalHooks` to take a logger argument and update call sites in `internal/bootstrapadapter/` accordingly. Preferred if `*state.Logger` already imports cleanly into `internal/tmux`.
  - **Option B**: add a sibling `RegisterPortalHooksWithLogger(c *Client, log MigrationLogger) error` and have the existing `RegisterPortalHooks` delegate to it with a no-op logger. The bootstrap adapter calls the sibling. Pick this if the existing `RegisterPortalHooks` has external callers that would otherwise need updating.
  - In either case, `migrateHydrationHooks` runs **before** the per-category register loop reaches the hydration-trigger category. The simplest placement: call `migrateHydrationHooks` once at the top of `RegisterPortalHooks`/`RegisterPortalHooksWithLogger` body, before the existing loop.
```

**Proposed**:
```markdown
- Add a sibling `RegisterPortalHooksWithLogger(c *Client, log MigrationLogger) error` in `internal/tmux/hooks_register.go` and have the existing `RegisterPortalHooks(c *Client) error` delegate to it with a no-op logger. The bootstrap adapter (`internal/bootstrapadapter`) is updated to call the sibling and pass the bootstrap-step `*state.Logger`. This shape is chosen over extending `RegisterPortalHooks` directly because (a) it keeps the no-logger external-caller surface stable for any consumer that does not need migration logs, (b) it avoids forcing every existing call site to thread a logger through, and (c) it keeps `internal/tmux` free of an `internal/state` dependency by virtue of the small in-package `MigrationLogger` interface (`Info(component, format string, args ...any)` / `Warn(component, format string, args ...any)`) which `*state.Logger` satisfies structurally.
- `migrateHydrationHooks` is called once at the top of `RegisterPortalHooksWithLogger`, before the per-category register loop reaches the hydration-trigger category.
```

**Resolution**: Fixed
**Notes**:

---

### 3. Task 1-3 "Do" step 11 punts production-adapter discovery to implementation time

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-1-3` → `Do` step 11
**Category**: Task Self-Containment

**Change Type**: update-task

**Details**:
Step 11 says "Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass a real hooks adapter that calls `RegisterPortalHooks`... (`bootstrapadapter.HooksAdapter{...}` or whatever the production seam is named — discover via grep at implementation time)." The CLAUDE.md package map identifies `internal/bootstrapadapter` as the production wiring package, and Phase 1 (this plan) is itself adding the logger-aware sibling — so the seam name is knowable now. Punting to grep-time forces the implementer to context-switch mid-task and risks two implementers picking different names. Naming the concrete type up front keeps the task self-contained.

**Current**:
```markdown
  11. Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass a real hooks adapter that calls `RegisterPortalHooks` (so the migration code from Task 2 and the `--` separator from Task 1 actually run). The existing test wires `bootstrap.NoOpHooks{}`; this new test must use the production wiring (`bootstrapadapter.HooksAdapter{...}` or whatever the production seam is named — discover via grep at implementation time).
```

**Proposed**:
```markdown
  11. Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass the real hooks adapter from `internal/bootstrapadapter` that calls `RegisterPortalHooksWithLogger` (so the migration code from Task 1-2 and the `--` separator from Task 1-1 actually run). The existing test wires `bootstrap.NoOpHooks{}`; this new test must substitute the production wiring. If the adapter type's exported name has changed during Task 1-2 implementation, update this step to match — the load-bearing requirement is "use the same wiring the production `cmd/root.go` `PersistentPreRunE` uses", not the literal type name.
```

**Resolution**: Fixed
**Notes**:

---

### 4. Task 2-2 "Do" mandates a manual code-injection sanity check that has no committable artifact

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-2-2` → `Do` final bullet and `Acceptance Criteria` item 8
**Category**: Acceptance Criteria Quality

**Change Type**: update-task

**Details**:
The final `Do` bullet and AC item 8 require the implementer to "temporarily reintroduce a fake `WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1` log line via the test's logger... and confirm the assertion fails. Remove the temporary injection before committing." This is process guidance with no committable artifact and no way for a reviewer to verify it was performed (the injection must be removed before commit). It mixes verifiable test behaviour with manual ritual.

A stronger, verifiable equivalent is to add a unit test directly against the regex itself: a tiny test that feeds two strings into the compiled regex and asserts (a) a known offending shape matches, (b) the preserved `armPanes:202` warning shape ("`live pane count %d != saved count %d`") does not match. This proves the regex is meaningful and false-positive-safe at every CI run, without manual ritual.

**Current**:
```markdown
- Verify the assertion is meaningful: temporarily reintroduce a fake `WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1` log line via the test's logger (or a one-line manual injection) and confirm the assertion fails. Remove the temporary injection before committing.
```

And:
```markdown
- [ ] Manual sanity-check: temporarily injecting a fake `predicted=...__0.0 live=...__1.1` line into `portal.log` causes the assertion to fail (verify locally; do not commit the injection).
```

**Proposed**:
Replace the `Do` bullet with:
```markdown
- Verify the regex is meaningful and false-positive-safe with a unit test (in the same `_test.go` file or a sibling `cmd/bootstrap/predicted_vs_live_regex_test.go`) named `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`. The test compiles the same `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+` regex used by the integration assertion and asserts:
  - It matches a representative offending line (`WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1`).
  - It does NOT match the preserved `armPanes:202` shape (e.g. `WARN | restore | session "alpha": live pane count 2 != saved count 3`).
  This unit test is plain `go test ./cmd/bootstrap/...` (no `integration` build tag) so it runs on every CI invocation without requiring tmux.
```

Replace AC item 8 with:
```markdown
- [ ] A unit test (`TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`) compiles the same regex used by the integration assertion and proves it (a) matches a representative `predicted=...__0.0 live=...__1.1` line and (b) does not match the preserved `armPanes:202` "live pane count != saved count" shape.
```

**Resolution**: Fixed
**Notes**:

---

### 5. Phase 1 task 1-3 implicit dependency on tasks 1-1 and 1-2 not surfaced

**Severity**: Minor
**Plan Reference**: `planning.md` Phase 1 → Tasks table
**Category**: Dependencies and Ordering

**Change Type**: update-task

**Details**:
Task 1-3's value (asserting end-to-end scrollback replay with the new `--`-separated hook entry installed by `RegisterPortalHooks`) is conditional on tasks 1-1 (the constant change) and 1-2 (the migration) being complete. The plan relies on natural intra-phase ordering by internal ID, which is permitted by the integrity criteria — and indeed the IDs `1-1`, `1-2`, `1-3` produce the correct order. So no dependency edge is required. However, task 1-3's `Do` step 11 references "the migration code from Task 2 and the `--` separator from Task 1" using **task-name** numbering ("Task 1", "Task 2") that conflicts with the **internal-ID** numbering ("1-1", "1-2"). Because both phases have a "Task 1", a future reader could read step 11's "Task 1 / Task 2" as referring to phase-2 tasks. This is a documentation-clarity issue, not a structural dependency issue.

**Current**:
```markdown
  11. Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass a real hooks adapter that calls `RegisterPortalHooks` (so the migration code from Task 2 and the `--` separator from Task 1 actually run).
```

**Proposed**:
```markdown
  11. Wire the bootstrap orchestrator with **production** hook-registration adapters this time — i.e. pass a real hooks adapter that calls `RegisterPortalHooksWithLogger` (so the migration code from task 1-2 and the `--` separator from task 1-1 actually run).
```

(Note: this overlaps with finding 3's proposed text. If finding 3 is approved first, this clarification is already covered.)

**Resolution**: Fixed
**Notes**:

---
