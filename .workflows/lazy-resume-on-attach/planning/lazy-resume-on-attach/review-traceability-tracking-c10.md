# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. The committed row frame draws the two indicators a cell apart; the plan builds them adjacent and never says so

**Type**: Incomplete coverage
**Spec Reference**: §8.3 — the two indicators pack right in a fixed order, and under `NO_COLOR` each renders as a letter in the same cell as its dot: `A` alone, `P` alone, or `AP` when both, with the packing order untouched and no second row geometry. §5.5 — the row frame is the only reference that exists for the reworked trailing region, and the comparison is judged for layout, structure and colour-role match rather than by pixel diff, with the tokens remaining the contract.
**Plan Reference**: Phase 6 → the reference-frame bullet of the phase's **Planner's calls**; task `lazy-resume-on-attach-6-8` (The row on demand for the visual check) → **Context**
**Move**: settled
**Change Type**: add-to-task

**Problem**:
The session row's two indicators come out of this work touching — one cell each, side by side — because that is the only form whose colourless rendering is the `AP` pair the design calls for, and the only one that keeps a row the same width whether it carries one indicator or two. The design export the plan sends the reader to at the Phase 6 visual check does not show that. Measured on the committed frame (`testdata/vhs/reference/sessions-pending-resume-dot-nord.png`): every single-indicator row draws its dot in the cell ending at x=1627, while the one row carrying both (`folio-Jiz4el`) draws the green attached dot in the cell ending at x=1589 — 38 px away at the frame's 17.85 px character pitch, which is one blank cell between the pair.

The plan names exactly this kind of divergence for the discard-confirmation frame, whose committed export carries superseded copy, so the reader arrives at that gate knowing which parts of the picture to trust. It says nothing of the kind here, and this frame is the sole reference for precisely the question it gets wrong — where the indicators sit relative to each other. The reader then either signs off a row that does not match the picture in front of them, or asks for the space to be added; and adding it makes the coloured pair three cells wide against a colourless pair of two, so a row with both indicators stops matching the width of a row with one, and the dots-versus-letters switch stops being a swap in place.

**Proposal**:
Name the divergence where the plan points the reader at the frame — once at the phase level and once in the task that produces the live render — and state that the design's written form governs, exactly as the plan already does for the discard-confirmation frame. Nothing about what gets built changes: the adjacent cluster the row task builds is already the form the specification states, and the width-identity criterion already pins it.

**Current**:

*(1) `planning.md`, Phase 6 → Planner's calls, final bullet:*

```
- The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, beside the sessions frames already there. It is the only design reference that exists for the reworked trailing region: those existing frames show the row this phase changes, with the word `attached` and a single indicator, so they cannot settle the packing, the spacing or the colourless form. The capture fixtures exist so the row can be rendered live and held against that frame; the check is the gate, not a criterion.
```

*(2) `phase-6-tasks.md`, task `lazy-resume-on-attach-6-8` → Context, final paragraph:*

```
> The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, and it is the design reference the phase's visual gate reads: the sessions frames already in that directory show the row before the word was dropped, so they say nothing about where the indicators sit. These fixtures exist so the row can be viewed live and held against that frame and against Portal's own existing row grammar; that check is the phase's gate, not a criterion of this task.
```

**Proposed Text**:

*(1) `planning.md`, Phase 6 → Planner's calls, final bullet — replaced by:*

```
- The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, beside the sessions frames already there. It is the only design reference that exists for the reworked trailing region: those existing frames show the row this phase changes, with the word `attached` and a single indicator, so they cannot settle the packing, the spacing or the colourless form. The capture fixtures exist so the row can be rendered live and held against that frame; the check is the gate, not a criterion. One thing on that frame diverges from what this phase builds, and it sits on the very question the frame is the reference for: its single two-indicator row draws the pair one blank cell apart, where the specification states the colourless form as `AP` — each letter in the cell its own dot occupies, with the packing order untouched and no second row geometry — which is the adjacent cluster the indicator tasks build. The specification's stated form governs; the frame is read for layout, structure and colour-role match rather than by pixel diff, exactly as the two panel frames are.
```

*(2) `phase-6-tasks.md`, task `lazy-resume-on-attach-6-8` → Context, final paragraph — replaced by:*

```
> The frame the specification names for this row — **Sessions — pending resume dot (Nord)** — is committed at `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`, and it is the design reference the phase's visual gate reads: the sessions frames already in that directory show the row before the word was dropped, so they say nothing about where the indicators sit. These fixtures exist so the row can be viewed live and held against that frame and against Portal's own existing row grammar; that check is the phase's gate, not a criterion of this task.
>
> That frame diverges from what these fixtures render in one visible way, and the divergence lands on the question it is the sole reference for. Its one two-indicator row (`folio-Jiz4el`) draws the attached dot one blank cell to the left of the pending one; every single-indicator row draws its dot in the same rightmost cell, so the packing order and the right edge match, and only the gap between the pair does not. The specification states the pair as `AP` under `NO_COLOR` — each letter in the cell its own dot occupies, with no second row geometry — which is the adjacent cluster task 6.5 builds and which the row's width-identity criterion pins. The specification's stated form governs, and the frame is read at the gate for layout, structure and colour-role match rather than by pixel diff: a space inserted between the indicators to match it would make the coloured pair three cells wide against a colourless pair of two, so a row carrying both would stop matching the width of a row carrying one.
```

**Resolution**: Pending
**Notes**:

---
