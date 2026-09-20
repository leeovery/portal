# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. The small-pane resume panel spends a row on a label the specification's small-pane form does not carry

**Type**: Hallucinated content
**Spec Reference**: §5.2 ("Below the size the card needs, the panel degrades instead of disappearing: the canvas is painted as always, and the title, the command and the key hints stack plainly without the card frame, down to the smallest pane a restore can produce. Enter and `d` act at every size. A waiting pane swallows every other key, so one that drew nothing would read as an ordinary restored pane with a dead keyboard."); §5.4 for the confirmation's own enumeration, which the plan does follow
**Plan Reference**: Phase 3 — `lazy-resume-on-attach-3-3` (Do, Acceptance Criteria, Tests) and `lazy-resume-on-attach-3-2` (Edge Cases)
**Move**: settled
**Change Type**: update-task

**Problem**:
A pane too short for the card can come back holding the panel with no `⏎ resume` / `d discard` line on it. The panel below the card's size stacks its parts top-down and drops whatever does not fit, and the plan puts an `ON RESUME` label row between the title and the command — a row the specification's small-pane form does not carry. In a four-row pane holding a command that wraps to two lines, that label takes the row the key hints would have had. A waiting pane ignores every key but those two, so a user looking at a panel that names neither of them sees a restored pane with a dead keyboard and no way to tell what it wants — the exact outcome the small-pane rule exists to prevent. The discard confirmation is built the other way in the same phase: its stack carries the four parts §5.4 names and no more, so the two screens currently disagree about what a small pane shows.

**Proposal**:
The specification enumerates the small-pane panel as the title, the command and the key hints; the card keeps the `ON RESUME` label, the stack does not. Drop the label from the degraded stack, keep the badge where the plan already puts it (appended to the title row, so it costs no row and carries the paused state under `NO_COLOR` where hue is gone), and keep the report row immediately above the hints as it sits on the card — it is conditional content, present only when there is something to report, and a report that could not be shown at a small size would leave the user believing an answer was carried out. This brings the waiting panel's stack into line with the confirmation's, which already carries exactly its own §5.4 enumeration. Task 3.2's ladder wording is amended in the same breath so the two tasks do not describe the stack differently.

**Current**:

Task `lazy-resume-on-attach-3-3`, **Do**:

```
- Build the plain stack from the same pieces at the pane's width: the title row (with the badge appended after a gap when the width holds both, title alone when it does not), the label, the command rows wrapped to the pane's width, the report row when present, then the key-hint row.
```

Task `lazy-resume-on-attach-3-3`, **Acceptance Criteria**:

```
- [ ] Below the card's size the panel renders the plain stack carrying the title, the label, the command, the report when present and the key hints, and never an empty screen.
```

Task `lazy-resume-on-attach-3-3`, **Tests**:

```
- `"it renders the plain stack below the card's size"`
- `"it draws the title the command and the key hints at the smallest pane"`
```

Task `lazy-resume-on-attach-3-2`, **Edge Cases**:

```
- The plain stack keeps every part the card carried, including the report row — the parts are the screen's, and which of them survive is decided by the pane's rows, not by which screen is drawing.
```

**Proposed Text**:

Task `lazy-resume-on-attach-3-3`, **Do** — replace that bullet with:

```
- Build the plain stack from the same pieces at the pane's width: the title row (with the badge appended after a gap when the width holds both, title alone when it does not), the command rows wrapped to the pane's width, the report row when present, then the key-hint row. The `ON RESUME` label belongs to the card and is not stacked: the small-pane form is the title, the command and the key hints, and every row ahead of the hints is a row the pane's top-down clamp can cost them.
```

Task `lazy-resume-on-attach-3-3`, **Acceptance Criteria** — replace that criterion with these two:

```
- [ ] Below the card's size the panel renders the plain stack carrying the title, the command, the report when present and the key hints, and never an empty screen; the `ON RESUME` label is absent from it.
- [ ] A four-row pane holding a command that wraps to two rows renders the title, both command rows and the key hints — the stack spends no row on anything the small-pane form does not carry.
```

Task `lazy-resume-on-attach-3-3`, **Tests** — replace those two entries with:

```
- `"it renders the plain stack below the card's size"`
- `"it drops the ON RESUME label from the plain stack"`
- `"it renders the title the command and the key hints in a four-row pane"` (command wrapping to two rows)
- `"it draws the title the command and the key hints at the smallest pane"`
```

Task `lazy-resume-on-attach-3-2`, **Edge Cases** — replace that bullet with:

```
- The plain stack keeps every part its screen's builder hands it, including the report row — which parts a screen stacks is the screen's own decision, and which of them survive is decided by the pane's rows, not by the ladder.
```

**Resolution**: Pending
**Notes**:

---
