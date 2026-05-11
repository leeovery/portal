---
status: in-progress
created: 2026-05-11
cycle: 4
phase: Plan Integrity Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Integrity

## Findings

### 1. Task 2.2 Solution paragraph still references the pre-Fn helper symbol

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` Task 2.2, Solution (line 93)
**Category**: Task Template Compliance / Task Self-Containment / consistency
**Change Type**: update-task

**Details**:
Cycle 3 finding 2 corrected the two Do bullets at lines 99 and 103 from `killSaverAndWaitForDaemon(c, stateDir)` to `killSaverAndWaitForDaemonFn(c, stateDir)` so they match the recorder seam introduced at line 106 (`var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon`) and the acceptance criteria/tests further down which all reference `killSaverAndWaitForDaemonFn`.

One reference outside the Do section was missed: the Solution paragraph at line 93 still describes the replacement as `_ = killSaverAndWaitForDaemon(c, stateDir)` — the non-`Fn` form. The Solution is the high-level statement of what the task does, read top-down before the Do bullets, so an implementer skimming Solution-only would form the wrong mental model of which symbol the call sites bind to.

This is exactly the same self-containment defect cycle 3 finding 2 closed for the Do bullets — purely a missed location during that pass. Impact is cosmetic; an implementer who reads the full task body will reconcile against the Do bullet at line 106 ("Both call-site replacements above use `killSaverAndWaitForDaemonFn(c, stateDir)`, not the helper directly"). Bringing the Solution into alignment removes the remaining inconsistency and makes the task fully self-consistent on a single read.

The acceptance criteria (line 113), Edge Cases prose, and Tests (lines 128–135) all already use the `Fn` form, so this is the last stale-symbol occurrence in Task 2.2.

**Current**:

```markdown
**Solution**: Replace the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:111` (inside `EnsurePortalSaverVersion`'s mismatch branch) and the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:68` (inside `BootstrapPortalSaver`'s stale-daemon branch) with `_ = killSaverAndWaitForDaemon(c, stateDir)`. Both call sites now route through the same synchronisation helper. `BootstrapPortalSaver`'s signature already accepts `stateDir` (line 63), and `EnsurePortalSaverVersion` already has `stateDir` in scope (line 106), so no signature changes are required. Verify by injection-recorder unit tests that **both** sites invoke the helper, and that the steady-state path (saver alive, version matches) does **not** invoke it.
```

**Proposed**:

```markdown
**Solution**: Replace the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:111` (inside `EnsurePortalSaverVersion`'s mismatch branch) and the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:68` (inside `BootstrapPortalSaver`'s stale-daemon branch) with `_ = killSaverAndWaitForDaemonFn(c, stateDir)` — routed through the test-only injection recorder seam `var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon` introduced in the Do section below. Both call sites now route through the same synchronisation helper. `BootstrapPortalSaver`'s signature already accepts `stateDir` (line 63), and `EnsurePortalSaverVersion` already has `stateDir` in scope (line 106), so no signature changes are required. Verify by injection-recorder unit tests that **both** sites invoke the helper, and that the steady-state path (saver alive, version matches) does **not** invoke it.
```

**Resolution**: Pending
**Notes**:
