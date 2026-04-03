---
status: complete
created: 2026-04-03
cycle: 2
phase: Plan Integrity Review
topic: Resume Hooks Lost On Server Restart
---

# Review Tracking: Resume Hooks Lost On Server Restart - Integrity

## Cycle 1 Follow-Up

All 5 cycle 1 findings were applied. Phase 3 tasks now have Do and Tests fields. Task 2-4 Do step clarified. Task 2-3 has Context for cross-phase compile error. Task 3-1 has Context for store-side rename scope. No regressions from applied fixes.

## Findings

### 1. Tasks 2-2 and 2-3 Missing Required Tests Field

**Severity**: Important
**Plan Reference**: Phase 2 / Task 2-2 (tick-a415a7) and Task 2-3 (tick-77abcf)
**Category**: Task Template Compliance
**Change Type**: update-task

**Details**:
The task template requires a Tests field with "At least one test name; include edge cases, not just happy path." Tasks 2-2 and 2-3 omit the Tests section entirely. Task 2-2 embeds test case names within Do step 4, and Task 2-3 describes test file updates in the Do section, but neither has a standalone Tests field. This is the same class of issue cycle 1 fixed for Phase 3 tasks -- these two Phase 2 tasks were overlooked. Tasks 2-1 and 2-4 both have Tests fields, making Phase 2 internally inconsistent.

**Current**:

Task 2-2 (tick-a415a7) has no Tests section. Description goes: Problem -> Solution -> Outcome -> Do -> Acceptance Criteria -> Spec Reference.

Task 2-3 (tick-77abcf) has no Tests section. Description goes: Problem -> Solution -> Outcome -> Do -> Acceptance Criteria -> Context -> Spec Reference.

**Proposed**:

**Task 2-2** (tick-a415a7) -- insert after Acceptance Criteria:
```
**Tests**:
- "returns structural key for valid pane ID"
- "returns error for invalid pane ID"
- "returns error when tmux command fails"
```

**Task 2-3** (tick-77abcf) -- insert after Acceptance Criteria (before Context):
```
**Tests**:
- "returns hooks sorted by key then event" -- RENAMED from "sorted by pane ID"
- "Set stores hook under structural key" -- UPDATED values
- "Remove deletes hook by structural key" -- UPDATED values
- "Get retrieves hook by structural key" -- UPDATED values
- "CleanStale removes entries not in liveKeys" -- UPDATED values
- "Load parses structural key format from JSON" -- UPDATED values
- Plus all existing store tests with structural key values replacing pane IDs
```

**Resolution**: Fixed
**Notes**:
