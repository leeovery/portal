---
status: complete
created: 2026-05-06
cycle: 1
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Read the plan end-to-end (planning.md plus all four phase task detail files). Walked every task against the eight integrity dimensions: template compliance, vertical slicing, phase structure, dependencies/ordering, self-containment, scope/granularity, AC quality, and external dependencies (n/a — feature, not epic).

**Overall assessment**: The plan is implementation-ready. Every task carries the full template (Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference); criteria are concrete, pass/fail, and pull spec context inline; tasks are TDD-cycle-shaped vertical slices; phase ordering follows a defensible foundation → core → cycling+chrome → edge cases progression. The constructor-injection pattern is consistently described; the three-shape `Tail` contract surfaces consistently across reader, model, placeholder, and error tasks.

Three integrity findings were identified, all of which are surface-level inconsistencies between phases (method names, file names, an unused Bubble Tea constant) that would cause minor confusion at implementation time. None are architectural or scope problems. They are recorded below in priority order.

## Findings

### 1. Method name inconsistency — `Enumerate` vs `ListWindowsAndPanesInSession`

**Severity**: Important
**Plan Reference**: Phase 4 Task 4-8 (Acceptance Criteria, Do step, multiple test names); Phase 3 Task 3-7 (one test name)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The seam interface `TmuxEnumerator` is declared in Task 2-1 with a single method `ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error)`. Tasks 4-8 (three places) and 3-7 (one test name) reference a different method name `Enumerate` / `TmuxEnumerator.Enumerate` / `Enumerate(session)`. An implementer following 4-8 literally would either invent a non-existent method or drop the criterion. The hermetic side-effect test (4-8) is the spec's central regression-pin for the side-effect-free contract — its assertions need to bind to the actual interface method to be enforceable.

This is purely a naming mismatch from earlier drafts; the criterion's intent ("exactly one structural-enumeration call across the full lifecycle") is correct and traceable to spec. The fix is mechanical: replace `Enumerate` with `ListWindowsAndPanesInSession` everywhere it appears in 4-8 and 3-7.

**Current** (from Task 4-8):
```markdown
**Do**:
- In `internal/tui/pagepreview_test.go`, build a recording `TmuxEnumerator` mock: tracks `Enumerate(session)` call count and arguments.

**Acceptance Criteria**:
- [ ] After a full open + cycle + dismiss flow, `TmuxEnumerator.Enumerate` was called exactly once.

**Tests**:
- `"hermetic preview cycle: exactly one TmuxEnumerator call across full lifecycle"` — drive full cycle + dismiss; assert call count.
```

```markdown
**Acceptance Criteria** (Task 4-6):
- [ ] `TmuxEnumerator.Enumerate` is called exactly once across the entire test (regression-pin against mid-preview re-enumeration).
```

```markdown
**Tests** (Task 3-7):
- `"full ] [ Tab cycle sequence produces exactly one Enumerate call"`
```

**Proposed**:

For Task 4-8 — update the **Do** bullet:
```markdown
- In `internal/tui/pagepreview_test.go`, build a recording `TmuxEnumerator` mock: tracks `ListWindowsAndPanesInSession(session)` call count and arguments.
```

For Task 4-8 — update the second **Acceptance Criterion**:
```markdown
- [ ] After a full open + cycle + dismiss flow, `TmuxEnumerator.ListWindowsAndPanesInSession` was called exactly once.
```

For Task 4-6 — update the first **Acceptance Criterion**:
```markdown
- [ ] `TmuxEnumerator.ListWindowsAndPanesInSession` is called exactly once across the entire test (regression-pin against mid-preview re-enumeration).
```

For Task 3-7 — update the first **Tests** entry:
```markdown
- `"full ] [ Tab cycle sequence produces exactly one ListWindowsAndPanesInSession call"`
```

**Resolution**: Fixed
**Notes**: Task 2-1's interface declaration and Phase 1 Task 1-5's `*tmux.Client` method both use `ListWindowsAndPanesInSession`. This finding aligns 4-8, 4-6, and 3-7 with that single name. No spec content changes; purely a naming consistency fix.

---

### 2. Filename inconsistency — `preview.go` vs `pagepreview.go`

**Severity**: Important
**Plan Reference**: Phase 2 Task 2-2 (Solution + Do); Phase 4 Tasks 4-1, 4-2, 4-7, 4-8 (Solution + Do); Phase 2 Task 2-7 (Test file: `pagepreview_test.go`)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Task 2-2 specifies the preview model lives in `internal/tui/preview.go`. Tasks 4-1, 4-2, 4-7, and 4-8 reference `internal/tui/pagepreview.go` as the file containing the preview model and its constants/string searches. The test file is consistently `internal/tui/pagepreview_test.go` across the plan (2-7, 3-1, 3-7, 4-1, 4-2, 4-3, 4-4, 4-5, 4-6, 4-7, 4-8) — so the test naming is coherent. Only the production source file name diverges between Phase 2 and Phase 4.

An implementer reading 4-1 in isolation would create or open `pagepreview.go` and miss the existing `preview.go` from Phase 2 — leading to two preview files, or to spec searches in 4-7 and 4-8 (which grep the source for `_portal-saver`, `SetSkeletonMarker`, etc.) checking the wrong file.

The convention in the rest of `internal/tui/` is single-word filenames like `model.go`, `loading.go`, `projects.go`. `pagepreview.go` matches the test file name (`pagepreview_test.go`) and matches other page-arm files conceptually. `preview.go` is shorter and aligns with the model type name (`previewModel`). Either is defensible — the plan should pick one and apply it consistently. Recommending `pagepreview.go` for production source because (a) it pairs with the existing `pagepreview_test.go` name pinned across the plan, (b) the plan's grep-based audits in 4-7 and 4-8 specifically read `pagepreview.go`.

**Current** (Task 2-2 Solution):
```markdown
**Solution**: Add a `previewModel` struct in a new file (e.g. `internal/tui/preview.go`) plus an exported constructor `NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, width, height int) (previewModel, bool)` returning `(model, ok)` where `ok=false` signals "do not transition to pagePreview". The constructor performs steps 1–4 of the spec's initial-open ordering inline; the caller (Sessions page) checks `ok` to decide whether to switch pages.
```

**Current** (Task 2-2 Do, first bullet):
```markdown
- In `internal/tui/preview.go` declare:
```

**Current** (Task 2-4 Do):
```markdown
- In `internal/tui/preview.go` (or wherever `previewModel` lives), in `previewModel.Update`:
```

**Proposed** (Task 2-2 Solution):
```markdown
**Solution**: Add a `previewModel` struct in a new file `internal/tui/pagepreview.go` plus an exported constructor `NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, width, height int) (previewModel, bool)` returning `(model, ok)` where `ok=false` signals "do not transition to pagePreview". The constructor performs steps 1–4 of the spec's initial-open ordering inline; the caller (Sessions page) checks `ok` to decide whether to switch pages. The filename `pagepreview.go` pairs with the pinned test file name `pagepreview_test.go` and is the file the Phase 4 audit tasks (4-7, 4-8) grep against.
```

**Proposed** (Task 2-2 Do, first bullet):
```markdown
- In `internal/tui/pagepreview.go` declare:
```

**Proposed** (Task 2-4 Do):
```markdown
- In `internal/tui/pagepreview.go` (or wherever `previewModel` lives), in `previewModel.Update`:
```

**Resolution**: Fixed
**Notes**: This change harmonises file naming across all four phases. Phase 4 tasks already use `pagepreview.go` correctly; the fix is to update Phase 2 (Tasks 2-2 and 2-4) to match. No content change; purely the filename string. Test file (`pagepreview_test.go`) is already consistent and needs no edit.

---

### 3. `tea.KeySpace` is not a Bubble Tea constant

**Severity**: Minor
**Plan Reference**: Phase 2 Task 2-3 (Tests)
**Category**: Task Self-Containment / Tests
**Change Type**: update-task

**Details**:
Task 2-3's first test entry says `synthesise a tea.KeyMsg{Type: tea.KeySpace}`. Bubble Tea has no `tea.KeySpace` constant — space is represented as a `tea.KeyRunes` event with `Runes: []rune{' '}` (or matched via `msg.String() == " "`). An implementer pasting this literal would hit a compile error and need to translate. The Do section avoids the issue by suggesting `key.NewBinding(key.WithKeys(" "))` (correct) but the Tests section reintroduces the wrong constant.

The fix is to align the test wording with the Do section's binding shape — match on the rune `' '` or on `msg.String() == " "`.

**Current** (Task 2-3 Tests, first entry):
```markdown
**Tests**:
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeySpace}`, drive `Update`, assert `m.page == pagePreview`.
```

**Proposed**:
```markdown
**Tests**:
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` (Bubble Tea has no `tea.KeySpace` constant — space is a runes key), drive `Update`, assert `m.page == pagePreview`.
```

**Resolution**: Fixed
**Notes**: Minor inaccuracy that would surface as a one-line compile error and easy fix at implementation time. Updating the plan keeps the test name list accurate without redesigning anything.
