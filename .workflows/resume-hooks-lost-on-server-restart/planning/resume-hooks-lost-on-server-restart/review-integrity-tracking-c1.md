---
status: in-progress
created: 2026-04-03
cycle: 1
phase: Plan Integrity Review
topic: Resume Hooks Lost On Server Restart
---

# Review Tracking: Resume Hooks Lost On Server Restart - Integrity

## Findings

### 1. Phase 3 Tasks 3-2, 3-3, 3-4, 3-5 Missing Required Do Field

**Severity**: Important
**Plan Reference**: Phase 3 / Tasks 3-2, 3-3, 3-4, 3-5
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
The task template requires a Do field with "At least one concrete action." Tasks 3-2, 3-3, 3-4, and 3-5 jump from Outcome/Solution directly to Acceptance Criteria with no Do section. An implementer would need to infer the implementation steps from the Problem/Solution description. Phase 1 and Phase 2 tasks all have detailed Do sections — Phase 3 is inconsistent.

Task 3-1 already has a Do section and is not included in this finding.

**Current**:

Task 3-2 (tick-310fb7) has no Do section. Description goes: Problem -> Solution -> Outcome -> Acceptance Criteria -> Spec Reference.

Task 3-3 (tick-bd2554) has no Do section. Description goes: Problem -> Solution -> Outcome -> Acceptance Criteria -> Spec Reference.

Task 3-4 (tick-557972) has no Do section. Description goes: Problem -> Solution -> Outcome -> Acceptance Criteria -> Spec Reference.

Task 3-5 (tick-4056c4) has no Do section. Description goes: Problem -> Solution -> Outcome -> Acceptance Criteria -> Spec Reference.

**Proposed**:

Add Do section to each task:

**Task 3-2** (tick-310fb7) — insert after Outcome:
```
**Do**:
1. In cmd/hooks.go, add StructuralKeyResolver interface with ResolveStructuralKey(paneID string) (string, error) method
2. Add KeyResolver field of type StructuralKeyResolver to HooksDeps struct
3. In hooksSetCmd RunE, after requireTmuxPane call, add keyResolver.ResolveStructuralKey(paneID) call — use returned structural key for store.Set and MarkerName instead of raw paneID
4. Return user-facing error if ResolveStructuralKey fails
5. In cmd/hooks_test.go, add mockKeyResolver implementing StructuralKeyResolver
6. Update all TestHooksSetCommand test cases: mock KeyResolver to return structural key, update store.Set and marker assertions to expect structural key values
7. Add test case "ResolveStructuralKey failure returns error" with mockKeyResolver returning error
8. Run go test ./cmd/ -run TestHooksSet
```

**Task 3-3** (tick-bd2554) — insert after Outcome:
```
**Do**:
1. In cmd/hooks.go hooksRmCmd RunE, after requireTmuxPane call, add hooksDeps.KeyResolver.ResolveStructuralKey(paneID) call — use returned structural key for store.Remove and marker deletion instead of raw paneID
2. Return user-facing error if ResolveStructuralKey fails
3. In cmd/hooks_test.go, update all TestHooksRmCommand test cases: set mockKeyResolver to return structural key, update store.Remove and marker assertions to expect structural key values
4. Add test case "ResolveStructuralKey failure returns error" with mockKeyResolver returning error
5. Run go test ./cmd/ -run TestHooksRm
```

**Task 3-4** (tick-557972) — insert after Outcome:
```
**Do**:
1. In cmd/clean_test.go, replace all pane ID mock values (%3, %7, etc.) with structural key values (e.g., "my-session:0.0", "my-session:0.1")
2. Update ListAllPanes mock return values to structural keys
3. Update CleanStale mock assertions to expect structural keys
4. Update marker-related assertions from @portal-active-%N to @portal-active-session:window.pane format
5. Run go test ./cmd/ -run TestClean
```

**Task 3-5** (tick-4056c4) — insert after Outcome:
```
**Do**:
1. In internal/hooks/executor_test.go, add test "multi-pane independent hooks fire correctly": set up session with 3 panes (my-session:0.0, my-session:0.1, my-session:1.0), each with independent on-resume hooks, verify all 3 send-keys calls fire with correct structural key targets
2. Add test "orphaned structural keys produce graceful no-op": set up hooks keyed to structural positions not in ListPanes result, verify no send-keys calls and no errors
3. Add test "hooks survive empty pane list after restart": set up hooks, ListAllPanes returns empty, verify CleanStale NOT called and hooks remain in store (re-load after ExecuteHooks returns same data)
4. Run go test ./internal/hooks/... then go test ./...
```

**Resolution**: Pending
**Notes**:

---

### 2. All Phase 3 Tasks Missing Required Tests Field

**Severity**: Important
**Plan Reference**: Phase 3 / Tasks 3-1, 3-2, 3-3, 3-4, 3-5
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
The task template requires a Tests field with "At least one test name; include edge cases, not just happy path." All five Phase 3 tasks omit the Tests section entirely. Phase 1 and Phase 2 tasks all include explicit test names. Without test names, the implementer must invent test naming from acceptance criteria, which introduces ambiguity.

**Current**:

Tasks 3-1 through 3-5 have no Tests section. Each task goes from Acceptance Criteria directly to Spec Reference.

**Proposed**:

Add Tests section to each task:

**Task 3-1** (tick-e2081d) — insert after Acceptance Criteria:
```
**Tests**:
- "displays structural keys instead of pane IDs" -- UPDATED existing test
- "lists multiple hooks sorted by key then event" -- UPDATED to structural key values
- "empty hook store returns no output" -- EXISTING unchanged
```

**Task 3-2** (tick-310fb7) — insert after Acceptance Criteria:
```
**Tests**:
- "stores hook under structural key resolved from TMUX_PANE" -- UPDATED
- "sets volatile marker using structural key format" -- UPDATED
- "ResolveStructuralKey failure returns user-facing error" -- NEW
- Plus existing set tests updated to structural key values
```

**Task 3-3** (tick-bd2554) — insert after Acceptance Criteria:
```
**Tests**:
- "removes hook using structural key resolved from TMUX_PANE" -- UPDATED
- "deletes volatile marker using structural key format" -- UPDATED
- "ResolveStructuralKey failure returns user-facing error" -- NEW
- Plus existing rm tests updated to structural key values
```

**Task 3-4** (tick-557972) — insert after Acceptance Criteria:
```
**Tests**:
- "cleans stale hooks using structural key matching" -- UPDATED mock values
- "skips cleanup when no panes exist" -- EXISTING, updated mock values
- "removes orphaned marker options" -- UPDATED marker format assertions
```

**Task 3-5** (tick-4056c4) — insert after Acceptance Criteria:
```
**Tests**:
- "multi-pane session fires independent hooks for each structural position"
- "orphaned structural keys produce no errors and no send-keys calls"
- "empty pane list after restart preserves hooks and skips CleanStale"
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 2-4 Do Step Misleadingly Duplicates Task 1-1 Assertion Change

**Severity**: Important
**Plan Reference**: Phase 2 / Task 2-4 (tick-6f568e)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 2-4 Do step says: "Update 'no tmux server running' test to assert CleanStale NOT called (Phase 1 guard) — Rename to 'empty pane list skips cleanup and continues hook execution'". However, Task 1-1 already changes this test's assertion from "CleanStale IS called" to "CleanStale NOT called". By the time Task 2-4 executes, the assertion is already correct. The Do step reads as if 2-4 is making the assertion change, which would confuse an implementer who sees the assertion is already correct. The step should clarify that 2-4 is migrating test values to structural keys and renaming the test, not changing the assertion.

**Current**:
```
2. In internal/hooks/executor_test.go:
   ...
   - Update "no tmux server running" test to assert CleanStale NOT called (Phase 1 guard)
   - Rename to "empty pane list skips cleanup and continues hook execution"
   ...
```

**Proposed**:
```
2. In internal/hooks/executor_test.go:
   ...
   - Rename "no tmux server running skips cleanup gracefully" to "empty pane list skips cleanup and continues hook execution"
   - Update its mock values from pane IDs to structural keys (assertion already corrected by Phase 1 Task 1-1)
   ...
```

**Resolution**: Pending
**Notes**:

---

### 4. Phase 2 Ends With Non-Compiling Codebase

**Severity**: Important
**Plan Reference**: Phase 2 / Task 2-3 (tick-77abcf) and Phase 3 / Task 3-1 (tick-e2081d)
**Category**: Phase Structure
**Change Type**: add-to-task

**Details**:
Task 2-3 renames the exported field `Hook.PaneID` to `Hook.Key` in `internal/hooks/store.go`. The `cmd/hooks.go` file references `h.PaneID` on line 69. After Phase 2 completes, `go test ./...` and `go build ./...` will fail with a compile error in the `cmd` package. The fix is Task 3-1 (first task of Phase 3), which changes `h.PaneID` to `h.Key`.

Phase 2 acceptance criteria and Task 2-3 acceptance criteria only require `go test ./internal/hooks/...` to pass, which avoids the broken cmd package. This is technically workable but means Phase 2 cannot be verified with a full build. An implementer should know this is expected, not a mistake.

Adding a Context note to Task 2-3 to document this known state, and ensuring the Phase 2 acceptance criteria explicitly acknowledge the limited test scope.

**Current**:
Task 2-3 (tick-77abcf) has no Context section. Last acceptance criterion reads:
```
- [ ] go test ./internal/hooks/... passes (store tests only; executor tests are Task 4)
```

**Proposed**:
Add Context section to Task 2-3 after Edge Cases (or after last field):
```
**Context**:
> Renaming the exported Hook.PaneID field to Hook.Key will cause a compile error in cmd/hooks.go (line 69 references h.PaneID). This is intentional — the fix is Phase 3 Task 3-1 which is the first task in the next phase. Full-suite `go test ./...` will not pass until Task 3-1 completes. Scope this task's verification to `go test ./internal/hooks/...` only.
```

**Resolution**: Pending
**Notes**:

---

### 5. Task 3-1 Missing Edge Case for Sort Field Rename

**Severity**: Minor
**Plan Reference**: Phase 3 / Task 3-1 (tick-e2081d)
**Category**: Task Self-Containment
**Change Type**: add-to-task

**Details**:
Task 3-1 fixes the compile error by changing `h.PaneID` to `h.Key` in cmd/hooks.go line 69. However, the `List()` method in store.go also uses `Hook.PaneID` in its sort comparison (lines 118-119): `list[i].PaneID < list[j].PaneID`. Task 2-3 already renames these in store.go, so cmd/hooks.go line 69 is the only remaining reference. This is correct as written, but Task 3-1's Do section should include a note that the store-side rename was already handled by Task 2-3, so the implementer knows to only change cmd/hooks.go and its tests. The current Do section is sufficient but a brief Context note would strengthen self-containment.

**Current**:
Task 3-1 (tick-e2081d) has no Context section.

**Proposed**:
Add Context section to Task 3-1 after Acceptance Criteria:
```
**Context**:
> Phase 2 Task 2-3 already renamed Hook.PaneID to Hook.Key in internal/hooks/store.go (struct field, List() sort, all store tests). The only remaining reference is cmd/hooks.go line 69 which uses h.PaneID in the list output formatting. This task fixes that single reference and updates the cmd-layer tests.
```

**Resolution**: Pending
**Notes**:
