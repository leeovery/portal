# Investigation: Resume Hooks Not Firing After Server Kill

## Symptoms

### Problem Description

**Expected behavior:**
After `tmux kill-server`, reopening Portal should resurrect previous sessions (via tmux-resurrect) and fire resume hooks, resuming Claude Code sessions in their respective panes.

**Actual behavior:**
Portal boots tmux but goes straight to the projects page instead of the sessions page — sessions are not resurrected and resume hooks do not fire.

### Manifestation

- No sessions present after server restart — user lands on projects page instead of sessions page
- Resume hooks never execute because there are no sessions to trigger them
- The recent `resume-hooks-lost-on-server-restart` bugfix was supposed to address this scenario

### Reproduction Steps

1. Open Portal, initiate two Claude Code sessions — resume hooks register successfully
2. Run `tmux kill-server` to kill the tmux server
3. Open a new terminal and press `x` (Portal shortcut)
4. Portal boots tmux but shows the projects page — no sessions restored

**Reproducibility:** Confirmed on at least one system

### Environment

- **Affected environments:** Local (tested on a separate MacBook Pro, not the dev machine)
- **Platform:** macOS
- **User conditions:** tmux-resurrect plugin may or may not be installed on the test machine — this is a prerequisite for session restoration

### Impact

- **Severity:** High
- **Scope:** All users relying on resume hooks after server restart
- **Business impact:** Core workflow disruption — Claude sessions lost on tmux restart

### References

- Previous bugfix: `resume-hooks-lost-on-server-restart` (recent commits on main)

---

## Analysis

### Initial Hypotheses

- tmux-resurrect may not be installed on the test machine, which is a prerequisite for session restoration
- The resume hooks implementation may have a bug in the session resurrection or hook execution flow
- Pane ID changes after server restart may not be handled correctly

### Code Trace

_To be filled during code analysis_

### Root Cause

_To be determined_

### Contributing Factors

_To be determined_

### Why It Wasn't Caught

_To be determined_

### Blast Radius

_To be determined_

---

## Fix Direction

_To be determined after analysis_

---

## Notes

- User explicitly asked not to kill tmux server on the dev machine during investigation
- Need to distinguish between "tmux-resurrect not installed" (environment issue) vs "Portal bug" (code issue)
