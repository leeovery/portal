---
status: in-progress
created: 2026-05-07
cycle: 1
phase: Traceability Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Traceability

Re-read the specification in full. Walked every spec section, decision, edge case, constraint, data-model element, integration point, validation rule, and acceptance criterion against the plan's four phases and 29 tasks. Then walked every task's Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases back to the specification.

The plan is overall a faithful, near-complete translation of the specification — every architectural choice (always-disk reads, sequential window-grouped rendering, constructor-injected seams, three-shape `Tail` contract, `]` `[` `Tab` keymap, 1-based ordinal chrome counters, fresh-model-per-open lifecycle, side-effect-free invariant) is represented by a task with matching acceptance criteria. Phase ordering follows the spec's data-flow architecture (read pipeline foundations → page entry/single-pane → multi-pane cycling/chrome → edge cases). Out-of-scope items (live capture, literal layout rendering, in-preview stepping, deeper history, auto-refresh, position memory, command/position hints, privacy toggle, off-Sessions-page entry points, preview-layer `_portal-saver` suppression) are correctly absent.

One incomplete-coverage finding stands: the spec's "surface label honesty" constraint (no chrome wording may promise liveness) is not represented in the chrome-rendering task's acceptance criteria. The constraint is a negative behavioural rule on chrome wording, and chrome wording is itself an open item handed to the build phase — without pinning the constraint in the plan, the build-phase wording choice is unsupervised against this requirement.

No hallucinated content was found. Minor implementation-detail expansions (e.g. `PORTAL_SKIP_PERF` env opt-out for the perf benchmark, defensive handling of `|` in window names, working error-string label `"(unable to read scrollback)"`) are reasonable build-phase pinning of spec open items, not invented requirements.

## Findings

### 1. Surface label honesty constraint missing from chrome rendering task

**Type**: Incomplete coverage
**Spec Reference**: § *Source of Preview Bytes > Surface label honesty*: "Preview is a snapshot, not 'what attaching now would show'. Any user-facing labelling must not promise liveness."
**Plan Reference**: Phase 3, Task 3-5 (Chrome rendering: counters, window name, and keystroke hints)
**Change Type**: add-to-task

**Details**:
The spec mandates that any user-facing labelling in preview must not promise liveness, because preview is a snapshot, not a live view. Chrome is the only user-facing label surface in preview, and chrome wording is explicitly handed to the build phase as an Open Item ("Exact chrome wording, header vs footer, single-line vs two-line"). Without an acceptance criterion in 3-5 capturing the no-liveness-promise constraint, the build-phase wording choice has no traceable guard against accidentally adding live-sounding language (e.g. "Live preview", "Now showing", "Currently:"). The constraint is a negative behavioural rule that needs to be visible in the task that owns chrome wording so it survives the build-phase decision moment.

Task 3-5 currently asserts only structural content (counters, window name, hint keys). Adding a single acceptance criterion + a corresponding test pin closes the gap without changing the task's scope.

**Current**:
```markdown
**Acceptance Criteria**:
- [ ] `chromeLine()` returns a string containing `Window {wOrdinal} of {wTotal}` where `wOrdinal = windowIdx + 1` and `wTotal = len(groups)`.
- [ ] The string contains `Pane {pOrdinal} of {pTotal}` where `pOrdinal = paneIdx + 1` and `pTotal = len(currentGroup().PaneIndices)`.
- [ ] The string contains the window name (`currentGroup().WindowName`) verbatim, including any spaces or special characters.
- [ ] The string contains visible cycle-key hints: at minimum `]`, `[`, `Tab`, `Esc` each appear textually.
- [ ] Counters never expose raw tmux `WindowIndex` or `PaneIndices[i]` values — confirmed by a test case with non-contiguous indices.
- [ ] `chromeLine()` is pure: no I/O, no calls to `m.reader` or `m.enumerator`, no goroutines.

**Tests**:
- `"chromeLine renders 1-based ordinals for 0-indexed groups"`
- `"chromeLine renders 1..N counters when WindowIndex values are non-contiguous (0,2,5)"`
- `"chromeLine renders 1..N counters when PaneIndices start at 1 (pane-base-index 1)"`
- `"chromeLine includes the window name verbatim including spaces"`
- `"chromeLine includes ] [ Tab Esc as visible hints"`
- `"chromeLine produces no I/O when invoked (does not call reader or enumerator)"`
```

**Proposed**:
```markdown
**Acceptance Criteria**:
- [ ] `chromeLine()` returns a string containing `Window {wOrdinal} of {wTotal}` where `wOrdinal = windowIdx + 1` and `wTotal = len(groups)`.
- [ ] The string contains `Pane {pOrdinal} of {pTotal}` where `pOrdinal = paneIdx + 1` and `pTotal = len(currentGroup().PaneIndices)`.
- [ ] The string contains the window name (`currentGroup().WindowName`) verbatim, including any spaces or special characters.
- [ ] The string contains visible cycle-key hints: at minimum `]`, `[`, `Tab`, `Esc` each appear textually.
- [ ] Counters never expose raw tmux `WindowIndex` or `PaneIndices[i]` values — confirmed by a test case with non-contiguous indices.
- [ ] `chromeLine()` is pure: no I/O, no calls to `m.reader` or `m.enumerator`, no goroutines.
- [ ] The chrome wording does not promise liveness — no substrings such as `"live"`, `"now showing"`, `"current"`, `"realtime"`, or other language implying the rendered content is live tmux output. Preview is a snapshot per spec § *Source of Preview Bytes > Surface label honesty*; chrome wording must reflect that.

**Tests**:
- `"chromeLine renders 1-based ordinals for 0-indexed groups"`
- `"chromeLine renders 1..N counters when WindowIndex values are non-contiguous (0,2,5)"`
- `"chromeLine renders 1..N counters when PaneIndices start at 1 (pane-base-index 1)"`
- `"chromeLine includes the window name verbatim including spaces"`
- `"chromeLine includes ] [ Tab Esc as visible hints"`
- `"chromeLine produces no I/O when invoked (does not call reader or enumerator)"`
- `"chromeLine wording does not promise liveness"` — assert the rendered string (case-insensitive) contains none of the substrings `live`, `now showing`, `realtime`, `current command`, or other liveness-implying tokens; pin the negative-substring set in the test so future wording changes have to update the guard deliberately.
```

**Resolution**: Pending
**Notes**: The proposed criterion is a negative-substring guard, which is the simplest operationalisation of the spec constraint without dictating the build-phase's positive wording choice. The criterion can be revised down to a comment-level note if the orchestrator/user prefers a softer guard, but pinning it as an AC + test is the durable form.
