---
status: complete
created: 2026-07-23
cycle: 4
phase: Input Review
topic: remote-trigger-spawns-on-local-terminal
---

# Review Tracking: remote-trigger-spawns-on-local-terminal - Input Review

## Findings

### 1. Residual same-second edge narrowed — investigation says "same/later second", spec says only "same second"

**Source**: investigation.md — H3 (line 68: "a local client active in the **same/later second** is a residual edge") and Contributing Factors (line 118: "a local client active in the **same/later second** as the remote trigger could still win")
**Category**: Enhancement to existing topic
**Affects**: "Out of Scope" section (second bullet, line 99); secondarily the "same-second residual edge" mention in "Edge Contracts to Pin" (line 73)

**Details**:
The investigation twice describes the acknowledged residual edge as a local client active in the **same OR later** second as the remote trigger. The spec narrows this to only "the same `client_activity` second." The "later second" case is real: detection runs a moment after the trigger keystroke, so a local client that types in a *later* second — after the remote trigger bumped its activity but before the detection read enumerates clients — could still end up as the most-active winner. The spec's narrower "same-second-only" framing understates the window in which the documented residual edge can occur.

The resolution is unchanged either way (explicitly out of scope, no workaround built), so this has no effect on what is implemented or tested. It is a documentation-accuracy point: the spec's behavioral contract describes the not-fixed edge more narrowly than the source establishes, so an implementer auditing "when could a local terminal incorrectly win?" would get an incomplete answer. Low impact; flagged so the team can consciously confirm or skip.

**Current**:
> - **The same-epoch-second residual edge** — if a person were actively typing on the local terminal in the same `client_activity` second the remote triggers, the local could tie/win. Explicitly ruled a non-issue: two people interacting with one mirrored session simultaneously is inherently ambiguous and not Portal's to arbitrate. **No workaround will be built for it.** The deterministic first-listed tie-break is the only rule applied.

**Proposed Addition**:
{leave blank until discussed — likely widen "the same `client_activity` second" to "the same or a later `client_activity` second (between the remote trigger and the detection read)" in the Out of Scope bullet, and align the "same-second residual edge" phrasing in Edge Contracts to Pin.}

**Resolution**: Approved
**Notes**: Auto-approved. Widened the Out of Scope bullet to "same or a later `client_activity` second than the remote trigger" and aligned the Edge Contracts phrasing to "same-or-later-second residual edge".

---
