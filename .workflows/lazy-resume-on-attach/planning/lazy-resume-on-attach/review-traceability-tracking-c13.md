# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. A small pane can lose the two key hints that say how to answer it

**Type**: Incomplete coverage
**Spec Reference**: §5.2 ("A pane too small for the card still says what it is… the title, the command and the key hints stack plainly without the card frame, down to the smallest pane a restore can produce. Enter and `d` act at every size. A waiting pane swallows every other key, so one that drew nothing would read as an ordinary restored pane with a dead keyboard."), §5.4 (the confirmation degrades the same way, carrying `y discard   esc cancel`)
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-2` (the size ladder's clamp rule), which both `3-3` and `3-4` stack through
**Move**: settled
**Change Type**: update-task

**Problem**:
A restored pane can easily be two or three rows tall — a saved window split three ways reproduces exactly that geometry on the next boot. The plan clamps the small-pane form from the top, so the rows that go are the ones at the bottom: the `⏎ resume` / `d discard` hints on the waiting panel, and `y discard   esc cancel` on the discard confirmation. A four-row pane holding a command that wraps to three rows shows the title and the command and no hints at all.

That pane swallows every key it is not answered with. Without the hint row the user is looking at a Portal screen with a dead keyboard and nothing on it saying which two keys act — which is the precise failure the degraded form exists to prevent, and the same failure the plan's own reasoning gives for refusing a clipped frame ("a frame missing its bottom rows hides the key hints"). The specification puts the key hints in the small-pane form down to the smallest pane a restore can produce and states that both keys act at every size; a top-down clamp does not deliver that.

**Proposal**:
Clamp by keeping the stack's first and last rows and dropping from the rows between them. Both screens build their stack with the title first and the key hints last, so this preserves the hints at every pane down to two rows without the ladder needing to know what any row means — and it costs a body row instead, which is the row the specification's own enumeration can spare. A one-row pane still renders the first row alone, as the task already says. Determined by §5.2's degraded enumeration and its stated reason, and by §5.4's "`y` and Escape act at every size, as Enter and `d` do".

**Current**:
```markdown
- Clamp the plain stack to the pane's rows top-down (the title first) and to its columns, so a pane too short drops trailing rows rather than overflowing the pane and scrolling the transcript underneath it.
```

```markdown
- [ ] Content taller or wider than the pane is clamped rather than overflowing: a stack of twenty rows in a five-row pane renders five rows, and they are the first five.
```

```markdown
- `"it clamps content taller than the pane"`
```

```markdown
- Content is clamped to the pane's rows and columns rather than overflowing: a render taller than the pane would scroll the pane's primary buffer, which is where the user's replayed transcript is sitting.
```

```markdown
- The smallest pane a restore can produce still draws the title, the command and the key hints — asserted at a realistically small pane as well as at the degenerate `1x1`, where the single row must be the title.
```

**Proposed Text**:
```markdown
- Clamp the plain stack to the pane's rows by keeping its first row and its last row and dropping from the end of the rows between them, and to its columns, so a pane too short loses body rows rather than the row its screen put last, and never overflows the pane and scrolls the transcript underneath it. A one-row pane renders the first row alone.
```

```markdown
- [ ] Content taller or wider than the pane is clamped rather than overflowing, and the clamp keeps the stack's first and last rows: a twenty-row stack in a five-row pane renders the first row, the three after it and the last row; a six-row stack in a three-row pane renders the first row, the one after it and the last row; a two-row pane renders the first and the last; a one-row pane renders the first alone.
```

```markdown
- `"it clamps content taller than the pane"`
- `"it keeps the stack's first and last rows when it clamps"` (table: twenty rows in five, six rows in three, six rows in two, six rows in one)
```

```markdown
- Content is clamped to the pane's rows and columns rather than overflowing: a render taller than the pane would scroll the pane's primary buffer, which is where the user's replayed transcript is sitting. What goes is taken from the middle, because both screens stack their key hints last and a pane that swallows every key it is not answered with must never be the one that hides which keys answer it — the same failure a clipped frame produces, and the reason the degraded form exists at all.
```

```markdown
- The smallest pane a restore can produce still draws the title, the command and the key hints — asserted at a realistically small pane, at a three-row pane holding a command that wraps past one row, and at the degenerate `1x1`, where the single row must be the title.
```

**Resolution**: Pending
**Notes**:

---

### 2. The `● PAUSED` badge is put on a screen the specification does not put it on

**Type**: Hallucinated content
**Spec Reference**: §5.2 ("Below the size the card needs, the panel degrades instead of disappearing: the canvas is painted as always, and the title, the command and the key hints stack plainly without the card frame"); §5.3 enumerates the badge as part of the card's header row and states the panel "carries this, and carries nothing else"
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-3` (the plain stack's title row)
**Move**: settled
**Change Type**: update-task

**Problem**:
The small-pane form of the waiting panel is specified as three things — the title, the command and the key hints. The plan adds a fourth: the `● PAUSED` badge, appended to the title row whenever the pane is wide enough to hold both. The badge is card content — it occupies the header slot the rename modal gives `◉ EDIT MODE`, and there is no header on a screen with no frame — so a small pane would show a composition the design never described and the visual gate has no reference for. It is also the one presentational addition in Phase 3 the plan does not name as its own call, while the same task's acceptance criterion for the degraded form lists only the three specified parts, so the task contradicts itself about what that screen carries.

**Proposal**:
Drop the badge from the plain stack, leaving the title row alone there. That is the specification's own enumeration of the degraded form, and it matches both the task's existing acceptance criterion and the discard confirmation's plain stack, which carries exactly the four parts §5.4 enumerates for it. Nothing is lost under `NO_COLOR` either: the specification lists the badge among the card's glyph-backed carriers and does not carry it into the degraded form.

**Current**:
```markdown
- Build the plain stack from the same pieces at the pane's width: the title row (with the badge appended after a gap when the width holds both, title alone when it does not), the command rows wrapped to the pane's width, the report row when present, then the key-hint row. The `ON RESUME` label belongs to the card and is not stacked: the small-pane form is the title, the command and the key hints, and every row ahead of the hints is a row the pane's top-down clamp can cost them.
```

```markdown
- [ ] Below the card's size the panel renders the plain stack carrying the title, the command, the report when present and the key hints, and never an empty screen; the `ON RESUME` label is absent from it.
```

```markdown
- `"it drops the ON RESUME label from the plain stack"`
```

**Proposed Text**:
```markdown
- Build the plain stack from the same pieces at the pane's width: the title row alone, the command rows wrapped to the pane's width, the report row when present, then the key-hint row. Neither the `● PAUSED` badge nor the `ON RESUME` label is stacked — both are card parts, the badge occupying a header slot a frameless screen does not have, and the small-pane form is the title, the command and the key hints. The title row is the stack's first row and the key-hint row its last, which is what the pane's clamp keeps.
```

```markdown
- [ ] Below the card's size the panel renders the plain stack carrying the title, the command, the report when present and the key hints, and never an empty screen; neither the `ON RESUME` label nor the `● PAUSED` badge appears on it.
```

```markdown
- `"it drops the ON RESUME label and the PAUSED badge from the plain stack"`
```

**Resolution**: Pending
**Notes**:
