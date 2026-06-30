# Investigation: Session Rename Orphans Resume Hook

## Symptoms

### Problem Description

**Expected behavior:**
After renaming a tmux session with `tmux rename-session`, the session's Portal
resume hook should remain intact so that on the next reboot the session resumes
what was running (not a bare shell).

**Actual behavior:**
Renaming a tmux session silently orphans its Portal resume hook. The session
keeps running fine, but after the next restart it comes back as a bare shell
instead of resuming.

### Manifestation

- No error, no warning at rename time — the failure is silent.
- The session keeps running normally; nothing looks wrong until a reboot.
- On reboot the renamed session resurrects as a bare shell instead of resuming
  the inner process.
- Surfaces as a "didn't resume" case in partial-restore reports (e.g. 28 of 32
  sessions resuming).

### Reproduction Steps

1. Create a Portal session (generated name `{project}-{nanoid}`) with a resume
   hook registered (e.g. `portal hooks set --on-resume "..."`).
2. Rename the session with `tmux rename-session <new-name>` (the inner process
   keeps running — it is **not** restarted).
3. Wait for the next bootstrap / orphan cleanup pass (or trigger one).
4. Reboot / restart the tmux server.
5. Observe: the renamed session comes back as a bare shell — hook gone.

**Reproducibility:** Always (per the seed's clean correlation).

### Environment

- **Affected environments:** Local (Portal's normal runtime).
- **Browser/platform:** macOS, tmux 3.6b.
- **User conditions:** Any session that has been renamed away from its generated
  nanoid name while carrying a resume hook.

### Impact

- **Severity:** Medium–High (silent data loss of resume config; affects an
  encouraged workflow).
- **Scope:** Any renamed session. Live evidence 2026-06-30: of 24 live sessions,
  exactly the two that had been renamed lost their hooks.
- **Business impact:** Trust — reboot resume silently drops work the user
  expects to come back.

### References

- Seed: `.workflows/session-rename-orphans-resume-hook/seeds/2026-06-30-session-rename-orphans-resume-hook.md`
- Discovery: `.workflows/session-rename-orphans-resume-hook/discovery/session-001.md`

---

## Analysis

### Initial Hypotheses

Suspected mechanism from discovery (to be **validated**, not assumed):

Resume hooks in `hooks.json` are keyed by the structural key
`session_name:window.pane` (`StructuralKeyFormat` in `internal/tmux/tmux.go`).
After a rename:

1. The session name changes but the inner pane process does not restart.
2. The stored hook key no longer matches the live pane's structural key.
3. Portal's stale-hook cleanup deletes the now-unmatched key (no live pane
   corresponds to it).
4. Because the inner process never restarted, nothing re-registers a hook under
   the new name.

Net: the hook is silently gone. Live correlation (2026-06-30): the only two
sessions of 24 missing hooks were the two renamed away from their generated
nanoid names; underlying history files still exist, so the prune-on-missing-
history path is **not** involved.

### Code Trace

_(to be filled during Step 5 — Code Analysis)_

### Root Cause

_(to be filled during Step 6)_

### Contributing Factors

_(to be filled)_

### Why It Wasn't Caught

_(to be filled)_

### Blast Radius

_(to be filled)_

---

## Fix Direction

### Chosen Approach

_(to be filled during Step 8 — Findings Review)_

### Options Explored

Discovery flagged a candidate direction: a stable, rename-immune session
identity (the keying is on the mutable session name). Design choice deferred to
the specification phase.

### Discussion

_(to be filled)_

### Testing Recommendations

_(to be filled)_

### Risk Assessment

_(to be filled)_

---

## Notes

- Distinct from the separate, parked SessionEnd-cleanup question (bare shells
  resurrecting Claude).
