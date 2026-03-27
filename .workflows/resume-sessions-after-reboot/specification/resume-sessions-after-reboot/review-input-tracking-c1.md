---
status: in-progress
created: 2026-03-27
cycle: 1
phase: Input Review
topic: resume-sessions-after-reboot
---

# Review Tracking: resume-sessions-after-reboot - Input Review

## Findings

### 1. Post-execution volatile marker state is unspecified

**Source**: Discussion, "Should Portal detect dead processes or just execute whatever is registered?" (decision section, lines 118-136); Specification, "Execution Mechanics" and "Volatile Marker Mechanism"
**Category**: Gap/Ambiguity
**Affects**: Execution Mechanics, Volatile Marker Mechanism

**Details**:
After a reboot, Portal executes a restart command because the persistent entry exists and the volatile marker is absent. But after execution, Portal does not set the volatile marker itself. This means on the *next* `portal open` for that session, the two-condition check still passes (entry exists, marker still absent), and the command fires again.

For self-registering tools like Claude Code, this is fine — the `SessionStart` hook fires on resume and calls `xctl hooks set`, which sets the volatile marker. But the discussion and research both describe the system as generic ("dev servers, etc."). A non-self-registering tool would have its restart command re-executed on every subsequent `portal open` until the server dies again.

Neither source explicitly discusses this scenario. The discussion's scenario table covers first-attach-after-reboot but not second-attach-after-reboot-with-command-already-executed.

The specification should either: (a) state that Portal sets the volatile marker after executing a restart command, preventing re-execution, or (b) explicitly document that re-execution is expected and tools are responsible for idempotency or re-registration.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:
