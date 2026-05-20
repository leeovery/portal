---
status: complete
created: 2026-05-20
cycle: 2
phase: Gap Analysis
topic: esc-after-preview-hides-session-list
---

# Review Tracking: esc-after-preview-hides-session-list - Gap Analysis

## Findings

### 1. AC #1 cursor preservation is not exercised by the prescribed test assertion

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Test Coverage > "Lock in the fix at the wrong-axis miss site"; Acceptance Criteria #1

**Details**:
Acceptance Criterion #1 states three behaviours after the single `Esc`:
1. Committed filter text preserved.
2. Matching rows remain visible.
3. **"the previously-highlighted row remains the cursor."**

The prescribed augmentation to `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` is "Add a `VisibleItems()` assertion" — singular. That assertion covers points 1–2 (filter text via existing `FilterValue` check; visible rows via `visibleSessionNames`). It does not cover point 3 — there's no prescription to assert on the list's cursor / selected index after dismissal.

The cursor-preservation behaviour is real: the dismissed preview's session name is captured (`m.preview.session`) and `reanchorSessionCursor(msg.PreserveName)` runs in the `previewSessionsRefreshedMsg` handler (`internal/tui/model.go:1011-1023`). On the primary path the underlying session slice is unchanged across the round-trip and the previously-highlighted row should naturally remain selected — but "naturally" is not a tested invariant. If an implementer reorders the handler (e.g. swaps `applySessions` and `reanchorSessionCursor`) or the bubbles/list cursor-index behaviour shifts under the propagated `FilterMatchesMsg`, AC #1's third clause could regress silently.

Worth deciding explicitly: either (a) add a cursor-index assertion to the augmented test (`got.sessionList.Index()` or equivalent helper, asserting it points at the originally highlighted row), or (b) drop the cursor clause from AC #1 as covered transitively by the no-mutation invariant on the primary path.

The Scope section already excludes the harder cursor-reanchor-under-filter case on the externally-killed branch as out-of-scope. That excludes the kill-during-preview-while-filtered variant, but leaves AC #1's primary-path cursor claim in scope and untested.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Applied — see specification diff.

---

### 2. `ProjectsLoadedMsg` handler return-value characterization is missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Approach > Secondary sweep — `ProjectsLoadedMsg` handler

**Details**:
Cycle 1 finding 7 closed the asymmetry for the `SessionsMsg` handler by adding the explicit characterization: "currently returns `nil` on both branches it can reach after `applySessions` ... no `tea.Batch` is needed at this site." The same asymmetry now exists for the `ProjectsLoadedMsg` handler (`internal/tui/model.go:936-947`) — the spec says "capture the cmd from the `SetItems` call and batch/return it from the handler" without stating whether the handler currently returns `nil` (so the new cmd is the sole return) or non-nil (so `tea.Batch` is needed).

Minor — the implementer reads the handler anyway. But the parallel guidance for the sister site is now an obvious omission. A one-clause characterization ("handler currently returns `nil` — return the propagated cmd directly" or "handler returns `X` — use `tea.Batch(existing, propagated)`") would make the edit mechanical.

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Applied — see specification diff.

---
