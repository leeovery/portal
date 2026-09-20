# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The discard confirmation cuts the sentence that says the act is permanent, on the pane sizes the degraded form exists for

**Severity**: Important
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-4` (The discard confirmation)
**Category**: Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
Below the size the card needs, the discard confirmation stacks its parts plainly on the pane — and every part except one is built at the pane's width. The command rows and the report row are re-wrapped and `…`-marked to the pane by the shared helpers; the consequence line is taken from the destructive builder, which wraps at its own fixed 52-cell content width and is never told what the pane is. The card only fits a pane of roughly 58 columns or more, so every pane below that renders the plain stack, and every pane below 52 columns renders it with a consequence row wider than the pane. The canvas clamps it, so what the user actually sees on a 45-column pane is `Removes this pane's resume command permanentl` with the rest of the word gone and a second line orphaned under it. That is the one sentence that tells them `y` destroys the only copy of their command, and a 40-to-50-column pane is an ordinary two- or three-way split. Following the plan as written produces it: the Do bullet says in as many words that the consequence keeps the builder's own wrap.

**Proposal**:
Wrap the consequence at the width the stack is built for, exactly as the command and report rows already are — the plan's own rule for this screen, stated in task 3-1: a row is wrapped at the width it will be shown at so the canvas is never handed a row it has to cut. The compartments accessor the task is already introducing takes a wrap width; the card path passes `destructiveBodyWidth`, which is what the builder wraps at today, so the kill and delete modals and their byte-identical golden are untouched, and the plain-stack path passes the pane's width. One acceptance criterion, one test and one edge case pin it.

**Current**:

*`planning.md`, Phase 3 task table, the `lazy-resume-on-attach-3-4` row:*

```
| lazy-resume-on-attach-3-4 | The discard confirmation | the kill and delete modals render byte-identically after the builder's compartments are exposed, the command takes the multi-line block where the kill modal takes a one-line session name, a report row on the confirmation itself on the same single row the waiting panel gives one, the confirmation degrades with the pane exactly as the waiting panel does, under NO_COLOR the `▲` and the words carry the destructive signal where the token drops, the consequence line is plain language and names no tool, nothing structural differs from the kill modal |
```

*`phase-3-tasks.md`, task 3.4, **Do**, first bullet:*

```
- In `internal/tui/destructive_confirm.go`: add `targetRows []string` and `reportRows []string` to `destructiveConfirmSpec` — `targetRows`, when non-empty, replaces the single `destructiveNameRow`; `reportRows` is appended after the consequence rows — and split the compartment assembly into `destructiveConfirmCompartments(spec, th, colourless) [][]string`, with `renderDestructiveConfirm` reduced to `renderJoinedPanel(destructiveConfirmCompartments(…), th.Border, th, colourless)`.
```

*`phase-3-tasks.md`, task 3.4, **Do**, fourth bullet:*

```
- Build the plain stack by flattening `destructiveConfirmCompartments` for a spec built at the pane's width — the same title, consequence, confirm key and label, with `targetRows` and `reportRows` rebuilt through the shared helpers at that width — so the degraded screen carries the title, the command, the consequence line, the report when present and the key hints without the frame, cannot drift from the card, and wraps the command exactly as the waiting panel's stack does. The consequence keeps the builder's own wrap, as the kill modal's does.
```

*`phase-3-tasks.md`, task 3.4, **Acceptance Criteria**, the degraded-form criterion:*

```
- [ ] Below the card's size the confirmation renders the plain stack carrying the title, the command, the consequence line, the report when present and `y discard   esc cancel`, and never an empty screen.
```

*`phase-3-tasks.md`, task 3.4, **Tests**, the degraded-form test:*

```
- `"it degrades to the plain stack below the card's size"`
```

*`phase-3-tasks.md`, task 3.4, **Edge Cases**, the degradation entry:*

```
- The confirmation degrades with the pane exactly as the waiting panel does — below the card's size the frame goes and the parts stack plainly, because a confirmation that drew nothing would leave the user pressing the key the footer offered a moment earlier against a question they never saw.
```

**Proposed Text**:

*`planning.md`, Phase 3 task table, the `lazy-resume-on-attach-3-4` row:*

```
| lazy-resume-on-attach-3-4 | The discard confirmation | the kill and delete modals render byte-identically after the builder's compartments are exposed, the command takes the multi-line block where the kill modal takes a one-line session name, a report row on the confirmation itself on the same single row the waiting panel gives one, the confirmation degrades with the pane exactly as the waiting panel does, the consequence re-wraps to the pane in the degraded form rather than being cut mid-word by the canvas, under NO_COLOR the `▲` and the words carry the destructive signal where the token drops, the consequence line is plain language and names no tool, nothing structural differs from the kill modal |
```

*`phase-3-tasks.md`, task 3.4, **Do**, first bullet:*

```
- In `internal/tui/destructive_confirm.go`: add `targetRows []string` and `reportRows []string` to `destructiveConfirmSpec` — `targetRows`, when non-empty, replaces the single `destructiveNameRow`; `reportRows` is appended after the consequence rows — and split the compartment assembly into `destructiveConfirmCompartments(spec destructiveConfirmSpec, wrapWidth int, th theme.Theme, colourless bool) [][]string`, which wraps the consequence at `wrapWidth` rather than at the package constant, with `renderDestructiveConfirm` reduced to `renderJoinedPanel(destructiveConfirmCompartments(spec, destructiveBodyWidth, th, colourless), th.Border, th, colourless)` — the framed path passes the width the builder wraps at today, so the kill and delete modals are untouched.
```

*`phase-3-tasks.md`, task 3.4, **Do**, fourth bullet:*

```
- Build the plain stack by flattening `destructiveConfirmCompartments` at the pane's width — the same title, consequence, confirm key and label, with `targetRows` and `reportRows` rebuilt through the shared helpers at that width — so the degraded screen carries the title, the command, the consequence line, the report when present and the key hints without the frame, cannot drift from the card, and wraps the command exactly as the waiting panel's stack does. The consequence wraps at that same width: every part of the stack is built at the width it will be shown at, so the canvas is never handed a row it has to cut.
```

*`phase-3-tasks.md`, task 3.4, **Acceptance Criteria**, the degraded-form criterion, plus one new criterion after it:*

```
- [ ] Below the card's size the confirmation renders the plain stack carrying the title, the command, the consequence line, the report when present and `y discard   esc cancel`, and never an empty screen.
- [ ] No row of the plain stack is wider than the pane it was built for, the consequence rows included: at a pane narrower than the builder's own wrap width the consequence is re-wrapped to the pane, every word of it still present, rather than being cut mid-word by the canvas.
```

*`phase-3-tasks.md`, task 3.4, **Tests**, the degraded-form test, plus one new test after it:*

```
- `"it degrades to the plain stack below the card's size"`
- `"it wraps the consequence to the pane in the plain stack"` (a pane narrower than the builder's wrap width: no row exceeds the pane and the sentence's words are all present)
```

*`phase-3-tasks.md`, task 3.4, **Edge Cases**, the degradation entry, plus one new entry after it:*

```
- The confirmation degrades with the pane exactly as the waiting panel does — below the card's size the frame goes and the parts stack plainly, because a confirmation that drew nothing would leave the user pressing the key the footer offered a moment earlier against a question they never saw.
- The consequence re-wraps with the pane in the degraded form. The card only fits a pane of roughly the builder's wrap width plus its frame, so the plain stack is what a two- or three-way split actually renders, and a consequence left at the builder's own width there is a row the canvas cuts mid-word — on the one screen whose job is to make an irreversible act deliberate. The framed path still passes the builder's own width, so the kill and delete modals and their byte-identical golden are untouched.
```

**Resolution**: Fixed — task 3-4's compartments accessor gains a `wrapWidth` parameter (framed path passes `destructiveBodyWidth`, so kill/delete are untouched; the stack path passes the pane's width), the fourth Do bullet states the every-part-at-its-own-width rule, and one acceptance criterion, one test and one edge case pin it. Phase 3's task-table row carries the same clause. Tick body re-synced and byte-verified.
**Notes**: Verified before applying — `destructiveBodyWidth = 52` at `internal/tui/destructive_confirm.go:15`, applied unconditionally by `ansi.Wordwrap` at line 65. Both `destructiveConfirmCompartments` call sites in the plan are inside task 3-4, so the signature change strands nothing outside it.

---

### 2. The visual gate for the discard confirmation points at a committed frame the plan's own copy contradicts

**Severity**: Important
**Plan Reference**: Phase 3, task `lazy-resume-on-attach-3-4` (The discard confirmation); the frame is also named by task `lazy-resume-on-attach-3-6` and by Phase 3's acceptance criteria
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: add-to-task

**Problem**:
Phase 3 ends at a visual gate where the live discard confirmation is held against the committed frame `testdata/vhs/reference/resume-panel-discard-confirm-nord.png`. That frame was exported before the specification's 2026-09-19 corrigendum, and it disagrees with what this plan builds in two ways a reader cannot miss: its consequence line reads `Removes this pane's resume command for good. It won't come back after a restart. Can't be undone.` where the task's constant is `Removes this pane's resume command permanently. The session and its scrollback are untouched.`, and it renders the command truncated to a single row with a ` · ~/Code/flowx` directory trailer in the kill modal's trailer slot, where this screen wraps the command over up to three rows and shows no directory at all. Nothing in the plan says which one wins. The person at the gate is therefore being asked to sign off a screen against a reference it visibly fails, and the two available moves are both wrong: reject a correct render, or bring the copy back to the frame and undo the corrigendum that chose those words and the design decision that cut the meta.

**Proposal**:
Say in the task that the frame predates the corrigendum, name the two divergences, and state that the specification's strings govern and the frame is read for card geometry, compartment structure and colour-role match. The specification settles this outright: its corrigendum states the consequence line verbatim and in bold, and §5.3 records that the directory meta was drafted and cut. One Context paragraph in task 3.4 — the task that owns both constants — carries it to the executor and to the gate.

**Current**:

*`phase-3-tasks.md`, task 3.4, **Context**, the final two paragraphs:*

```
> The specification places the waiting panel's report row "between the command and the key hints" and says the confirmation's sits "on the same single line" — which on a screen whose body is command-then-consequence leaves the row's exact position open. It is rendered as the last body row, immediately above the key hints, which satisfies both statements literally; nothing downstream depends on the choice and the visual gate can settle it.
>
> Dispatching `d`, `y` and Escape, dropping input already in flight, and removing the registration are Phase 5's. This task renders the screen.
```

**Proposed Text**:

*`phase-3-tasks.md`, task 3.4, **Context**, the final two paragraphs with one paragraph inserted between them:*

```
> The specification places the waiting panel's report row "between the command and the key hints" and says the confirmation's sits "on the same single line" — which on a screen whose body is command-then-consequence leaves the row's exact position open. It is rendered as the last body row, immediately above the key hints, which satisfies both statements literally; nothing downstream depends on the choice and the visual gate can settle it.
>
> The committed frame for this screen — `testdata/vhs/reference/resume-panel-discard-confirm-nord.png` — predates the corrigendum that stated the consequence line verbatim, and it diverges from what this task builds in two visible ways. It carries the earlier drafted wording (`Removes this pane's resume command for good. It won't come back after a restart. Can't be undone.`) where the constant above is the one the specification now states; and it renders the command truncated to a single row with a ` · ~/Code/flowx` trailer in the kill modal's trailer slot, where this screen wraps the command over up to three rows and carries no directory — the meta that was drafted and cut. The specification's stated strings and this task's constants govern both. The frame is read at the phase's visual gate for card geometry, compartment structure and colour-role match — which is what it was built by duplicating the Nord kill modal for — and never for copy.
>
> Dispatching `d`, `y` and Escape, dropping input already in flight, and removing the registration are Phase 5's. This task renders the screen.
```

**Resolution**: Fixed — task 3-4's Context gains a paragraph naming the frame, both divergences and the rule that the specification's strings govern while the frame is read for geometry, structure and colour roles. Tick body re-synced and byte-verified.
**Notes**: Verified by opening the committed frame before applying: it does carry `Removes this pane's resume command for good. It won't come back after a restart. Can't be undone.` and a single-row `claude --resume 4560…  · ~/Code/flowx`, both as the finding states. The waiting-panel and pending-dot frames were left alone — neither was reported as diverging and neither is touched by this fix.
