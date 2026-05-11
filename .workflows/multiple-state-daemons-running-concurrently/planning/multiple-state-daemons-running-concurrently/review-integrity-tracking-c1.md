---
status: in-progress
created: 2026-05-11
cycle: 1
phase: Plan Integrity Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Integrity

## Findings

### 1. Task 1.1 omits explicit "no t.Parallel()" acceptance criterion

**Severity**: Minor
**Plan Reference**: Phase 1, Task 1.1 (`multiple-state-daemons-running-concurrently-1-1`)
**Category**: Task Template Compliance / Acceptance Criteria Quality
**Change Type**: add-to-task

**Details**:
Every other task in the plan (1.2, 1.3, 1.4, 2.1, 2.2, 2.3) carries an explicit `Tests do not use t.Parallel()` (or equivalent wording) acceptance criterion. Task 1.1 is the only task that omits it. The Phase 1 acceptance block lists "Tests do not use `t.Parallel()`" as a phase-level requirement, but the per-task acceptance criteria are the gate an implementer runs through when ticking off work — leaving it off Task 1.1's checklist creates an inconsistency that an implementer could miss. This is the task that introduces brand-new test files (`internal/state/daemon_lock_test.go`), so the convention reminder lands precisely where the test files are authored.

**Current**:

```markdown
**Acceptance Criteria**:
- [ ] `AcquireDaemonLock(stateDir)` exists in package `state`, accepts a state directory parameter (no hardcoded path).
- [ ] Lock file path resolves to `<stateDir>/daemon.lock`; `DaemonLock(dir)` accessor exposed alongside `DaemonPID` / `DaemonVersion`.
- [ ] Lock file created with mode `0600`.
- [ ] Helper does NOT create `<stateDir>` (no `MkdirAll`); state-dir existence is caller's responsibility.
- [ ] On `unix.Flock` returning `EWOULDBLOCK` the helper returns the exported sentinel `ErrDaemonLockHeld`, distinguishable via `errors.Is`.
- [ ] On `open(2)` errors (`ENOENT`, `EACCES`, `ENOSPC`, `EMFILE`, `ENFILE`) the helper returns a wrapped error that does NOT match `ErrDaemonLockHeld`.
- [ ] On success the returned fd has `FD_CLOEXEC` set (verifiable via `unix.FcntlInt(fd, F_GETFD, 0) & FD_CLOEXEC != 0`).
- [ ] `unix.Flock` call is dispatched via the package-level `lockAcquire` seam so tests inject a fake.
- [ ] No new test pattern departures — seam style matches existing `daemonRunFunc` / `BootstrapAliveCheck`.
- [ ] Daemon-side FIFO-sweep paths reviewed and confirmed read-only — there is no daemon-side write path into the FIFO surface that two concurrent daemons could race on. `FIFOSweeper` is single-shot per process during bootstrap; daemon-side FIFO interaction is read-only. Confirmation recorded as a code-trace assertion in the task's implementation notes / commit message, not as a runtime test (matching spec § "Potentially affected" which framed this as a confirmation requirement, not a verification requirement).
```

**Proposed**:

```markdown
**Acceptance Criteria**:
- [ ] `AcquireDaemonLock(stateDir)` exists in package `state`, accepts a state directory parameter (no hardcoded path).
- [ ] Lock file path resolves to `<stateDir>/daemon.lock`; `DaemonLock(dir)` accessor exposed alongside `DaemonPID` / `DaemonVersion`.
- [ ] Lock file created with mode `0600`.
- [ ] Helper does NOT create `<stateDir>` (no `MkdirAll`); state-dir existence is caller's responsibility.
- [ ] On `unix.Flock` returning `EWOULDBLOCK` the helper returns the exported sentinel `ErrDaemonLockHeld`, distinguishable via `errors.Is`.
- [ ] On `open(2)` errors (`ENOENT`, `EACCES`, `ENOSPC`, `EMFILE`, `ENFILE`) the helper returns a wrapped error that does NOT match `ErrDaemonLockHeld`.
- [ ] On success the returned fd has `FD_CLOEXEC` set (verifiable via `unix.FcntlInt(fd, F_GETFD, 0) & FD_CLOEXEC != 0`).
- [ ] `unix.Flock` call is dispatched via the package-level `lockAcquire` seam so tests inject a fake.
- [ ] No new test pattern departures — seam style matches existing `daemonRunFunc` / `BootstrapAliveCheck`.
- [ ] Daemon-side FIFO-sweep paths reviewed and confirmed read-only — there is no daemon-side write path into the FIFO surface that two concurrent daemons could race on. `FIFOSweeper` is single-shot per process during bootstrap; daemon-side FIFO interaction is read-only. Confirmation recorded as a code-trace assertion in the task's implementation notes / commit message, not as a runtime test (matching spec § "Potentially affected" which framed this as a confirmation requirement, not a verification requirement).
- [ ] Tests do not use `t.Parallel()`.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 2.1 leaves WARN-emission mechanism under-specified

**Severity**: Minor
**Plan Reference**: Phase 2, Task 2.1 (`multiple-state-daemons-running-concurrently-2-1`) — Do section, third bullet
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Task 2.1's Do section describes the WARN-emission seam as: *"Declare a logger seam `var killBarrierLogger *state.Logger` initialised to a no-op default (or accept a `*state.Logger` parameter — choose whichever matches existing tmux-package conventions; if no logger is wired into tmux package yet, prefer a package-level `var killBarrierLogWarn = func(format string, args ...any) {}` that production wiring (Task 2-2 call sites) can replace with a real WARN emitter). The exact emission mechanism is a planning detail — the load-bearing requirement is that the timeout produces **exactly one** WARN-level event observable by tests via a recorder seam."*

This leaves an implementer with three live options (logger pointer var, logger parameter, function var) and the deciding criterion ("matches existing tmux-package conventions") is itself unclear — the tmux package does already have a precedent (`MigrationLogger` interface seam in `internal/tmux/hooks_register.go:137`), but the task does not name it. The acceptance criteria are similarly soft: *"A WARN log emission seam exists (logger interface, package-level `var`, or equivalent)..."*. An implementer would have to make a design call rather than execute a defined plan, and Task 2.2 references `killBarrierLogger` indirectly via "the helper owns the WARN line on timeout, the call site is silent" without pinning the production wiring shape either.

Recommendation: pick the `MigrationLogger`-pattern interface seam (precedent already in the package) and name it concretely so Task 2.2's call sites have a deterministic surface to wire against.

**Current**:

```markdown
- Declare a logger seam `var killBarrierLogger *state.Logger` initialised to a no-op default (or accept a `*state.Logger` parameter — choose whichever matches existing tmux-package conventions; if no logger is wired into tmux package yet, prefer a package-level `var killBarrierLogWarn = func(format string, args ...any) {}` that production wiring (Task 2-2 call sites) can replace with a real WARN emitter). The exact emission mechanism is a planning detail — the load-bearing requirement is that the timeout produces **exactly one** WARN-level event observable by tests via a recorder seam.
```

**Proposed**:

```markdown
- Declare a logger seam matching the existing `MigrationLogger` precedent in `internal/tmux/hooks_register.go:137`: define a minimal local interface `BarrierLogger { Warn(component, format string, args ...any) }` and a package-level `var killBarrierLogger BarrierLogger = noopBarrierLogger{}` initialised to a no-op default. Production wiring at the Task 2-2 call sites replaces the no-op via a top-level `init()` (or via the existing bootstrap-adapter path that injects a real `*state.Logger`, which structurally satisfies the interface). Tests install a recording fake via the package var with `t.Cleanup` reset. The load-bearing requirement is that the timeout produces **exactly one** WARN-level event observable by tests through the recorder seam; log content is illustrative per spec §"Acceptance Criteria → Observability".
```

**Resolution**: Pending
**Notes**:
