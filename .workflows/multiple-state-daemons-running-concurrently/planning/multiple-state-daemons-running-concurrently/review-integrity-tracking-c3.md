---
status: complete
created: 2026-05-11
cycle: 3
phase: Plan Integrity Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Integrity

## Findings

### 1. Stale hyphen-form task references survived cycle 2's dot-form conversion

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` Task 2.3, Do step 10 (line 190) and Edge Cases bullet 3 (line 223)
**Category**: Task Template Compliance / consistency
**Change Type**: update-task

**Details**:
Cycle 2 finding 2 aligned every Phase 2 task heading and self-reference from the hyphen form (`Task 2-1`/`2-2`/`2-3`) to the dot form (`Task 2.1`/`2.2`/`2.3`) so the plan reads consistently with Phase 1's convention. Two in-prose references inside Task 2.3 were missed: both render as the mixed form `Task 2.1/2-2`, where the first index uses a dot and the second a hyphen.

Impact is purely cosmetic — an implementer can still resolve which tasks are being referenced — but the inconsistency is the same kind of small irritant cycle 2 finding 2 set out to remove. Fixing both occurrences brings Phase 2 to full dot-form alignment.

**Current**:

```markdown
  10. Second invocation: `if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil { t.Fatalf(...) }`. This must trigger the recycle: kill the old saver, run the barrier (Task 2.1/2-2), wait for the prior daemon to exit, then create a fresh saver.
```

```markdown
- The first daemon's tick is mid-flight when the kill arrives → the barrier (Task 2.1/2-2) waits for it; the integration test allows up to a 3 s settle window after the second `EnsurePortalSaverVersion` returns to absorb this without flakiness.
```

**Proposed**:

```markdown
  10. Second invocation: `if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil { t.Fatalf(...) }`. This must trigger the recycle: kill the old saver, run the barrier (Task 2.1/2.2), wait for the prior daemon to exit, then create a fresh saver.
```

```markdown
- The first daemon's tick is mid-flight when the kill arrives → the barrier (Task 2.1/2.2) waits for it; the integration test allows up to a 3 s settle window after the second `EnsurePortalSaverVersion` returns to absorb this without flakiness.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task 2.2 Do bullets describe the wrong symbol for the call-site replacement

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` Task 2.2, Do section (lines 99 and 103)
**Category**: Task Template Compliance / Task Self-Containment / consistency

**Change Type**: update-task

**Details**:
Cycle 2 finding 1 introduced the test-only injection recorder seam `var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon` and required both call sites to route through `killSaverAndWaitForDaemonFn`, not the helper directly. The bullet that introduces the seam (line 106) ends with: *"Both call-site replacements above use `killSaverAndWaitForDaemonFn(c, stateDir)`, not the helper directly."*

But the two Do bullets that describe the call-site replacement (lines 99 and 103) still say:

```markdown
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
```

```markdown
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
```

The function name in those two bullets is missing the `Fn` suffix. An implementer reading top-down would apply the replacement as written first, then discover at the seam-introduction bullet that the call sites need to use the `Fn` form instead, and have to re-edit. This is the same self-containment issue cycle 2 finding 1's Proposed intended to close (its proposed text wrote `killSaverAndWaitForDaemonFn` on both bullets), but the merged plan retained the older non-`Fn` form on lines 99 and 103.

Aligning both bullets to the `Fn` form removes the contradiction and matches the rest of the task (acceptance criteria at line 113 already reference `killSaverAndWaitForDaemonFn`, and tests at lines 128–135 all stub `killSaverAndWaitForDaemonFn`).

**Current**:

```markdown
- In `internal/tmux/portal_saver.go`, modify `EnsurePortalSaverVersion` at the mismatch branch (currently line 108-112):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The helper itself issues the kill, so the surrounding `if portalSaverVersionMismatch(...) && c.HasSession(PortalSaverName)` guard is preserved unchanged — the barrier is only entered when both the mismatch and presence conditions hold (steady-state path with matching version OR absent session does not enter).
  - The error return is intentionally ignored: the helper returns nil even on timeout per Task 2.1, and the spec mandates the caller proceeds to `BootstrapPortalSaver` regardless.
- In `internal/tmux/portal_saver.go`, modify `BootstrapPortalSaver`'s stale-daemon branch (currently line 66-70):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The surrounding `if sessionPresent && !BootstrapAliveCheck(stateDir)` guard is preserved unchanged.
  - `stateDir` is already a parameter to `BootstrapPortalSaver` (line 63), so no signature change required.
```

**Proposed**:

```markdown
- In `internal/tmux/portal_saver.go`, modify `EnsurePortalSaverVersion` at the mismatch branch (currently line 108-112):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemonFn(c, stateDir)` (routed through the recorder seam introduced below).
  - The helper itself issues the kill, so the surrounding `if portalSaverVersionMismatch(...) && c.HasSession(PortalSaverName)` guard is preserved unchanged — the barrier is only entered when both the mismatch and presence conditions hold (steady-state path with matching version OR absent session does not enter).
  - The error return is intentionally ignored: the helper returns nil even on timeout per Task 2.1, and the spec mandates the caller proceeds to `BootstrapPortalSaver` regardless.
- In `internal/tmux/portal_saver.go`, modify `BootstrapPortalSaver`'s stale-daemon branch (currently line 66-70):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemonFn(c, stateDir)` (routed through the recorder seam introduced below).
  - The surrounding `if sessionPresent && !BootstrapAliveCheck(stateDir)` guard is preserved unchanged.
  - `stateDir` is already a parameter to `BootstrapPortalSaver` (line 63), so no signature change required.
```

**Resolution**: Fixed
**Notes**:
