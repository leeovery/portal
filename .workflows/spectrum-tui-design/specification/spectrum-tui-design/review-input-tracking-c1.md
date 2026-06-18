---
status: in-progress
created: 2026-06-18
cycle: 1
phase: Input Review
topic: spectrum-tui-design
---

# Review Tracking: spectrum-tui-design - Input Review

## Findings

### 1. Edit-modal: default landing element when Tab enters a chip field (Aliases/Tags) is unspecified

**Source**: discussion `spectrum-tui-design.md` — "Edit modal — interaction model (proposed, mocked)", lines 476–478: *"**Tab into Aliases/Tags lands on the `+ add` slot** (input ready) — adding is the common action; `←` reaches the chips. So `Tab` from Aliases → Tags (next field); `→` is what reaches the next chip."*
**Category**: Gap/Ambiguity
**Affects**: §8.2 (Edit Project modal — Navigate mode)

**Details**:
The discussion made an explicit decision about *which element receives focus when you `Tab` into a chip field*: it lands on the trailing `+ add` slot (adding being the common action), with `←` then reaching the existing chips. The spec's §8.2 describes `←/→` moving "across chips and the trailing `+ add` slot" but never states the **default landing position** when a chip field is first entered via `Tab`/`Shift+Tab`. An implementer is left to guess whether focus lands on the first chip or on `+ add`.

This detail is genuinely ambiguous (not just omitted) because the final two-mode model changed the meaning of `+ add`: in the proposed model `+ add` was a passive inline input slot, but in the decided model (discussion lines 504–508, spec §8.2 Edit mode) *landing on `+ add` spawns a new empty chip already in edit mode*. So "Tab lands on `+ add`" under the new model would imply Tab-into-a-chip-field auto-enters edit mode on a fresh empty chip — which may or may not be the intent. The model UPDATE in the discussion only explicitly superseded the "no-in-place-edit" and "batch-all" calls; it never restated the landing-position decision, leaving the interaction undefined for the field the spec ships.

Worth a one-line clarification in §8.2 so the navigate-mode entry point into a chip field (and whether it auto-enters edit mode) is unambiguous.

**Proposed Addition**:
(leave blank until discussed)

**Resolution**: Pending
**Notes**:
