---
status: complete
created: 2026-05-11
cycle: 2
phase: Plan Integrity Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Integrity

## Findings

### 1. Task 2-2 inherits production logger wiring from Task 2-1 but does not action it

**Severity**: Important
**Plan Reference**: Phase 2, Task 2-2 (`multiple-state-daemons-running-concurrently-2-2`) — Do section, Acceptance Criteria
**Category**: Task Self-Containment / Acceptance Criteria Quality / Task Template Compliance
**Change Type**: update-task

**Details**:
The cycle 1 fix to Task 2-1 introduced a `BarrierLogger` interface plus a `var killBarrierLogger BarrierLogger = noopBarrierLogger{}` no-op default, and explicitly hands off the production-wiring step to Task 2-2:

> Production wiring at the Task 2-2 call sites replaces the no-op via a top-level `init()` (or via the existing bootstrap-adapter path that injects a real `*state.Logger`, which structurally satisfies the interface).

But Task 2-2's Do section, Acceptance Criteria, and Tests make no reference to replacing the no-op default. The seven Do bullets cover call-site rewriting and the `killSaverAndWaitForDaemonFn` seam; the nine acceptance criteria cover routing assertions and steady-state path verification; the eight tests cover invocation recorders. None of them installs a real logger into `killBarrierLogger`.

Result: an implementer working Task 2-2 in isolation lands the call-site rewrite, the helper executes on production, the timeout path triggers, and the WARN line is swallowed by the no-op default — silently violating spec §"Acceptance Criteria → Observability" ("barrier timeout produces exactly one WARN-level log line"). The implementer would have to cross-reference Task 2-1's prose to discover the dangling instruction. This is the same self-containment failure cycle 1 fixed for the logger seam shape — the wiring step itself slipped through.

Additionally, the `state` package's `*Logger` type is what `internal/bootstrapadapter/adapters.go` already constructs and passes into `tmux.RegisterPortalHooks`. The cleanest production-wiring shape is to add a top-level `init()` (or, preferably, an exported `SetBarrierLogger(BarrierLogger)` setter that `bootstrapadapter` calls during initialisation, mirroring the parameter-injection precedent at `internal/bootstrapadapter/adapters.go:69`). Either approach is acceptable; the plan should pin one so the implementer does not have to design.

**Current**:

```markdown
**Do**:
- In `internal/tmux/portal_saver.go`, modify `EnsurePortalSaverVersion` at the mismatch branch (currently line 108-112):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The helper itself issues the kill, so the surrounding `if portalSaverVersionMismatch(...) && c.HasSession(PortalSaverName)` guard is preserved unchanged — the barrier is only entered when both the mismatch and presence conditions hold (steady-state path with matching version OR absent session does not enter).
  - The error return is intentionally ignored: the helper returns nil even on timeout per Task 2-1, and the spec mandates the caller proceeds to `BootstrapPortalSaver` regardless.
- In `internal/tmux/portal_saver.go`, modify `BootstrapPortalSaver`'s stale-daemon branch (currently line 66-70):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The surrounding `if sessionPresent && !BootstrapAliveCheck(stateDir)` guard is preserved unchanged.
  - `stateDir` is already a parameter to `BootstrapPortalSaver` (line 63), so no signature change required.
- Introduce a test-only injection recorder via a package-level `var` (or replace the existing `killBarrierReadPID` test seam with a recording wrapper) that lets tests assert **whether** the barrier helper was invoked on each call-site path. Implementation choice: expose `var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon` at package scope and route both call sites through this `var` so tests can swap in a recorder. This is a minimal seam — the function body itself was already seamed inside Task 2-1.
- Verify by code review that there are exactly **two** call sites to the barrier helper in `internal/tmux/portal_saver.go` after the change (one in `EnsurePortalSaverVersion`, one in `BootstrapPortalSaver`); no other production call sites should exist.
- Do not add new logging at either call site — the helper owns the WARN line on timeout, the call site is silent.
- Steady-state path verification: when `portalSaverVersionMismatch` returns false (stored version matches current and is non-empty/non-dev), `EnsurePortalSaverVersion` returns without invoking the helper. When `BootstrapAliveCheck(stateDir)` returns true (live daemon), `BootstrapPortalSaver` skips the stale-daemon branch and goes straight to set-option, never invoking the helper.
```

**Proposed**:

```markdown
**Do**:
- In `internal/tmux/portal_saver.go`, modify `EnsurePortalSaverVersion` at the mismatch branch (currently line 108-112):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemonFn(c, stateDir)` (routed through the recorder seam introduced below).
  - The helper itself issues the kill, so the surrounding `if portalSaverVersionMismatch(...) && c.HasSession(PortalSaverName)` guard is preserved unchanged — the barrier is only entered when both the mismatch and presence conditions hold (steady-state path with matching version OR absent session does not enter).
  - The error return is intentionally ignored: the helper returns nil even on timeout per Task 2-1, and the spec mandates the caller proceeds to `BootstrapPortalSaver` regardless.
- In `internal/tmux/portal_saver.go`, modify `BootstrapPortalSaver`'s stale-daemon branch (currently line 66-70):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemonFn(c, stateDir)`.
  - The surrounding `if sessionPresent && !BootstrapAliveCheck(stateDir)` guard is preserved unchanged.
  - `stateDir` is already a parameter to `BootstrapPortalSaver` (line 63), so no signature change required.
- Introduce a test-only injection recorder via a package-level `var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon` at package scope and route both call sites through this `var` so tests can swap in a recorder. This is a minimal seam — the function body itself was already seamed inside Task 2-1.
- **Wire the production logger into `killBarrierLogger`** (handed off from Task 2-1): expose an exported setter `SetBarrierLogger(l BarrierLogger)` in `internal/tmux/portal_saver.go` that assigns `killBarrierLogger = l` when `l != nil`, mirroring the parameter-injection precedent at `internal/bootstrapadapter/adapters.go:69` (`RegisterPortalHooks(r.Client, r.Logger)`). Then add a call to `tmux.SetBarrierLogger(r.Logger)` from `internal/bootstrapadapter/adapters.go` (the same site that already constructs the `*state.Logger` for `RegisterPortalHooks`) so the no-op default is replaced exactly once at adapter wiring time, before `BootstrapPortalSaver` or `EnsurePortalSaverVersion` is first invoked. The `*state.Logger` type structurally satisfies `BarrierLogger { Warn(component, format string, args ...any) }` — no adapter shim required.
- Verify by code review that there are exactly **two** call sites to the barrier helper in `internal/tmux/portal_saver.go` after the change (one in `EnsurePortalSaverVersion`, one in `BootstrapPortalSaver`); no other production call sites should exist.
- Do not add new logging at either call site — the helper owns the WARN line on timeout, the call site is silent.
- Steady-state path verification: when `portalSaverVersionMismatch` returns false (stored version matches current and is non-empty/non-dev), `EnsurePortalSaverVersion` returns without invoking the helper. When `BootstrapAliveCheck(stateDir)` returns true (live daemon), `BootstrapPortalSaver` skips the stale-daemon branch and goes straight to set-option, never invoking the helper.
```

Additionally, append the following acceptance criterion (insert between the existing "When `sessionPresent` is false..." bullet and "All existing tests in `internal/tmux/portal_saver_test.go`..." bullet so the production-wiring property is gated alongside the routing properties):

```markdown
- [ ] `tmux.SetBarrierLogger` is exported and called from `internal/bootstrapadapter/adapters.go` so `killBarrierLogger` is replaced with the production `*state.Logger` at adapter wiring time; the no-op default does not persist into production. Verified by a unit test in `internal/tmux/portal_saver_test.go` that asserts the WARN-on-timeout reaches a recording logger installed via `SetBarrierLogger` (mirrors the existing `t.Cleanup` reset pattern).
```

And append the following test to the Tests block (before the closing item):

```markdown
- `"it routes WARN-on-timeout through the logger installed via SetBarrierLogger"` — install a recording `BarrierLogger` via `tmux.SetBarrierLogger(recorder)` with `t.Cleanup` resetting to the no-op default; seed the helper for the timeout path (PID never dies, shrunk clock); invoke `EnsurePortalSaverVersion` against a version-mismatch scenario; assert the recorder captured exactly one WARN entry under `ComponentBootstrap`. Guards against the no-op default persisting into production.
```

**Resolution**: Fixed
**Notes**:

---

### 2. Task heading style is inconsistent between Phase 1 and Phase 2

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` task headings (lines 9, 89, 166)
**Category**: Task Template Compliance / consistency
**Change Type**: update-task

**Details**:
Phase 1 task headings use the dot separator (`### Task 1.1:`, `### Task 1.2:`, `### Task 1.3:`, `### Task 1.4:`), and all cross-references inside Phase 1 prose follow the same form (e.g., "Task 1.1", "Task 1.2", "Task 1.4"). Phase 2 task headings use the hyphen separator (`### Task 2-1:`, `### Task 2-2:`, `### Task 2-3:`), even though Phase 1 prose at line 192 still says "the Phase 2 integration test." The hyphen form mirrors the internal IDs (`multiple-state-daemons-running-concurrently-2-1`), but the visible "Task X.Y" form is the human-facing convention established in Phase 1.

Impact is low — both forms unambiguously identify their tasks — but the inconsistency makes cross-phase scanning slightly harder and would propagate into future phases if not corrected. Aligning Phase 2 headings to the `Task 2.1` / `Task 2.2` / `Task 2.3` form (and updating Phase 2 prose self-references at lines 9, 27, 31, 49, 73, 89, 92, 95, 106, 137, 156, 166, 215 accordingly) restores plan-wide consistency.

**Current**:

```markdown
### Task 2-1: Add seam-injectable killSaverAndWaitForDaemon helper
```

```markdown
### Task 2-2: Wire barrier into both kill call sites (EnsurePortalSaverVersion + BootstrapPortalSaver)
```

```markdown
### Task 2-3: Real-tmux integration test asserts singleton invariant after recycle
```

**Proposed**:

```markdown
### Task 2.1: Add seam-injectable killSaverAndWaitForDaemon helper
```

```markdown
### Task 2.2: Wire barrier into both kill call sites (EnsurePortalSaverVersion + BootstrapPortalSaver)
```

```markdown
### Task 2.3: Real-tmux integration test asserts singleton invariant after recycle
```

In addition, replace every in-prose self-reference inside `phase-2-tasks.md` of the form `Task 2-1`, `Task 2-2`, `Task 2-3` with `Task 2.1`, `Task 2.2`, `Task 2.3` respectively (the internal-ID strings of the form `multiple-state-daemons-running-concurrently-2-1` are unchanged — only the human-facing display names are aligned).

**Resolution**: Fixed
**Notes**:
