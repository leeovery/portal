---
status: in-progress
created: 2026-07-21
cycle: 1
phase: Input Review
topic: spawned-window-dead-ends-on-session-exit
---

# Review Tracking: spawned-window-dead-ends-on-session-exit - Input Review

## Findings

### 1. The `[detached (from session <name>)]` line is tmux output and persists after the fix

**Source**: investigation — Code Trace step 4 (lines 165-177: "The `[detached …]` line above it is confirmed tmux client-detach output") and Symptoms/Manifestation (lines 27-29)
**Category**: Enhancement to existing topic
**Affects**: Testing Requirements (Manual validation) / Acceptance Criteria (criterion 1)

**Details**:
The investigation deliberately distinguishes the two lines in the reported dead-end block:
- `[detached (from session <name>)]` — confirmed to be tmux's own client-detach output.
- `Process exited. Press any key to close the terminal.` — Ghostty's `wait after command` end-of-command prompt (the actual dead-end).

The fix removes only the second line. On the **detach** path (as opposed to a clean session end), the `[detached (from session <name>)]` line is normal tmux output and will still print — with the fallback shell prompt appearing beneath it. The specification renders the full two-line block in "Observed Bug" and its acceptance/validation criteria describe the post-fix landing as "the user's normal interactive login shell prompt … not the 'Process exited…' dead-end," without noting that the tmux `[detached]` line remains above that prompt on the detach path. A validator running the documented manual test on the detach path could see `[detached …]` and mistake it for an incomplete fix. Calling out that this line is tmux's normal output (unchanged by the fix) removes that ambiguity.

**Current**:
(Acceptance Criteria, criterion 1)
"1. When a session running inside a burst-spawned (N−1 external) native-Ghostty window exits or detaches, the window lands at the user's normal interactive login shell prompt (`$SHELL`, login + interactive) — not the "Process exited. Press any key to close the terminal." dead-end."

(Testing Requirements, Manual validation)
"…open a Ghostty window via the adapter's command shape, kill/detach the session, and confirm the window lands at the user's normal interactive login shell (`$SHELL`, login+interactive) rather than a "Press any key to close" dead-end."

**Proposed Addition**:
(leave blank until discussed — likely a one-clause note that the tmux `[detached (from session <name>)]` line still prints above the fallback prompt on the detach path and is expected, since it is tmux's own client-detach output and outside the fix's scope)

**Resolution**: Pending
**Notes**:

---
