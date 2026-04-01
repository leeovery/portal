# Investigation: Resume Hooks Lost On Server Restart

## Symptoms

### Problem Description

**Expected behavior:**
Resume hooks registered in `hooks.json` should survive tmux server restarts and fire when Portal reopens sessions.

**Actual behavior:**
`CleanStale()` removes all hooks from `hooks.json` after a server restart because the old pane IDs no longer exist in the new tmux server. Hooks are destroyed before they get a chance to execute.

### Manifestation

- Hooks silently vanish from `hooks.json` after tmux server kill/restart
- Portal reopens with "No sessions available" in the project picker
- Resume hooks never fire — session continuity is broken
- No error messages or warnings — the deletion is silent

### Reproduction Steps

1. Have two or more Claude Code sessions running in tmux panes, each with an on-resume hook registered (e.g., pane `%0`, `%1`)
2. Kill the tmux server (`tmux kill-server`) or reboot
3. Reopen Portal — project picker shows "No sessions available"
4. Inspect `hooks.json` — entries have been removed

**Reproducibility:** Always

### Environment

- **Affected environments:** Local (any system using tmux + Portal)
- **User conditions:** Multiple panes with registered resume hooks
- **First observed:** 2026-04-01, on freshly installed MacBook Pro — first real test of this functionality on new hardware
- **Workaround:** None — hooks are destroyed before they can be used
- **History:** Likely always been this way; never tested against a server restart scenario before

### Impact

- **Severity:** High
- **Scope:** All users relying on resume hooks for session continuity
- **Business impact:** Core workflow broken — resume hooks are the mechanism for restoring Claude Code sessions after restart

### References

- Inbox bug report: `.workflows/.inbox/.archived/bugs/2026-04-01--resume-hooks-lost-on-server-restart.md`

---

## Analysis

### Initial Hypotheses

`ExecuteHooks()` calls `store.CleanStale()` unconditionally at the start of execution. After a server restart, the new tmux server has no knowledge of old pane IDs — they're ephemeral identifiers that reset. `CleanStale()` sees all old entries as stale and deletes them.

### Code Trace

*To be filled during code analysis.*

### Root Cause

*To be determined.*

### Contributing Factors

*To be determined.*

### Why It Wasn't Caught

*To be determined.*

### Blast Radius

*To be determined.*

---

## Fix Direction

### Chosen Approach

*To be determined after findings review.*

### Options Explored

*To be determined.*

### Discussion

*To be captured during findings review.*

### Testing Recommendations

*To be determined.*

### Risk Assessment

*To be determined.*

---

## Notes

*Additional observations during investigation.*
