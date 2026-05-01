---
status: complete
created: 2026-04-30
cycle: 2
phase: Plan Integrity Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Integrity

Cycle 1 verification: all five cycle-1 findings were applied correctly to the plan.

- Finding 1 (savedPanePos in Phase 2 plan-level grep AC): present at planning.md line 46. ✓
- Finding 2 (logger Option B locked in task 1-2 Do): phase-1-tasks.md lines 93-94 describe the sibling `RegisterPortalHooksWithLogger` with the `MigrationLogger` interface. ✓
- Finding 3 (production adapter naming clarified): phase-1-tasks.md line 196 names `internal/bootstrapadapter` and `RegisterPortalHooksWithLogger`. ✓
- Finding 4 (manual sanity check replaced with regex unit test): phase-2-tasks.md lines 81-84 describe the test, AC item 8 (line 94) names it. ✓
- Finding 5 (task-id naming "1-1"/"1-2" disambiguation): phase-1-tasks.md line 196 uses internal IDs. ✓

Two follow-up findings surfaced where cycle 1 edits did not propagate consistently across all sections of the same task. Both are Minor.

## Findings

### 1. Task 1-2 AC item references the old caller name after Option-B lock-in

**Severity**: Minor
**Plan Reference**: `phase-1-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-1-2` → Acceptance Criteria, item about `errors.Join` aggregation
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Cycle 1 finding 2 locked the logger-aware caller to the new sibling `RegisterPortalHooksWithLogger` (the `Do` section now states this explicitly). The migration is invoked once at the top of `RegisterPortalHooksWithLogger`, so error aggregation also happens there. However, the AC item describing the `ShowGlobalHooks` failure path still names `RegisterPortalHooks` as the caller that performs `errors.Join` aggregation. After the Option-B lock-in, `RegisterPortalHooks` is the no-op-logger wrapper that delegates to `RegisterPortalHooksWithLogger` — so naming the wrapper as the aggregator is inconsistent with the Do section. Updating the AC to name `RegisterPortalHooksWithLogger` keeps the contract internally consistent and removes a small ambiguity for the implementer about which function owns the aggregation.

**Current**:
```markdown
- [ ] A `ShowGlobalHooks` failure causes `migrateHydrationHooks` to return a wrapped error; the caller in `RegisterPortalHooks` aggregates it via `errors.Join` alongside any per-event register errors. Bootstrap does not abort on this path; the orchestrator surfaces the result as a soft warning per the existing bootstrap-step error contract.
```

**Proposed**:
```markdown
- [ ] A `ShowGlobalHooks` failure causes `migrateHydrationHooks` to return a wrapped error; the caller in `RegisterPortalHooksWithLogger` aggregates it via `errors.Join` alongside any per-event register errors (the no-op-logger wrapper `RegisterPortalHooks` inherits this behaviour by delegation). Bootstrap does not abort on this path; the orchestrator surfaces the result as a soft warning per the existing bootstrap-step error contract.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 2-2 Tests section does not list the regex unit test added by cycle 1's fix

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` task `scrollback-not-restored-with-non-zero-base-index-2-2` → Tests section
**Category**: Task Template Compliance / Acceptance Criteria Quality

**Change Type**: update-task

**Details**:
Cycle 1 finding 4 added a concrete unit test (`TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`) to the `Do` section (lines 81-84) and the Acceptance Criteria (item 8, line 94). The Tests section, however, was not updated in lockstep — it still contains a bullet that reads "verify by inspection that the existing warning shape... cannot match" rather than naming the new unit test. This leaves the Tests section out of sync with both the Do and AC sections of the same task: the `Do` mandates a test be written, the AC mandates that test exist, but the Tests section frames the same check as a manual inspection. The task-design template treats Tests as the authoritative test list; aligning it with AC item 8 keeps the canonical task fields consistent.

**Current**:
```markdown
**Tests**:
- `"portal.log contains zero predicted-vs-live WARN lines under non-zero base-index"` — the headline assertion this task adds.
- `"regex does not false-positive on the preserved armPanes:202 pane-count mismatch warning"` — verify by inspection that the existing warning shape (`live pane count %d != saved count %d`) cannot match `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`.
- `"existing base-index drift round-trip behaviour is unchanged"` — the rest of `runRebootRoundTrip`'s assertions (structural-key drift, hook firing, scrollback replay) continue to pass.
- `"no developer-primary-server interaction"` — verify by inspection that the test only references `tmuxtest.New(...)` / its returned `Client`; no bare `exec.Command("tmux", ...)` against the default socket.
```

**Proposed**:
```markdown
**Tests**:
- `"portal.log contains zero predicted-vs-live WARN lines under non-zero base-index"` — the headline integration assertion this task adds, gated by `//go:build integration`.
- `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning` — plain `go test` unit test (no integration tag) compiling the same `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+` regex used by the integration assertion and asserting (a) it matches a representative `WARN | restore | session "alpha": pane 0 predicted=alpha__0.0 live=alpha__1.1` line and (b) it does NOT match the preserved `armPanes:202` shape (`WARN | restore | session "alpha": live pane count 2 != saved count 3`).
- `"existing base-index drift round-trip behaviour is unchanged"` — the rest of `runRebootRoundTrip`'s assertions (structural-key drift, hook firing, scrollback replay) continue to pass.
- `"no developer-primary-server interaction"` — verify by inspection that the test only references `tmuxtest.New(...)` / its returned `Client`; no bare `exec.Command("tmux", ...)` against the default socket.
```

**Resolution**: Fixed
**Notes**:

---
