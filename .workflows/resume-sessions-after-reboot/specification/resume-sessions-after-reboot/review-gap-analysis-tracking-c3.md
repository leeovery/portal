---
status: complete
created: 2026-03-27
cycle: 3
phase: Gap Analysis
topic: resume-sessions-after-reboot
---

# Review Tracking: resume-sessions-after-reboot - Gap Analysis

## Findings

### 1. Event type flag requirement and behavior when omitted is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: CLI Surface

**Details**:
The spec shows `hooks set --on-resume "cmd"` and `hooks rm --on-resume` and states "Only `--on-resume` implemented initially; surface supports future event types (e.g., `--on-start`, `--on-close`)." However, it does not specify:

1. Whether the event type flag is **required** -- what happens if a user runs `xctl hooks set` or `xctl hooks rm` with no event type flag? Should the command error with a usage message?
2. Whether `hooks rm` without a flag removes **all** hooks for the current pane (a "remove everything" shorthand) or errors.

With only one event type today this has low practical impact, but the spec explicitly designs for future event types, making the flag contract a structural decision for the Cobra command definition. An implementer would need to decide: is `--on-resume` a required flag, and does bare `hooks rm` mean "remove all" or "missing required flag"?

**Proposed Addition**:

**Resolution**: Approved
**Notes**: Event type flag required for set and rm. Added to CLI Surface section.
