---
status: in-progress
created: 2026-05-09
cycle: 1
phase: Plan Integrity Review
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Integrity

## Findings

### 1. Task 2-1 Tests section contains authoring meta-commentary

**Severity**: Minor
**Plan Reference**: Phase 2 → Task daemon-merge-reintroduces-dead-sessions-2-1 → Tests (`phase-2-tasks.md` line 41 and the corresponding tick description)
**Category**: Task Template Compliance / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The first bullet of the Tests section reads as a stream-of-consciousness draft — it begins with a hypothetical that the author then talks themselves out of mid-sentence ("...wait, this overlaps with task 2-3's zero-panes guard. Use markers..."). The intended test description is recoverable but the prose is meant for the implementer, not for the author's own working notes. Polish to a single, declarative test name and description so an implementer reads a clean spec.

**Current**:
```
**Tests**:
- `"it unsets a marker whose paneKey is not present in the live-pane set"` — given marker `{x}` and zero live panes... wait, this overlaps with task 2-3's zero-panes guard. Use markers `{stale__0.0, live__0.0}` and live panes `{live:0.0}` so the live-pane set is non-empty and the zero-panes guard does not skip. Assert exactly one unset call for `@portal-skeleton-stale__0.0`.
- `"it leaves a marker alone whose paneKey is present in the live-pane set"` — given marker `{live__0.0}` and live pane `live:0.0`, assert zero unset calls.
- `"it requests live panes with the canonical session:window.pane format"` — assert the format string passed to `ListAllPanesWithFormat` is `#{session_name}:#{window_index}.#{pane_index}` exactly.
- `"it composes the option name from SkeletonMarkerPrefix"` — assert the option name passed to `UnsetServerOption` is `@portal-skeleton-<paneKey>` (constructed via the `state.SkeletonMarkerPrefix` constant; verifiable by checking the literal value matches `state.SkeletonMarkerPrefix + paneKey`).
```

**Proposed**:
```
**Tests**:
- `"it unsets a marker whose paneKey is not present in the live-pane set"` — given markers `{stale__0.0, live__0.0}` and live panes `{live:0.0}` (non-empty live set so the zero-panes guard from task 2-3 does not short-circuit), assert exactly one unset call for `@portal-skeleton-stale__0.0` and zero unset calls for `@portal-skeleton-live__0.0`.
- `"it leaves a marker alone whose paneKey is present in the live-pane set"` — given marker `{live__0.0}` and live pane `live:0.0`, assert zero unset calls.
- `"it requests live panes with the canonical session:window.pane format"` — assert the format string passed to `ListAllPanesWithFormat` is `#{session_name}:#{window_index}.#{pane_index}` exactly.
- `"it composes the option name from SkeletonMarkerPrefix"` — assert the option name passed to `UnsetServerOption` is `@portal-skeleton-<paneKey>` (constructed via the `state.SkeletonMarkerPrefix` constant; verifiable by checking the literal value matches `state.SkeletonMarkerPrefix + paneKey`).
```

**Resolution**: Pending
**Notes**:

---
