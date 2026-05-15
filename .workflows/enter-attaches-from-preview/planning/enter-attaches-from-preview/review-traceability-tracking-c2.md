---
status: in-progress
created: 2026-05-15
cycle: 2
phase: Traceability Review
topic: Enter Attaches From Preview
---

# Review Tracking: Enter Attaches From Preview - Traceability

## Cycle 1 Fix Verification

Cycle 1's single finding (unconditional viewport-content-state acceptance criterion + three tests on task 1-6) is correctly integrated in `phase-1-tasks.md`:

- Acceptance criterion present at line 333 — exact wording from cycle 1's Proposed block.
- Three viewport-state dispatch tests present at lines 342-344 — exact wording from cycle 1's Proposed block.
- Task 1-8 retains its independent chrome-wording invariance tests (real bytes / `(nil, nil)` / OS error) — no duplication; the two tasks pin different surfaces (dispatch vs chrome) of the same spec invariant.

No regression introduced by the cycle 1 fix.

---

## Findings

### 1. Missing acceptance criterion + test for "no re-enumeration on Enter" hard constraint

**Type**: Incomplete coverage
**Spec Reference**: § Pre-select + attach sequence > step 2 — "**No re-enumeration**: do NOT call `list-panes -F` or any structural enumeration on Enter. Re-enumeration would cost a round-trip on every Enter for an edge case that is bounded and self-correcting."
**Plan Reference**: phase-1-tasks.md task 1-4 (`enter-attaches-from-preview-1-4`)
**Change Type**: add-to-task

**Details**:
The spec includes a hard MUST-NOT — the Enter dispatch must not perform any structural enumeration (`list-panes -F`, `list-windows -F`, or equivalent) before, during, or after the four-call sequence. The captured-at-open enumeration is the single source of truth; re-enumeration would defeat the spec's per-Enter cost budget and re-introduce a window for state drift between enumeration and pre-select.

Task 1-4's current acceptance criteria pin the four-call ordering (`HasSessionProbe → SelectWindow → SelectPane → connector`) and assert each fires exactly once, but there is no explicit assertion that NO OTHER tmux call shape (notably `list-panes` / `list-windows`) is dispatched. A future implementer adding a defensive "let's re-check the pane is still there" enumeration would satisfy the existing acceptance criteria while violating the spec. The constraint needs to be pinned by an explicit acceptance bullet and a corresponding test against the fake commander's recorded call list.

Task 1-6 has a similar surface (the Enter case body) but the natural place for the assertion is task 1-4 since the pipeline owns all tmux orchestration.

**Current**:

```markdown
**Acceptance Criteria**:
- [ ] `previewAttachPipeline.Run` returns a non-nil `tea.Cmd`.
- [ ] On the success path, the cmd invokes HasSessionProbe, SelectWindow, SelectPane, then connector.Connect — in that exact order — exactly once each.
- [ ] On `(present=false, *exec.ExitError)` from HasSessionProbe, the cmd returns `previewAttachBailMsg{Session: <name>}` and DOES NOT invoke SelectWindow, SelectPane, or connector.
- [ ] On `(present=true, OS-layer-err)` from HasSessionProbe, the cmd logs at WARN with `ComponentPreview` and proceeds.
- [ ] SelectWindow non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] SelectPane non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] Connector error is returned as `previewAttachErrorMsg{Err: err}`.
- [ ] No call passes a `nil`-receiver session/window/pane combo through silently — empty session bails out before any tmux call (defensive guard).

**Tests**:
- `"pipeline runs has-session, select-window, select-pane, connector in order on success"`
- `"pipeline returns previewAttachBailMsg when has-session reports absent"` — and verifies the session name is preserved in the message.
- `"pipeline does not invoke selects or connector after a bail signal"`
- `"pipeline proceeds and logs on has-session OS-layer error"`
- `"pipeline logs WARN with ComponentPreview when select-window fails"`
- `"pipeline logs WARN with ComponentPreview when select-pane fails"`
- `"pipeline returns connector error as previewAttachErrorMsg"`
- `"pipeline forwards connector choice (Attach vs Switch) without orchestration changes"` — runs the same fixture with two connector implementations.
```

**Proposed**:

```markdown
**Acceptance Criteria**:
- [ ] `previewAttachPipeline.Run` returns a non-nil `tea.Cmd`.
- [ ] On the success path, the cmd invokes HasSessionProbe, SelectWindow, SelectPane, then connector.Connect — in that exact order — exactly once each.
- [ ] On `(present=false, *exec.ExitError)` from HasSessionProbe, the cmd returns `previewAttachBailMsg{Session: <name>}` and DOES NOT invoke SelectWindow, SelectPane, or connector.
- [ ] On `(present=true, OS-layer-err)` from HasSessionProbe, the cmd logs at WARN with `ComponentPreview` and proceeds.
- [ ] SelectWindow non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] SelectPane non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] Connector error is returned as `previewAttachErrorMsg{Err: err}`.
- [ ] No call passes a `nil`-receiver session/window/pane combo through silently — empty session bails out before any tmux call (defensive guard).
- [ ] The pipeline performs NO structural enumeration on Enter — no `list-panes`, no `list-windows`, no `list-sessions`, no `display-message -p`, and no other tmux call shape beyond the four spec-pinned commands (`has-session`, `select-window`, `select-pane`, and the connector's `attach-session` / `switch-client`). Verified by asserting the fake commander's recorded call list contains exactly those argv prefixes and no others.

**Tests**:
- `"pipeline runs has-session, select-window, select-pane, connector in order on success"`
- `"pipeline returns previewAttachBailMsg when has-session reports absent"` — and verifies the session name is preserved in the message.
- `"pipeline does not invoke selects or connector after a bail signal"`
- `"pipeline proceeds and logs on has-session OS-layer error"`
- `"pipeline logs WARN with ComponentPreview when select-window fails"`
- `"pipeline logs WARN with ComponentPreview when select-pane fails"`
- `"pipeline returns connector error as previewAttachErrorMsg"`
- `"pipeline forwards connector choice (Attach vs Switch) without orchestration changes"` — runs the same fixture with two connector implementations.
- `"pipeline does not invoke list-panes, list-windows, or any other enumeration on the success path"` — fake commander records every argv; assert the recorded set is exactly `{has-session, select-window, select-pane, attach-session-or-switch-client}` with no `list-*` or `display-message` calls.
- `"pipeline does not invoke list-panes, list-windows, or any other enumeration on the bail path"` — bail path runs only `has-session`; assert no enumeration calls follow.
```

**Resolution**: Pending
**Notes**: Spec § Pre-select step 2 calls re-enumeration out by name as a MUST-NOT to protect the per-Enter cost budget. Without an explicit assertion, the constraint is invisible to future maintainers and would not surface as a test failure if a defensive enumeration were added. The Edge Cases section of task 1-4 may also benefit from a one-line addition pinning the spec rationale, but the acceptance criterion + tests are the load-bearing fix.

---
