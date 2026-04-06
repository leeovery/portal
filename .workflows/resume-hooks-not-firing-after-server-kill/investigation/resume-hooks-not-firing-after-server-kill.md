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
3. Open Ghostty terminal and type `x` (Portal alias) — not inside any tmux session
4. Portal shows "starting tmux server" loading page, then lands on projects page — no sessions restored
5. Running `tmux list-sessions` from another terminal shows "no server running"

**Reproducibility:** Confirmed on at least one system

**Critical finding:** After Portal's bootstrap, the tmux server is not running at all. The server either fails to start properly or starts and immediately dies. This means tmux-resurrect/continuum never get a chance to restore sessions.

### Environment

- **Affected environments:** Local (tested on a separate MacBook Pro, not the dev machine)
- **Platform:** macOS (Ghostty terminal)
- **tmux plugins confirmed:** tmux-resurrect + tmux-continuum installed with `@continuum-restore 'on'` and `@resurrect-capture-pane-contents 'on'`
- **No error output** from Portal during startup

### Impact

- **Severity:** High
- **Scope:** All users relying on resume hooks after server restart
- **Business impact:** Core workflow disruption — Claude sessions lost on tmux restart

### References

- Previous bugfix: `resume-hooks-lost-on-server-restart` (recent commits on main)

---

## Analysis

### Initial Hypotheses

- ~~tmux-resurrect may not be installed on the test machine~~ — CONFIRMED installed with auto-restore on
- Portal's EnsureServer / server bootstrap may start the server in a way that doesn't persist (e.g., server starts, Portal's TUI runs, server dies when Portal exits)
- The tmux server may start but without loading tmux.conf / TPM plugins, so resurrect never triggers
- Portal may be using `tmux new-session` in a way that creates and immediately destroys the server

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
