---
status: in-progress
created: 2026-05-11
cycle: 1
phase: Traceability Review
topic: Multiple State Daemons Running Concurrently
---

# Review Tracking: Multiple State Daemons Running Concurrently - Traceability

## Findings

### 1. FIFO sweep paths confirmation not recorded in plan

**Type**: Missing from plan
**Spec Reference**: § Potentially affected (to confirm during planning) → "FIFO sweep paths" — *"Two daemons could in principle both call into `state` cleanup helpers concurrently. `FIFOSweeper` itself runs only in bootstrap (single-shot per process), so daemon-side FIFO interaction is read-only — investigation assessed this as 'likely safe' but flagged that the fix design should explicitly confirm there is no daemon-side write path that two concurrent daemons could race on."*
**Plan Reference**: N/A (no task or acceptance bullet captures the confirmation)
**Change Type**: add-to-task

**Details**:
The specification explicitly carries forward an item for planning to resolve: *confirm there is no daemon-side write path that two concurrent daemons could race on* in the FIFO sweep surface. The plan ships without recording this confirmation, leaving an open assertion the spec asked planning to close.

Because the fix's correctness argument leans on "the lock catches everything else, including paths we haven't enumerated," explicitly closing this confirmation in the planning record (rather than leaving it as a dangling investigation note) is what the spec asked for. The cheapest place to land it is as an acceptance bullet on Task 1.1 (the lock helper) confirming the daemon-side state surface that the lock now guards, since the confirmation is upstream of any test — it is a code-trace assertion, not a runtime check.

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
```

**Resolution**: Fixed
**Notes**:

---

### 2. `@portal-restoring` marker extension under barrier timeout not surfaced in plan

**Type**: Incomplete coverage
**Spec Reference**: § Fix Part 2 → "Interaction with `@portal-restoring` marker" — *"On the barrier-timeout path, the marker remains set for up to 5 s longer than in current behaviour. This is explicitly acceptable: the marker is designed to bracket the whole bootstrap window, daemon `captureAndCommit` suppression is the intended behaviour while it is set, and a 5 s extension does not affect the correctness of steps 5 (Restore), 6 (EagerSignalHydrate), or downstream warning surfacing."*
**Plan Reference**: Phase 2 → Task 2-1 (barrier helper) / Task 2-2 (call-site wiring)
**Change Type**: add-to-task

**Details**:
The spec explicitly acknowledges that the barrier extends the `@portal-restoring` window by up to 5 seconds on the timeout path, and states this is acceptable because daemon `captureAndCommit` suppression remains in force throughout the bootstrap window. The plan does not surface this property in any task's Context or Edge Cases. A future maintainer reading Phase 2's tasks in isolation would not know that the barrier timeout interacts with the bootstrap marker semantics, and might mistakenly tighten the timeout, narrow the marker lifetime, or add an unrelated "marker dropped while barrier still running" diagnostic.

The cheapest place to record this is in Task 2-2's Edge Cases or Context (the call-site wiring task, since that is where the barrier executes within bootstrap step 4 between marker-set step 3 and marker-clear step 7).

**Current**:

```markdown
**Edge Cases**:
- `EnsurePortalSaverVersion` with mismatch=true but `HasSession` returns false — the existing guard at line 108 (`&& c.HasSession(PortalSaverName)`) means the barrier is **not** invoked when there is no session to kill. This is correct and preserved unchanged (covered by `TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists`).
- Tolerated `KillSession` errors from within the barrier helper — handled by Task 2-1; verified at the call site by the "tolerates a failing kill-session call" test from Task 2-1. The call site does not need additional tolerance because the helper already returns nil on failure.
- Concurrent bootstrap producing a race where one invocation reaches the kill site while another is mid-create — the spec accepts that concurrent bootstraps may produce one WARN on whichever loses the lock (Phase 1's responsibility). The barrier wiring does not introduce new ordering hazards.
- The retry loop in `createPortalSaverWithRetry` (line 138-158) is downstream of the barrier and unchanged — the barrier waits for the prior daemon to exit, then `createPortalSaverWithRetry` handles the create itself. Retry-on-transient-failure semantics are preserved.
```

**Proposed**:

```markdown
**Edge Cases**:
- `EnsurePortalSaverVersion` with mismatch=true but `HasSession` returns false — the existing guard at line 108 (`&& c.HasSession(PortalSaverName)`) means the barrier is **not** invoked when there is no session to kill. This is correct and preserved unchanged (covered by `TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists`).
- Tolerated `KillSession` errors from within the barrier helper — handled by Task 2-1; verified at the call site by the "tolerates a failing kill-session call" test from Task 2-1. The call site does not need additional tolerance because the helper already returns nil on failure.
- Concurrent bootstrap producing a race where one invocation reaches the kill site while another is mid-create — the spec accepts that concurrent bootstraps may produce one WARN on whichever loses the lock (Phase 1's responsibility). The barrier wiring does not introduce new ordering hazards.
- The retry loop in `createPortalSaverWithRetry` (line 138-158) is downstream of the barrier and unchanged — the barrier waits for the prior daemon to exit, then `createPortalSaverWithRetry` handles the create itself. Retry-on-transient-failure semantics are preserved.
- `@portal-restoring` marker lifetime under barrier timeout: bootstrap step 3 sets the marker and step 7 clears it; the barrier (executed inside step 4, EnsureSaver) extends the in-bracket window by up to 5 s on the timeout path. The spec explicitly accepts this — daemon `captureAndCommit` suppression remains in force throughout the bootstrap window, and the 5 s extension does not affect the correctness of steps 5 (Restore), 6 (EagerSignalHydrate), or downstream warning surfacing. **Do not** tighten the timeout, narrow the marker lifetime, or add a diagnostic for "marker still set while barrier running" — all three would regress against an explicitly accepted spec property.
```

**Resolution**: Pending
**Notes**:

---

### 3. CLAUDE.md `state` package documentation note not addressed

**Type**: Missing from plan
**Spec Reference**: § Risk and Rollout → "Documentation" — *"No user-facing documentation changes required. The fix is internal to the saver-bootstrap subsystem. The internal `CLAUDE.md` may want a brief note added to the `state` package row noting the `daemon.lock` invariant — to be evaluated during planning."*
**Plan Reference**: N/A (no task or acceptance bullet covers internal documentation)
**Change Type**: add-to-task

**Details**:
The specification explicitly defers a small documentation decision to planning: whether to add a brief note to the project `CLAUDE.md` `state` package row noting the new `daemon.lock` singleton invariant. The plan ships without recording either a decision to do this or a decision to skip it.

Given that other rows in `CLAUDE.md` (e.g. the `state` row already documents `IsRestoringSet`, `BootstrapPortalSaver` lifecycle, `TailScrollback` contract) consistently surface load-bearing structural invariants, and the lock fits that pattern precisely (it is the floor that holds singleton-ness for future seams), the most coherent resolution is to land the note as part of Task 1.2 (the task that introduces the production wiring of the lock). That keeps doc + behaviour in a single commit and avoids a stray follow-up task.

**Current**:

```markdown
**Acceptance Criteria**:
- [ ] `state.AcquireDaemonLock` is called from `stateDaemonCmd.RunE` after the `os.Remove(state.SaveRequested(dir))` line and before `state.WritePIDFile`.
- [ ] On success, the returned `*os.File` is assigned to a package-level `var daemonLockFile *os.File` so it cannot be GC-collected for the lifetime of the daemon process.
- [ ] No `runtime.SetFinalizer` is added; the default `*os.File` finalizer is suppressed by package-level retention, not by any explicit `SetFinalizer(f, nil)` call (which would be a no-op safeguard but is not required).
- [ ] On `ErrDaemonLockHeld`: exactly one WARN-level log line is emitted under `ComponentDaemon`, `daemon.pid` is NOT written (or, if pre-existing, NOT overwritten), `daemon.version` is NOT written, the tick loop is NOT entered, and `RunE` returns `nil` (cobra exit status 0).
- [ ] On other lock errors (e.g. `fs.ErrNotExist`, `syscall.EACCES`): exactly one ERROR-level log line is emitted, `RunE` returns a non-nil error (cobra exit non-zero).
- [ ] The existing test `TestStateDaemon_WritesPIDFileOnStartup` and friends continue to pass — the lock seam defaults to a real `unix.Flock` that will succeed against a fresh `t.TempDir()`, so non-contention tests are unaffected.
- [ ] Tests use `t.TempDir()` via `PORTAL_STATE_DIR` for isolation; tests do not use `t.Parallel()`.
```

**Proposed**:

```markdown
**Acceptance Criteria**:
- [ ] `state.AcquireDaemonLock` is called from `stateDaemonCmd.RunE` after the `os.Remove(state.SaveRequested(dir))` line and before `state.WritePIDFile`.
- [ ] On success, the returned `*os.File` is assigned to a package-level `var daemonLockFile *os.File` so it cannot be GC-collected for the lifetime of the daemon process.
- [ ] No `runtime.SetFinalizer` is added; the default `*os.File` finalizer is suppressed by package-level retention, not by any explicit `SetFinalizer(f, nil)` call (which would be a no-op safeguard but is not required).
- [ ] On `ErrDaemonLockHeld`: exactly one WARN-level log line is emitted under `ComponentDaemon`, `daemon.pid` is NOT written (or, if pre-existing, NOT overwritten), `daemon.version` is NOT written, the tick loop is NOT entered, and `RunE` returns `nil` (cobra exit status 0).
- [ ] On other lock errors (e.g. `fs.ErrNotExist`, `syscall.EACCES`): exactly one ERROR-level log line is emitted, `RunE` returns a non-nil error (cobra exit non-zero).
- [ ] The existing test `TestStateDaemon_WritesPIDFileOnStartup` and friends continue to pass — the lock seam defaults to a real `unix.Flock` that will succeed against a fresh `t.TempDir()`, so non-contention tests are unaffected.
- [ ] Tests use `t.TempDir()` via `PORTAL_STATE_DIR` for isolation; tests do not use `t.Parallel()`.
- [ ] Project `CLAUDE.md` `state` package row updated to note the `daemon.lock` singleton invariant — one short sentence in the existing row format, surfaced alongside the existing `BootstrapPortalSaver` / `IsRestoringSet` references. This addresses the spec's "to be evaluated during planning" disposition and keeps the doc + behaviour in a single commit.
```

**Resolution**: Pending
**Notes**:

---
