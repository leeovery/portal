# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The plan's index of the drawing process drops the one edge case that qualifies a promise another task makes

**Severity**: Minor
**Plan Reference**: Phase 4, task table row `lazy-resume-on-attach-4-1` (The panel-drawing process)
**Category**: Task Template Compliance
**Move**: settled
**Change Type**: update-task

**Problem**:
Task 4-2 states, as a plain guarantee, that a byte left unread when a key hands the pane over is inherited by whatever the handover execs — "only the confirmation drops input in flight". Task 4-1 narrows that guarantee: under an adaptive light/dark pair the draw's appearance probe reads the pane's stdin until a terminator or the detect timeout, so a keystroke typed inside that window on a redraw is swallowed rather than inherited. The narrowing is written in task 4-1's own edge cases, but it is the one item missing from the plan's task table for that task — the ten-item list there carries every other edge case in the same order. Anyone reading the plan rather than the task file — a later reviewer, or whoever next changes the wait loop's input handling — meets 4-2's inherited-bytes rule stated absolutely and sees nothing at plan level that qualifies it. The plan's convention through every prior cycle has been that the table row carries the same clauses the task does.

**Proposal**:
Insert the missing clause into the `lazy-resume-on-attach-4-1` table row in its task-file position — between the exec-target clause and the exec-failure clause — compressed to the table's one-clause-per-item, comma-free style. Determined by the plan's own convention: the other ten items on that row are the task's other ten edge cases, in the task's order.

**Current**:
```
| lazy-resume-on-attach-4-1 | The panel-drawing process | a failed or zero size read falls back to the bounded render rather than painting nothing, the alternate-screen entry is written before the paint and never left by this process, the enter and leave sequences are the one pair `cmd` already owns rather than a second spelling, NO_COLOR paints no canvas and writes no query, a prefs read that fails degrades to the shipped pair rather than blocking the draw, the theme read is the non-migrating one so every pane drawing at boot never races the one-shot appearance translation, a themes directory that will not resolve still paints from the embedded built-ins, the process role resolves to the existing hydrate role so the closed role space gains no member, the exec target carries everything a redraw needs so no screen re-reads the store to decide whether to draw, an exec that fails exits non-zero and leaves the chain's tail to recover the pane |
```

**Proposed Text**:
```
| lazy-resume-on-attach-4-1 | The panel-drawing process | a failed or zero size read falls back to the bounded render rather than painting nothing, the alternate-screen entry is written before the paint and never left by this process, the enter and leave sequences are the one pair `cmd` already owns rather than a second spelling, NO_COLOR paints no canvas and writes no query, a prefs read that fails degrades to the shipped pair rather than blocking the draw, the theme read is the non-migrating one so every pane drawing at boot never races the one-shot appearance translation, a themes directory that will not resolve still paints from the embedded built-ins, the process role resolves to the existing hydrate role so the closed role space gains no member, the exec target carries everything a redraw needs so no screen re-reads the store to decide whether to draw, the appearance probe is the draw's one stdin read and runs before the alternate-screen entry so a byte it swallows on a redraw can only be one that arrived before a screen was painted, an exec that fails exits non-zero and leaves the chain's tail to recover the pane |
```

**Resolution**: Fixed — the `lazy-resume-on-attach-4-1` table row gains the appearance-probe clause in its task-file position, between the exec-target and exec-failure clauses.
**Notes**: Verified before applying — task 4-1 carries eleven edge cases and the table row carried ten, the missing one being exactly the probe clause. Task file `phase-4-tasks.md` and the tick body already carry the edge case in full; only the plan-level table row needs the edit. No task-file or tick change follows from this finding.

---
