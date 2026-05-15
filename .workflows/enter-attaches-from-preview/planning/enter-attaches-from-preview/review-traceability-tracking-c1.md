---
status: in-progress
created: 2026-05-15
cycle: 1
phase: Traceability Review
topic: Enter Attaches From Preview
---

# Review Tracking: Enter Attaches From Preview - Traceability

## Findings

### 1. Missing acceptance criterion + test for unconditional Enter attach across viewport content states

**Type**: Incomplete coverage
**Spec Reference**: § Other edge cases > Mid-load / placeholder preview content — "Enter attaches unconditionally regardless of viewport content state. No confirmation prompt, no guard."
**Plan Reference**: phase-1-tasks.md task 1-6 (`enter-attaches-from-preview-1-6`)
**Change Type**: add-to-task

**Details**:
The spec explicitly mandates Enter dispatch must be unconditional across the three observable viewport content shapes (real bytes, `(nil, nil)` placeholder, OS-level read error). Task 1-8 covers chrome wording invariance across the three states, but task 1-6 (the Enter intercept itself) has no acceptance criterion or test asserting the attach pipeline dispatches regardless of viewport state. The implementation naturally satisfies it (the new case body does not inspect viewport content), but the contract should be pinned by an explicit assertion so a future viewport-state-conditional guard would surface as a test failure.

The current task 1-6 acceptance and tests cover index correctness, intercept-vs-forward, and nil-attacher defensiveness, but not the viewport-content-state invariance.

**Current**:

```markdown
**Acceptance Criteria**:
- [ ] `previewModel.Update` returns `(m, attacher.Run(session, w, p))` for `tea.KeyEnter` with raw indices from `currentRawIndices()`.
- [ ] The Enter case returns BEFORE the `viewport.Update` delegation at the bottom of the function — i.e. viewport is not updated for the Enter event.
- [ ] When the user has not navigated, raw indices match the captured-at-open `(WindowIndex, PaneIndices[0])` of the first group.
- [ ] When the user has navigated via `]`/`[`/`Tab`, raw indices reflect the walked focus.
- [ ] On a session with non-contiguous `window_index` (e.g. 0, 2, 5) or non-zero `pane-base-index`, the dispatched indices are the raw tmux values, not slice positions.
- [ ] When `m.attacher` is nil, Enter is a silent no-op (returns `(m, nil)`).

**Tests**:
- `"Enter dispatches with captured-at-open raw indices when user has not navigated"`
- `"Enter dispatches with walked indices after Tab"`
- `"Enter dispatches with walked indices after ]"`
- `"Enter dispatches with raw tmux indices on non-contiguous-index session"`
- `"Enter is intercepted and not forwarded to viewport"`
- `"Enter is a no-op when attacher is nil"`
```

**Proposed**:

```markdown
**Acceptance Criteria**:
- [ ] `previewModel.Update` returns `(m, attacher.Run(session, w, p))` for `tea.KeyEnter` with raw indices from `currentRawIndices()`.
- [ ] The Enter case returns BEFORE the `viewport.Update` delegation at the bottom of the function — i.e. viewport is not updated for the Enter event.
- [ ] When the user has not navigated, raw indices match the captured-at-open `(WindowIndex, PaneIndices[0])` of the first group.
- [ ] When the user has navigated via `]`/`[`/`Tab`, raw indices reflect the walked focus.
- [ ] On a session with non-contiguous `window_index` (e.g. 0, 2, 5) or non-zero `pane-base-index`, the dispatched indices are the raw tmux values, not slice positions.
- [ ] When `m.attacher` is nil, Enter is a silent no-op (returns `(m, nil)`).
- [ ] Enter dispatches the attach pipeline unconditionally regardless of viewport content state — real-bytes, `(nil, nil)` placeholder, and OS-level read error all produce identical dispatch behaviour. No confirmation prompt, no viewport-state guard.

**Tests**:
- `"Enter dispatches with captured-at-open raw indices when user has not navigated"`
- `"Enter dispatches with walked indices after Tab"`
- `"Enter dispatches with walked indices after ]"`
- `"Enter dispatches with raw tmux indices on non-contiguous-index session"`
- `"Enter is intercepted and not forwarded to viewport"`
- `"Enter is a no-op when attacher is nil"`
- `"Enter dispatches the pipeline when viewport rendered real bytes"` — construct previewModel with a ScrollbackReader returning real bytes, send Enter, assert attacher was called.
- `"Enter dispatches the pipeline when viewport rendered the (no saved content) placeholder"` — reader returns `(nil, nil)`, send Enter, assert attacher was called.
- `"Enter dispatches the pipeline when viewport rendered an OS read error"` — reader returns `(nil, errors.New("EIO"))`, send Enter, assert attacher was called.
```

**Resolution**: Pending
**Notes**: Also worth extending task 1-6 Edge Cases with a one-liner pinning the spec rationale, but the acceptance + tests change above is the load-bearing fix. The implementation does not need to change — only the task contract.

---
