---
status: in-progress
created: 2026-05-06
cycle: 4
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Re-read planning.md plus all four phase task detail files end-to-end after cycle 3's field-name alignment fix (`m.page` → `m.activePage`, `m.list` → `m.sessionList`). Confirmed cycle 3 fix landed cleanly: `phase-2-tasks.md` now contains zero `m.page` / `m.list` references; the Go code block in 2-5's Do section, AC criteria, Tests entries, Outcome, and Solution sections all reference real fields. Cross-checked the actual `internal/tui/model.go` to verify the field names: `sessionList list.Model` (line 132), `activePage page` (line 145), and the bubbles/list API surface (`SettingFilter()`, `FilterValue()`, `IsFiltered()`, `Index()`, `SelectedItem()`, etc.) — all valid. Verified cycles 1-3's prior fixes (method rename, filename, key constant, planning.md table-row, field-name alignment) all remain applied with no regression.

**Overall assessment**: The plan remains implementation-ready on every other dimension — phase ordering, vertical slicing, dependency edges, AC quality, scope/granularity, and self-containment are sound. Cycle 4 surfaces one Minor consistency drift of the same shape as cycle 3's: a non-existent function name (`updateSessionsPage`) is referenced in the plan as if it were the existing Sessions-page handler. The actual handler in `internal/tui/model.go` (line 1115) is `updateSessionList`. An implementer would discover this on first grep and translate, but the plan should match the codebase. Same category (name-translation drift), same severity, isolated fix.

## Findings

### 1. Phase 2 / Phase 4 task bodies reference non-existent handler name `updateSessionsPage`

**Severity**: Minor
**Plan Reference**: Phase 2 Tasks 2-3 (Solution, Do header bullet, AC), 2-5 (Solution, Do header bullet, Do confirmation bullet, AC); Phase 4 Task 4-5 (Solution)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The plan refers to the Sessions-page key handler as `updateSessionsPage` seven times across `phase-2-tasks.md` (six refs) and `phase-4-tasks.md` (one ref). The actual function in `internal/tui/model.go` is `updateSessionList` (line 1115). The function does exactly what the plan describes — switches on `tea.KeyMsg`, intercepts `m.sessionList.SettingFilter()` for passthrough at line 1131, and delegates remaining keys to `m.sessionList.Update(msg)` at line 1163 — so the prose around the name is correct. Only the symbol is wrong.

This is the same shape as cycle 3's `m.page`/`m.list` drift: a name-translation issue from planning that didn't consult the actual codebase symbol. An implementer searching for `updateSessionsPage` would find nothing, look around, and discover `updateSessionList` after a moment — avoidable noise. Replacing the name preserves all prose and pseudo-Go around it.

Phases 1 and 3 do not reference this name. The fix is mechanical: replace `updateSessionsPage` with `updateSessionList` in the seven referenced sites.

**Current** (Task 2-3, Solution):
```markdown
**Solution**: Add `pagePreview` to the `page` constant block in `internal/tui/model.go`, hold a `*previewModel` (or value-typed `previewModel` with a sentinel) on the root `Model`, route the page in the top-level `Update` switch, and bind `Space` in `updateSessionsPage` to construct a `previewModel` and transition to `pagePreview` when the constructor returns `ok=true`. When `ok=false`, the user remains on the Sessions page silently.
```

**Proposed** (Task 2-3, Solution):
```markdown
**Solution**: Add `pagePreview` to the `page` constant block in `internal/tui/model.go`, hold a `*previewModel` (or value-typed `previewModel` with a sentinel) on the root `Model`, route the page in the top-level `Update` switch, and bind `Space` in `updateSessionList` to construct a `previewModel` and transition to `pagePreview` when the constructor returns `ok=true`. When `ok=false`, the user remains on the Sessions page silently.
```

**Current** (Task 2-3, Do — first numbered-step header bullet):
```markdown
- In `updateSessionsPage` (Sessions page handler), add a `tea.KeyMsg` branch matching `Space` (key string `" "` or use `bubbles/key.NewBinding(key.WithKeys(" "))`):
```

**Proposed** (Task 2-3, Do):
```markdown
- In `updateSessionList` (Sessions page handler in `internal/tui/model.go`), add a `tea.KeyMsg` branch matching `Space` (key string `" "` or use `bubbles/key.NewBinding(key.WithKeys(" "))`):
```

**Current** (Task 2-5, Solution):
```markdown
**Solution**: In `updateSessionsPage`, the `Space` branch added in task 2-3 must explicitly check `m.sessionList.SettingFilter()` and fall through to the `bubbles/list` default handler (passing the message via `m.sessionList.Update(msg)`) when true. Only after the filter is committed (`SettingFilter()` returns false) does `Space` invoke `NewPreviewModel`.
```

**Proposed** (Task 2-5, Solution):
```markdown
**Solution**: In `updateSessionList`, the `Space` branch added in task 2-3 must explicitly check `m.sessionList.SettingFilter()` and fall through to the `bubbles/list` default handler (passing the message via `m.sessionList.Update(msg)`) when true. Only after the filter is committed (`SettingFilter()` returns false) does `Space` invoke `NewPreviewModel`.
```

**Current** (Task 2-5, Do — first bullet introducing the code block):
```markdown
- In `updateSessionsPage` in `internal/tui/model.go`, ensure the `Space` branch from task 2-3 begins with:
```

**Proposed** (Task 2-5, Do):
```markdown
- In `updateSessionList` in `internal/tui/model.go`, ensure the `Space` branch from task 2-3 begins with:
```

**Current** (Task 2-5, Do — third bullet asserting single-binding):
```markdown
- Confirm there is exactly one `Space` keybinding in `updateSessionsPage` — no second binding for "open preview while filtering".
```

**Proposed** (Task 2-5, Do):
```markdown
- Confirm there is exactly one `Space` keybinding in `updateSessionList` — no second binding for "open preview while filtering".
```

**Current** (Task 2-5, Acceptance Criteria — fourth bullet):
```markdown
- [ ] No second key binding exists for "open preview while filtering" (verified by code review — exactly one `Space` branch in `updateSessionsPage`).
```

**Proposed** (Task 2-5, Acceptance Criteria):
```markdown
- [ ] No second key binding exists for "open preview while filtering" (verified by code review — exactly one `Space` branch in `updateSessionList`).
```

**Current** (Task 4-5, Solution):
```markdown
**Solution**: Audit `internal/tui/model.go::updateSessionsPage` and the page transition handlers for any existing on-entry refresh path (e.g. a `loadSessionsCmd` already dispatched on `pageProjects → pageSessions` or `pageFileBrowser → pageSessions`). Two outcomes:
1. **Existing refresh found and applies to `pagePreview → pageSessions`**: add a regression-pinning test that exercises the dismiss path with a mock session lister whose return changes between the open and the dismiss, confirming the post-dismiss list reflects the new state.
2. **Gap found**: add a refresh dispatch in the dismiss handler (the `Esc`-from-preview branch in `Update`) that returns a `tea.Cmd` re-fetching the sessions list before re-rendering `pageSessions`. Re-test against the same scenario.

The dismiss handler must continue to preserve the `bubbles/list` cursor and filter state per Phase 2 task 2-4 — the refresh updates the list items, but cursor positioning falls back to `bubbles/list`'s default behaviour when the previously-selected session is gone (lands on a neighbouring entry).
```

**Proposed** (Task 4-5, Solution):
```markdown
**Solution**: Audit `internal/tui/model.go::updateSessionList` and the page transition handlers for any existing on-entry refresh path (e.g. a `loadSessionsCmd` already dispatched on `pageProjects → pageSessions` or `pageFileBrowser → pageSessions`). Two outcomes:
1. **Existing refresh found and applies to `pagePreview → pageSessions`**: add a regression-pinning test that exercises the dismiss path with a mock session lister whose return changes between the open and the dismiss, confirming the post-dismiss list reflects the new state.
2. **Gap found**: add a refresh dispatch in the dismiss handler (the `Esc`-from-preview branch in `Update`) that returns a `tea.Cmd` re-fetching the sessions list before re-rendering `pageSessions`. Re-test against the same scenario.

The dismiss handler must continue to preserve the `bubbles/list` cursor and filter state per Phase 2 task 2-4 — the refresh updates the list items, but cursor positioning falls back to `bubbles/list`'s default behaviour when the previously-selected session is gone (lands on a neighbouring entry).
```

**Resolution**: Pending
**Notes**: Mechanical alignment with the actual `internal/tui/model.go` symbol name. No spec content changes; no acceptance criterion changes meaning; no architectural impact. Phases 1 and 3 do not contain `updateSessionsPage` references, so the fix is confined to `phase-2-tasks.md` (six sites) and `phase-4-tasks.md` (one site). Same category and severity as cycle 3 finding.
