---
status: in-progress
created: 2026-03-27
cycle: 2
phase: Gap Analysis
topic: resume-sessions-after-reboot
---

# Review Tracking: resume-sessions-after-reboot - Gap Analysis

## Findings

### 1. Hook execution scope (target session only vs all sessions) is not explicitly stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics, Registry Model & Storage

**Details**:
The spec says "when Portal needs to act on a session's processes, it queries tmux for that session's panes and cross-references the registry" and "Restart commands fire when the user connects to a session via Portal." These phrases heavily imply that hook execution is scoped to the target session's panes only. However, the spec never explicitly states this constraint.

An implementer could reasonably interpret the execution trigger as: (a) execute hooks only for the panes belonging to the session being connected to, or (b) execute all pending hooks across all sessions whenever the user connects to any session via Portal.

After a reboot with 5 sessions restored, option (a) means hooks fire incrementally as the user opens each session. Option (b) means all hooks fire the first time the user opens any session. These produce very different user experiences and have different implications for error handling and timing.

The per-session reading is strongly implied but should be stated explicitly to prevent misinterpretation.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. send-keys failure handling during sequential multi-pane execution

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics

**Details**:
The spec states "Portal executes them sequentially (fire-and-forget via send-keys -- no waiting for completion)." It does not specify behavior when `send-keys` itself fails for a pane (e.g., tmux returns an error). With multiple panes, should Portal abort remaining hooks or continue to the next pane?

Given the fire-and-forget design, the answer is almost certainly "continue and ignore errors" -- but an implementer would need to make this decision. The `send-keys` command could fail if a pane ID exists in the registry but the pane is in an unexpected state, or if stale cleanup missed it due to a race condition. The spec should state whether errors are silently ignored to maintain the fire-and-forget contract.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
