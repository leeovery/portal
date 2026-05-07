---
status: complete
created: 2026-05-06
cycle: 3
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Re-read the plan end-to-end (planning.md plus all four phase task detail files) after cycle 2's planning.md table-row fix. Verified cycle 1 fixes (method name `Enumerate` → `ListWindowsAndPanesInSession` across 3-7/4-6/4-8; filename `preview.go` → `pagepreview.go` in 2-2/2-4; `tea.KeySpace` → `tea.KeyRunes` in 2-3) and cycle 2 fix (planning.md row for 3-7) all remain applied. Walked every task against the eight integrity dimensions (template compliance, vertical slicing, phase structure, dependencies/ordering, self-containment, scope/granularity, AC quality, external deps).

**Overall assessment**: The plan remains implementation-ready. Phase ordering, dependency edges, scope, AC quality, vertical slicing, and self-containment are all sound. No new architecture or scope drift introduced by cycles 1–2. Cycle 3 surfaces a single Minor consistency drift between the plan's pseudo-Go field references and the actual `internal/tui` Model field names — a code-accuracy issue confined to Phase 2 task bodies that an implementer would translate by inspection but should still be aligned for clarity.

## Findings

### 1. Phase 2 task bodies reference non-existent Model field names (`m.page`, `m.list`)

**Severity**: Minor
**Plan Reference**: Phase 2 Tasks 2-3 (Do step 6, Tests entries), 2-4 (Solution, Outcome, Do, AC, Tests, Edge Cases), 2-5 (Solution, Do code block, AC, Tests)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
The plan references the root TUI model fields as `m.page` (the page state) and `m.list` (the sessions list). The actual `internal/tui/model.go` exposes these as `m.activePage` and `m.sessionList` respectively (see lines 132 and 272 of `internal/tui/model.go`: `sessionList list.Model`, `activePage page`). This drift appears 20+ times in `phase-2-tasks.md` — across three sibling tasks — including inside a Go code block that an implementer would paste literally:

```
if m.list.SettingFilter() {
    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}
```

That snippet would not compile against the real `Model`. An implementer would notice on first build and translate, but the plan should match the actual codebase to spare the discovery loop and to keep the plan internally consistent with the task 2-2 Do block where `m.viewport` (a real future field on `previewModel`) and `m.preview` (the planned new field on `Model`) are both addressed correctly by their planned names.

This is purely a name-translation issue from planning that didn't consult the actual field names — same shape as cycle 1 finding #2 (filename) and finding #1 (method name). No spec content changes; no acceptance criterion changes meaning. The fix is mechanical: replace `m.page` with `m.activePage` and `m.list` with `m.sessionList` in Phase 2 task bodies (Phases 1, 3, 4 do not contain these references).

**Current** (Task 2-3, Do step 6):
```markdown
  6. If `ok==true` → store `m.preview = pmodel`, set `m.page = pagePreview`, return.
```

**Proposed** (Task 2-3, Do step 6):
```markdown
  6. If `ok==true` → store `m.preview = pmodel`, set `m.activePage = pagePreview`, return.
```

**Current** (Task 2-3, Do step 1 and 2):
```markdown
  1. If `m.list.SettingFilter()` is true → fall through to `bubbles/list`'s default handler (literal-space passthrough is task 2-5; this branch must NOT fire `NewPreviewModel`).
  2. If the list is empty (`len(m.list.Items()) == 0`) or `m.list.SelectedItem() == nil` → return without transition (no-op).
```

**Proposed** (Task 2-3, Do step 1 and 2):
```markdown
  1. If `m.sessionList.SettingFilter()` is true → fall through to `bubbles/list`'s default handler (literal-space passthrough is task 2-5; this branch must NOT fire `NewPreviewModel`).
  2. If the list is empty (`len(m.sessionList.Items()) == 0`) or `m.sessionList.SelectedItem() == nil` → return without transition (no-op).
```

**Current** (Task 2-3, Tests):
```markdown
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` (Bubble Tea has no `tea.KeySpace` constant — space is a runes key), drive `Update`, assert `m.page == pagePreview`.
- `"it remains on Sessions page when Space is pressed on an empty list"` — empty list, Space; assert page unchanged, `NewPreviewModel` not called (mock counter zero).
- `"it remains on Sessions page when enumeration fails"` — `TmuxEnumerator` mock returns error; Space; assert page unchanged.
- `"it remains on Sessions page when enumeration returns empty"` — mock returns empty groups; Space; assert page unchanged.
- `"it does not bind Space on the Loading page"` — drive `Update` with `m.page == PageLoading` and a Space KeyMsg; assert no preview construction.
- `"it does not bind Space on the Projects page"` — same but `m.page == PageProjects`.
- `"it does not bind Space on the FileBrowser page"` — same but `m.page == pageFileBrowser`.
```

**Proposed** (Task 2-3, Tests):
```markdown
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` (Bubble Tea has no `tea.KeySpace` constant — space is a runes key), drive `Update`, assert `m.activePage == pagePreview`.
- `"it remains on Sessions page when Space is pressed on an empty list"` — empty list, Space; assert page unchanged, `NewPreviewModel` not called (mock counter zero).
- `"it remains on Sessions page when enumeration fails"` — `TmuxEnumerator` mock returns error; Space; assert page unchanged.
- `"it remains on Sessions page when enumeration returns empty"` — mock returns empty groups; Space; assert page unchanged.
- `"it does not bind Space on the Loading page"` — drive `Update` with `m.activePage == PageLoading` and a Space KeyMsg; assert no preview construction.
- `"it does not bind Space on the Projects page"` — same but `m.activePage == PageProjects`.
- `"it does not bind Space on the FileBrowser page"` — same but `m.activePage == pageFileBrowser`.
```

**Current** (Task 2-4, Solution):
```markdown
**Solution**: In `previewModel.Update`, intercept `Esc` and return a marker (either via a returned `tea.Cmd` carrying a `previewDismissedMsg`, or by setting a flag the root model consults). The root model reacts by setting `m.page = PageSessions`. The `bubbles/list` model is left untouched across the open/dismiss round-trip — its cursor and filter state survive automatically because preview never mutates it.
```

**Proposed** (Task 2-4, Solution):
```markdown
**Solution**: In `previewModel.Update`, intercept `Esc` and return a marker (either via a returned `tea.Cmd` carrying a `previewDismissedMsg`, or by setting a flag the root model consults). The root model reacts by setting `m.activePage = PageSessions`. The `bubbles/list` model is left untouched across the open/dismiss round-trip — its cursor and filter state survive automatically because preview never mutates it.
```

**Current** (Task 2-4, Outcome):
```markdown
**Outcome**: `Esc` while on `pagePreview` returns the user to the Sessions list. The list cursor is byte-identical to where it was when `Space` was pressed (verified by reading `m.list.Index()` before and after). Committed filter state remains committed; no-filter remains no-filter. A second `Esc` (now on the Sessions page with a committed filter) clears the filter via `bubbles/list`'s default behaviour — preview doesn't need to do anything to make this work.
```

**Proposed** (Task 2-4, Outcome):
```markdown
**Outcome**: `Esc` while on `pagePreview` returns the user to the Sessions list. The list cursor is byte-identical to where it was when `Space` was pressed (verified by reading `m.sessionList.Index()` before and after). Committed filter state remains committed; no-filter remains no-filter. A second `Esc` (now on the Sessions page with a committed filter) clears the filter via `bubbles/list`'s default behaviour — preview doesn't need to do anything to make this work.
```

**Current** (Task 2-4, Do — `Set m.page = PageSessions`, `Do NOT mutate m.list`, `Confirm m.list.Index()...`):
```markdown
- In `internal/tui/pagepreview.go` (or wherever `previewModel` lives), in `previewModel.Update`:
  - Match `tea.KeyMsg` with `Type == tea.KeyEsc` (or `String() == "esc"`).
  - Return a sentinel `tea.Cmd` that emits `previewDismissedMsg{}` (declare the type in the same file).
- In `internal/tui/model.go` top-level `Update`, handle `previewDismissedMsg`:
  - Set `m.page = PageSessions`.
  - Do NOT mutate `m.list` (cursor and filter state must survive untouched).
  - Optional: zero out `m.preview` to release viewport memory; the next `Space` constructs fresh.
- Confirm that `bubbles/list` cursor (`m.list.Index()`) and filter state (`m.list.IsFiltered()`, `m.list.FilterValue()`) are not consulted/mutated during preview lifetime.
- Note: re-fetch on dismiss (for externally-killed sessions) is Phase 4 scope; this task only preserves existing cursor/filter, not session-list refresh.
```

**Proposed** (Task 2-4, Do):
```markdown
- In `internal/tui/pagepreview.go` (or wherever `previewModel` lives), in `previewModel.Update`:
  - Match `tea.KeyMsg` with `Type == tea.KeyEsc` (or `String() == "esc"`).
  - Return a sentinel `tea.Cmd` that emits `previewDismissedMsg{}` (declare the type in the same file).
- In `internal/tui/model.go` top-level `Update`, handle `previewDismissedMsg`:
  - Set `m.activePage = PageSessions`.
  - Do NOT mutate `m.sessionList` (cursor and filter state must survive untouched).
  - Optional: zero out `m.preview` to release viewport memory; the next `Space` constructs fresh.
- Confirm that `bubbles/list` cursor (`m.sessionList.Index()`) and filter state (`m.sessionList.IsFiltered()`, `m.sessionList.FilterValue()`) are not consulted/mutated during preview lifetime.
- Note: re-fetch on dismiss (for externally-killed sessions) is Phase 4 scope; this task only preserves existing cursor/filter, not session-list refresh.
```

**Current** (Task 2-4, Acceptance Criteria):
```markdown
- [ ] `Esc` while on `pagePreview` transitions back to `PageSessions`.
- [ ] After dismiss, `m.list.Index()` equals the value captured before `Space`.
- [ ] After dismiss, `m.list.FilterValue()` equals the value captured before `Space` (no filter case).
- [ ] After dismiss when a filter was committed before `Space`, the filter remains committed (`m.list.IsFiltered() == true` and same `FilterValue()`).
- [ ] A subsequent `Esc` on the Sessions page falls through to `bubbles/list`'s default Esc handling (committed filter clears, then unfiltered list), confirmed by integration test driving two consecutive Escs.
- [ ] Re-opening preview after dismiss constructs a fresh `previewModel` (no carried-over state).
```

**Proposed** (Task 2-4, Acceptance Criteria):
```markdown
- [ ] `Esc` while on `pagePreview` transitions back to `PageSessions`.
- [ ] After dismiss, `m.sessionList.Index()` equals the value captured before `Space`.
- [ ] After dismiss, `m.sessionList.FilterValue()` equals the value captured before `Space` (no filter case).
- [ ] After dismiss when a filter was committed before `Space`, the filter remains committed (`m.sessionList.IsFiltered() == true` and same `FilterValue()`).
- [ ] A subsequent `Esc` on the Sessions page falls through to `bubbles/list`'s default Esc handling (committed filter clears, then unfiltered list), confirmed by integration test driving two consecutive Escs.
- [ ] Re-opening preview after dismiss constructs a fresh `previewModel` (no carried-over state).
```

**Current** (Task 2-4, Tests):
```markdown
- `"it dismisses preview on Esc and returns to PageSessions"` — open preview, drive Esc; assert page transitions.
- `"it preserves the list cursor across open/dismiss"` — set list cursor to index 3, Space, Esc; assert `m.list.Index() == 3`.
- `"it preserves the no-filter state across open/dismiss"` — list with no filter, Space, Esc; assert `m.list.FilterValue() == ""`.
- `"it preserves committed filter across open/dismiss"` — commit a filter (`pigeon`), Space, Esc; assert filter still committed with same value.
- `"a second Esc clears a committed filter via list default behaviour"` — commit filter, Space, Esc (back to list with filter), Esc; assert filter cleared.
- `"it constructs a fresh previewModel on re-open after dismiss"` — open, dismiss, re-open; assert `Tail` was called twice total (once per open).
```

**Proposed** (Task 2-4, Tests):
```markdown
- `"it dismisses preview on Esc and returns to PageSessions"` — open preview, drive Esc; assert page transitions.
- `"it preserves the list cursor across open/dismiss"` — set list cursor to index 3, Space, Esc; assert `m.sessionList.Index() == 3`.
- `"it preserves the no-filter state across open/dismiss"` — list with no filter, Space, Esc; assert `m.sessionList.FilterValue() == ""`.
- `"it preserves committed filter across open/dismiss"` — commit a filter (`pigeon`), Space, Esc; assert filter still committed with same value.
- `"a second Esc clears a committed filter via list default behaviour"` — commit filter, Space, Esc (back to list with filter), Esc; assert filter cleared.
- `"it constructs a fresh previewModel on re-open after dismiss"` — open, dismiss, re-open; assert `Tail` was called twice total (once per open).
```

**Current** (Task 2-4, Edge Cases — second bullet):
```markdown
- The cursor preservation invariant relies on preview NEVER calling `m.list.Select` or any list mutator. Confirm by code review.
```

**Proposed** (Task 2-4, Edge Cases):
```markdown
- The cursor preservation invariant relies on preview NEVER calling `m.sessionList.Select` or any list mutator. Confirm by code review.
```

**Current** (Task 2-5, Solution):
```markdown
**Solution**: In `updateSessionsPage`, the `Space` branch added in task 2-3 must explicitly check `m.list.SettingFilter()` and fall through to the `bubbles/list` default handler (passing the message via `m.list.Update(msg)`) when true. Only after the filter is committed (`SettingFilter()` returns false) does `Space` invoke `NewPreviewModel`.
```

**Proposed** (Task 2-5, Solution):
```markdown
**Solution**: In `updateSessionsPage`, the `Space` branch added in task 2-3 must explicitly check `m.sessionList.SettingFilter()` and fall through to the `bubbles/list` default handler (passing the message via `m.sessionList.Update(msg)`) when true. Only after the filter is committed (`SettingFilter()` returns false) does `Space` invoke `NewPreviewModel`.
```

**Current** (Task 2-5, Do — code block and confirmation bullet):
```markdown
- In `updateSessionsPage` in `internal/tui/model.go`, ensure the `Space` branch from task 2-3 begins with:
  ```go
  if m.list.SettingFilter() {
      // Filter input mode: Space is text input — pass through to bubbles/list.
      var cmd tea.Cmd
      m.list, cmd = m.list.Update(msg)
      return m, cmd
  }
  ```
- Confirm there is exactly one `Space` keybinding in `updateSessionsPage` — no second binding for "open preview while filtering".
- Confirm that after `Enter` commits the filter, `m.list.SettingFilter()` returns false on subsequent `Space` events, so preview opens normally on the highlighted match.
```

**Proposed** (Task 2-5, Do):
```markdown
- In `updateSessionsPage` in `internal/tui/model.go`, ensure the `Space` branch from task 2-3 begins with:
  ```go
  if m.sessionList.SettingFilter() {
      // Filter input mode: Space is text input — pass through to bubbles/list.
      var cmd tea.Cmd
      m.sessionList, cmd = m.sessionList.Update(msg)
      return m, cmd
  }
  ```
- Confirm there is exactly one `Space` keybinding in `updateSessionsPage` — no second binding for "open preview while filtering".
- Confirm that after `Enter` commits the filter, `m.sessionList.SettingFilter()` returns false on subsequent `Space` events, so preview opens normally on the highlighted match.
```

**Current** (Task 2-5, Acceptance Criteria):
```markdown
- [ ] When `m.list.SettingFilter()` is true, `Space` passes through to `m.list.Update(msg)` and is consumed by `bubbles/list` as text input.
- [ ] When `m.list.SettingFilter()` is true, `NewPreviewModel` is NOT called (verified via mock counter).
- [ ] After `Enter` commits the filter, `Space` opens preview on the highlighted match.
- [ ] No second key binding exists for "open preview while filtering" (verified by code review — exactly one `Space` branch in `updateSessionsPage`).
- [ ] A literal space character is observably present in `m.list.FilterValue()` after typing `Space` during `SettingFilter()`.
```

**Proposed** (Task 2-5, Acceptance Criteria):
```markdown
- [ ] When `m.sessionList.SettingFilter()` is true, `Space` passes through to `m.sessionList.Update(msg)` and is consumed by `bubbles/list` as text input.
- [ ] When `m.sessionList.SettingFilter()` is true, `NewPreviewModel` is NOT called (verified via mock counter).
- [ ] After `Enter` commits the filter, `Space` opens preview on the highlighted match.
- [ ] No second key binding exists for "open preview while filtering" (verified by code review — exactly one `Space` branch in `updateSessionsPage`).
- [ ] A literal space character is observably present in `m.sessionList.FilterValue()` after typing `Space` during `SettingFilter()`.
```

**Current** (Task 2-5, Tests):
```markdown
- `"it inserts a literal space into the filter while SettingFilter"` — start filter input mode (drive a `/` key or whatever bubble/list uses to enter filter mode), type `pigeon`, then Space; assert `m.list.FilterValue()` contains `"pigeon "` and `NewPreviewModel` was not called.
- `"it does not open preview while SettingFilter"` — same setup, drive Space; assert `m.page != pagePreview`.
- `"Space after Enter-commit opens preview on the highlighted match"` — drive filter input, type `pigeon`, Enter, then Space; assert `m.page == pagePreview` and the previewed session matches the highlighted item.
- `"it does not register a second open-preview binding for filter mode"` — code-level test: assert no key in the keymap has `Space` while `SettingFilter` is true that fires preview. (Can be enforced via the test that the Space-during-SettingFilter path consumes the message via `m.list.Update` only.)
```

**Proposed** (Task 2-5, Tests):
```markdown
- `"it inserts a literal space into the filter while SettingFilter"` — start filter input mode (drive a `/` key or whatever bubble/list uses to enter filter mode), type `pigeon`, then Space; assert `m.sessionList.FilterValue()` contains `"pigeon "` and `NewPreviewModel` was not called.
- `"it does not open preview while SettingFilter"` — same setup, drive Space; assert `m.activePage != pagePreview`.
- `"Space after Enter-commit opens preview on the highlighted match"` — drive filter input, type `pigeon`, Enter, then Space; assert `m.activePage == pagePreview` and the previewed session matches the highlighted item.
- `"it does not register a second open-preview binding for filter mode"` — code-level test: assert no key in the keymap has `Space` while `SettingFilter` is true that fires preview. (Can be enforced via the test that the Space-during-SettingFilter path consumes the message via `m.sessionList.Update` only.)
```

**Resolution**: Fixed
**Notes**: Mechanical alignment with the actual `internal/tui/model.go` field names. No spec content changes; no architectural impact. Phases 1, 3, and 4 do not contain `m.page` / `m.list` references, so the fix is confined to `phase-2-tasks.md`. After this fix, the plan's pseudo-Go references compile cleanly against the real `Model`.
