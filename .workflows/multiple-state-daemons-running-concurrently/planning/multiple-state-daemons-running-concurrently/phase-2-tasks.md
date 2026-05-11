---
phase: 2
phase_name: Synchronous Kill Barrier and Singleton Invariant Verification
total: 3
---

## multiple-state-daemons-running-concurrently-2-1 | approved

### Task 2.1: Add seam-injectable killSaverAndWaitForDaemon helper

**Problem**: `EnsurePortalSaverVersion` and `BootstrapPortalSaver` both kill `_portal-saver` and immediately respawn the session without waiting for the prior `portal state daemon` process to observe its cancelled context and exit. Because the daemon's `tick()` runs synchronously inside the select arm at `cmd/state_daemon.go:54-63`, `ctx.Done()` is only reachable between ticks — a sweep already in flight runs to completion (cold-sweep ceiling 3.9 s on the affected user's 24-pane / ~28 MB scrollback profile) after SIGHUP arrives. The respawn therefore races a still-running prior daemon. Phase 1's `flock` is the structural safety net, but absent a barrier every recycle would produce a *"lock held; exiting"* WARN from the new daemon and (briefly) an empty `_portal-saver` session until the next bootstrap retries.

**Solution**: Add a new shared helper `killSaverAndWaitForDaemon(c *Client, stateDir string) error` in `internal/tmux/portal_saver.go`. The helper captures the prior PID via `state.ReadPIDFile(stateDir)` **before** issuing `KillSession(PortalSaverName)`, then polls `state.IsProcessAlive(prior_pid)` at a 50 ms cadence until the probe returns false or a 5-second timeout elapses. On clean exit within the timeout the helper returns silently (no log). On timeout it emits **one** WARN-level log line via the daemon-state `Logger` (component `ComponentBootstrap`) and returns nil — never fatal. All three sources of non-determinism (PID-file read, process-alive probe, time source) are seamed via package-level `var`s so unit tests can drive every path without spawning real processes or burning real wall time.

**Outcome**: A single, well-tested helper that makes the common-case recycle synchronous (prior daemon dead before the new daemon forks) while bounding worst-case latency at 5 s. The helper is self-contained — it does not yet wire into the call sites (that is Task 2.2) — and is fully covered by unit tests in `internal/tmux/portal_saver_test.go` that exercise every branch (clean exit, timeout, missing/dead/corrupted PID file, no-PID early exit, both successful and timed-out polling) without real processes or real sleeps.

**Do**:
- In `internal/tmux/portal_saver.go`, declare four new package-level seam `var`s adjacent to the existing `BootstrapAliveCheck` / `PortalSaverRetryDelay` seams:
  - `var killBarrierReadPID = state.ReadPIDFile` — returns `(int, error)`.
  - `var killBarrierIsAlive = state.IsProcessAlive` — returns `bool`.
  - `var killBarrierPollInterval = 50 * time.Millisecond` — exported as a `var` so tests can shrink (mirrors `PortalSaverRetryDelay`).
  - `var killBarrierTimeout = 5 * time.Second` — sized above the 3.9 s cold-sweep ceiling per spec, exported as a `var` for testability.
- Declare a logger seam matching the existing `MigrationLogger` precedent in `internal/tmux/hooks_register.go:137`: define a minimal local interface `BarrierLogger { Warn(component, format string, args ...any) }` and a package-level `var killBarrierLogger BarrierLogger = noopBarrierLogger{}` initialised to a no-op default. Production wiring at the Task 2.2 call sites replaces the no-op via a top-level `init()` (or via the existing bootstrap-adapter path that injects a real `*state.Logger`, which structurally satisfies the interface). Tests install a recording fake via the package var with `t.Cleanup` reset. The load-bearing requirement is that the timeout produces **exactly one** WARN-level event observable by tests through the recorder seam; log content is illustrative per spec §"Acceptance Criteria → Observability".
- Implement `killSaverAndWaitForDaemon(c *Client, stateDir string) error`:
  1. Call `killBarrierReadPID(stateDir)`. If the result is `(0, err)` for any reason (`ErrPIDFileAbsent`, parse error, generic read error), skip polling — there is no prior daemon to wait on. Issue `c.KillSession(PortalSaverName)` tolerantly (`_ = c.KillSession(...)`) and return nil.
  2. If `priorPID > 0` and `killBarrierIsAlive(priorPID)` is already false, the prior daemon has already exited; issue the tolerant kill and return nil immediately (no log, no polling).
  3. Otherwise: issue `c.KillSession(PortalSaverName)` tolerantly. Then enter a polling loop using `time.NewTicker(killBarrierPollInterval)` and a deadline computed from `time.Now().Add(killBarrierTimeout)`. After each tick re-probe `killBarrierIsAlive(priorPID)`; on false return nil. If the deadline passes without the probe returning false, emit one WARN line (e.g. *"prior daemon did not exit within timeout"*, content not load-bearing) and return nil.
- Keep the helper non-fatal: the spec requires it to return cleanly on timeout so callers can proceed to respawn under the protection of Phase 1's `flock`. Do **not** propagate the timeout as an error.
- The helper must not write to the state directory or mutate any global state beyond its log emission. The tolerant kill mirrors the existing call-site behaviour (`_ = c.KillSession(...)` at line 111).
- The polling loop must use the **injected** time source (`killBarrierPollInterval` ticker) so tests with a shrunk interval complete in microseconds. The deadline check should consult `time.Now()` — for tests that need to simulate "PID never dies," shrinking the timeout to 1 ms is the supported approach (matches the existing `shrinkRetryDelay` test helper pattern).
- Do **not** wire the helper into `EnsurePortalSaverVersion` or `BootstrapPortalSaver` in this task — that is Task 2.2's scope. The helper is dead code on `main` after this task lands; the next task wires it in.

**Acceptance Criteria**:
- [ ] `killSaverAndWaitForDaemon(c *Client, stateDir string) error` exists in `internal/tmux/portal_saver.go` and is unexported (it is package-internal — both call sites live in the same file).
- [ ] Four seams exist as package-level `var`s: `killBarrierReadPID`, `killBarrierIsAlive`, `killBarrierPollInterval`, `killBarrierTimeout`. Production values default to `state.ReadPIDFile`, `state.IsProcessAlive`, `50 * time.Millisecond`, and `5 * time.Second` respectively.
- [ ] A WARN log emission seam exists (logger interface, package-level `var`, or equivalent) such that tests can record exactly the WARN emitted on timeout and exactly zero WARNs on every other branch.
- [ ] On prior-PID-dies-within-timeout, helper returns nil with zero WARN emissions.
- [ ] On prior-PID-never-dies, helper returns nil after `killBarrierTimeout` wall-time has elapsed (via injected clock) with exactly one WARN emission. Wall time MUST be bounded — the helper does not block past `killBarrierTimeout`.
- [ ] On `ReadPIDFile` returning `ErrPIDFileAbsent`, generic read error, or parse error, helper issues the tolerant kill and returns nil immediately with zero WARN emissions and zero polling iterations.
- [ ] On `IsProcessAlive(priorPID) == false` at the first check (PID file points to an already-dead process), helper issues the tolerant kill and returns nil immediately with zero WARN emissions and zero polling iterations.
- [ ] The helper issues `c.KillSession(PortalSaverName)` exactly once per invocation regardless of which branch is taken, and tolerates kill errors (matches existing line 111 behaviour: `_ = c.KillSession(...)`).
- [ ] Helper does not write to the state directory and does not touch `daemon.pid`, `daemon.version`, or `daemon.lock`.
- [ ] All four seams are reset by tests via `t.Cleanup` (no leakage between test cases), matching the existing `stubAliveCheck` / `shrinkRetryDelay` test-helper pattern in `portal_saver_test.go`.
- [ ] `go build ./...` and `go test ./internal/tmux/...` pass.
- [ ] No use of `t.Parallel()` in any new test.

**Tests**:

Location: `internal/tmux/portal_saver_test.go` (extend existing file; reuse `stubAliveCheck` / `shrinkRetryDelay` helpers; introduce new helpers `stubKillBarrierPID`, `stubKillBarrierAlive`, `shrinkKillBarrierClock`, and a recording WARN logger).

- `"it returns nil with no WARN when prior PID dies before timeout"` — seed `killBarrierReadPID` to return `(12345, nil)`; seed `killBarrierIsAlive` to return true for the first N calls then false; shrink `killBarrierPollInterval` to a microsecond; assert helper returns nil, kill-session was called exactly once, the alive probe was called at least twice (first check + at least one poll iteration), and zero WARN entries were recorded.
- `"it emits one WARN and returns nil when prior PID never dies within timeout"` — seed `killBarrierReadPID` to return `(12345, nil)`; seed `killBarrierIsAlive` to always return true; shrink both `killBarrierPollInterval` and `killBarrierTimeout` to small values (e.g. 1 ms / 5 ms); assert helper returns nil, recorded total wall time is bounded by `killBarrierTimeout + epsilon`, exactly one WARN entry was recorded, and the kill-session call still occurred.
- `"it skips polling and returns immediately when daemon.pid is absent"` — seed `killBarrierReadPID` to return `(0, state.ErrPIDFileAbsent)`; seed `killBarrierIsAlive` to t.Fatalf if called (assertion: polling never starts); assert helper returns nil, kill-session called exactly once, zero WARN entries.
- `"it skips polling and returns immediately when daemon.pid is corrupted"` — seed `killBarrierReadPID` to return `(0, errors.New("parse daemon.pid: ..."))`; same expectations as the absent case.
- `"it skips polling and returns immediately when daemon.pid is unreadable"` — seed `killBarrierReadPID` to return `(0, errors.New("read daemon.pid: permission denied"))`; same expectations as absent case (defensive — equivalent treatment to no PID).
- `"it skips polling and returns immediately when prior PID is already dead"` — seed `killBarrierReadPID` to return `(12345, nil)`; seed `killBarrierIsAlive` to return false on the first call and t.Fatalf on subsequent calls (assertion: polling never enters its loop); assert helper returns nil, kill-session called once, zero WARN entries, alive-probe called exactly once.
- `"it tolerates a failing kill-session call"` — seed `killBarrierReadPID` to return `(12345, nil)`; seed `killBarrierIsAlive` to return false immediately; mock `c.KillSession` to return an error; assert helper still returns nil with no WARN (mirrors the existing `_ = c.KillSession(...)` tolerance at line 111).
- `"it does not mutate state-directory files"` — wire helper against a `t.TempDir()` and assert no `daemon.lock`, `daemon.version`, or modified `daemon.pid` after the helper returns (the helper is read-only on state dir).

**Edge Cases**:
- Prior PID dies between `ReadPIDFile` and the first `IsProcessAlive` probe — handled by the "already dead" early-exit branch.
- Prior PID is reused by an unrelated process between kill and probe — out of scope for this fix per spec; the lock (Phase 1) is the structural guard, not the barrier. The barrier is a polite synchronisation, not a correctness guarantee.
- `state.IsProcessAlive` returns true via `EPERM` (process exists but not signalable) — still treated as alive, polling continues. This is `state.IsProcessAlive`'s documented behaviour at `internal/state/daemon_state.go:70-72` and is intentional.
- `killBarrierPollInterval > killBarrierTimeout` — degenerate config where the first poll fires after the deadline; the deadline check on entry to the polling loop must handle this (first probe returns true → deadline exceeded → emit WARN and return). Test this via `killBarrierPollInterval = killBarrierTimeout` to guarantee at most one tick.
- Negative or zero prior PID (e.g. corrupted file content surviving the parse) — `state.IsProcessAlive` already returns false for `pid <= 0` (`internal/state/daemon_state.go:62-64`), so the "already dead" branch handles this cleanly. No special-case logic required.

**Context**:
> From Specification §"Fix Part 2: Synchronous Kill Barrier":
> - Steps 1–7 of the barrier behaviour are reproduced verbatim in the Do section above.
> - 5-second timeout chosen to sit above the 3.9 s cold-sweep upper bound at the affected user's profile.
> - Helper returns nil on timeout — never fatal. Phase 1's `flock` is the safety net.
> - The two kill sites are factored into a **shared helper** rather than duplicating the poll loop (Task 2.2 wires both).
>
> From Specification §"Test Strategy → Unit tests — synchronous kill barrier":
> - Mock seams required: `ReadPIDFile` wrapper var, `IsProcessAlive` injectable, time source injectable.
> - Required cases enumerate: dies-in-timeout (no WARN), never-dies (WARN + bounded wall), no-PID, dead-PID, unreadable/corrupted PID.
> - Both call sites verified through the shared helper — that assertion is Task 2.2's responsibility; this task delivers and tests the helper in isolation.
>
> From Specification §"Acceptance Criteria → Clean handover on the common case":
> - No WARN line emitted on the single-invocation recycle path when the prior daemon exits within timeout.
>
> From Specification §"Acceptance Criteria → Observability":
> - Log content is illustrative, not load-bearing. Tests assert **presence and WARN level** of one log line per timeout event — not literal text. Implementers may adjust wording.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` §"Fix Part 2: Synchronous Kill Barrier", §"Test Strategy → Unit tests — synchronous kill barrier", §"Acceptance Criteria → Clean handover on the common case", §"Acceptance Criteria → Observability".

## multiple-state-daemons-running-concurrently-2-2 | approved

### Task 2.2: Wire barrier into both kill call sites (EnsurePortalSaverVersion + BootstrapPortalSaver)

**Problem**: The shared barrier helper from Task 2.1 is dead code until both kill call sites in `internal/tmux/portal_saver.go` are updated to invoke it. Today the version-mismatch branch in `EnsurePortalSaverVersion` (line 111) and the stale-pidfile recovery branch in `BootstrapPortalSaver` (line 68) both call `c.KillSession(PortalSaverName)` directly with no synchronisation, then fall through to `BootstrapPortalSaver`'s `createPortalSaverWithRetry` path which forks a new daemon immediately. The spec explicitly requires both sites to use the barrier: "these are not alternatives" — leaving either site unrouted means one recycle path still races.

**Solution**: Replace the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:111` (inside `EnsurePortalSaverVersion`'s mismatch branch) and the bare `_ = c.KillSession(PortalSaverName)` call at `internal/tmux/portal_saver.go:68` (inside `BootstrapPortalSaver`'s stale-daemon branch) with `_ = killSaverAndWaitForDaemon(c, stateDir)`. Both call sites now route through the same synchronisation helper. `BootstrapPortalSaver`'s signature already accepts `stateDir` (line 63), and `EnsurePortalSaverVersion` already has `stateDir` in scope (line 106), so no signature changes are required. Verify by injection-recorder unit tests that **both** sites invoke the helper, and that the steady-state path (saver alive, version matches) does **not** invoke it.

**Outcome**: Every kill of `_portal-saver` triggered by portal's saver-bootstrap surface is now bracketed by the synchronous barrier from Task 2.1. The common-case recycle (single invocation, prior daemon exits within 100 ms) is silent. Steady-state bootstrap latency is unchanged (barrier not entered when no kill is needed). The composed system (Phase 1 lock + Phase 2 barrier) is now in place; Task 2.3's real-tmux integration test validates the composition end-to-end.

**Do**:
- In `internal/tmux/portal_saver.go`, modify `EnsurePortalSaverVersion` at the mismatch branch (currently line 108-112):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 111) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The helper itself issues the kill, so the surrounding `if portalSaverVersionMismatch(...) && c.HasSession(PortalSaverName)` guard is preserved unchanged — the barrier is only entered when both the mismatch and presence conditions hold (steady-state path with matching version OR absent session does not enter).
  - The error return is intentionally ignored: the helper returns nil even on timeout per Task 2.1, and the spec mandates the caller proceeds to `BootstrapPortalSaver` regardless.
- In `internal/tmux/portal_saver.go`, modify `BootstrapPortalSaver`'s stale-daemon branch (currently line 66-70):
  - Replace `_ = c.KillSession(PortalSaverName)` (line 68) with `_ = killSaverAndWaitForDaemon(c, stateDir)`.
  - The surrounding `if sessionPresent && !BootstrapAliveCheck(stateDir)` guard is preserved unchanged.
  - `stateDir` is already a parameter to `BootstrapPortalSaver` (line 63), so no signature change required.
- Introduce a test-only injection recorder via a package-level `var killSaverAndWaitForDaemonFn = killSaverAndWaitForDaemon` at package scope and route both call sites through this `var` so tests can swap in a recorder. This is a minimal seam — the function body itself was already seamed inside Task 2.1. Both call-site replacements above use `killSaverAndWaitForDaemonFn(c, stateDir)`, not the helper directly.
- **Wire the production logger into `killBarrierLogger`** (handed off from Task 2.1): expose an exported setter `SetBarrierLogger(l BarrierLogger)` in `internal/tmux/portal_saver.go` that assigns `killBarrierLogger = l` when `l != nil`, mirroring the parameter-injection precedent at `internal/bootstrapadapter/adapters.go:69` (`RegisterPortalHooks(r.Client, r.Logger)`). Then add a call to `tmux.SetBarrierLogger(r.Logger)` from `internal/bootstrapadapter/adapters.go` (the same site that already constructs the `*state.Logger` for `RegisterPortalHooks`) so the no-op default is replaced exactly once at adapter wiring time, before `BootstrapPortalSaver` or `EnsurePortalSaverVersion` is first invoked. The `*state.Logger` type structurally satisfies `BarrierLogger { Warn(component, format string, args ...any) }` — no adapter shim required.
- Verify by code review that there are exactly **two** call sites to the barrier helper in `internal/tmux/portal_saver.go` after the change (one in `EnsurePortalSaverVersion`, one in `BootstrapPortalSaver`); no other production call sites should exist.
- Do not add new logging at either call site — the helper owns the WARN line on timeout, the call site is silent.
- Steady-state path verification: when `portalSaverVersionMismatch` returns false (stored version matches current and is non-empty/non-dev), `EnsurePortalSaverVersion` returns without invoking the helper. When `BootstrapAliveCheck(stateDir)` returns true (live daemon), `BootstrapPortalSaver` skips the stale-daemon branch and goes straight to set-option, never invoking the helper.

**Acceptance Criteria**:
- [ ] Both kill call sites in `internal/tmux/portal_saver.go` invoke the barrier helper (via the `killSaverAndWaitForDaemonFn` seam): line ~111 in `EnsurePortalSaverVersion`, line ~68 in `BootstrapPortalSaver`.
- [ ] The bare `_ = c.KillSession(PortalSaverName)` calls at those two sites are no longer present in the production code path (grep for `c.KillSession(PortalSaverName)` in `portal_saver.go` should find them only **inside** `killSaverAndWaitForDaemon`).
- [ ] `BootstrapPortalSaver`'s function signature and `EnsurePortalSaverVersion`'s function signature are unchanged.
- [ ] When `portalSaverVersionMismatch` returns false in `EnsurePortalSaverVersion`, the helper is not invoked (steady-state path verified via recorder seam).
- [ ] When `BootstrapAliveCheck(stateDir)` returns true in `BootstrapPortalSaver`, the helper is not invoked (steady-state path verified via recorder seam).
- [ ] When `sessionPresent` is false in `BootstrapPortalSaver` (fresh server, no existing saver session), the helper is not invoked — kill is meaningless when there is no session.
- [ ] `tmux.SetBarrierLogger` is exported and called from `internal/bootstrapadapter/adapters.go` so `killBarrierLogger` is replaced with the production `*state.Logger` at adapter wiring time; the no-op default does not persist into production. Verified by a unit test in `internal/tmux/portal_saver_test.go` that asserts the WARN-on-timeout reaches a recording logger installed via `SetBarrierLogger` (mirrors the existing `t.Cleanup` reset pattern).
- [ ] All existing tests in `internal/tmux/portal_saver_test.go` continue to pass with the helper seam stubbed appropriately (the existing tests that exercise the kill path will now need to stub the barrier seam — most simply, stub `killSaverAndWaitForDaemonFn` to a recorder that delegates to `c.KillSession` so existing call-count assertions still hold).
- [ ] `go test ./internal/tmux/...` is green; `go test ./...` is green.
- [ ] No use of `t.Parallel()`.

**Tests**:

Location: `internal/tmux/portal_saver_test.go` (extend existing file).

- `"it invokes the barrier helper on version-mismatch in EnsurePortalSaverVersion"` — seed `daemon.version` with a stored value different from the current version, mark session as present, stub `killSaverAndWaitForDaemonFn` to a recorder. Call `EnsurePortalSaverVersion(client, dir, "current")`. Assert: recorder invoked exactly once; `c.HasSession(PortalSaverName)` was probed first (the guard); kill-session count via the underlying mock is consistent with the recorder having delegated to the real helper or having issued the kill itself (depending on recorder shape).
- `"it does not invoke the barrier helper on version-match in EnsurePortalSaverVersion"` — seed `daemon.version` with a value matching the current version, mark session as present and alive. Stub `killSaverAndWaitForDaemonFn` to a recorder that t.Fatalf's if called. Call `EnsurePortalSaverVersion(client, dir, "current")` and assert it returns nil without invoking the helper. Set-option must still fire (BootstrapPortalSaver still applies destroy-unattached=off).
- `"it invokes the barrier helper on stale-daemon in BootstrapPortalSaver"` — `BootstrapAliveCheck` stub returns false, session present. Stub `killSaverAndWaitForDaemonFn` to a recorder. Call `BootstrapPortalSaver(client, dir)`. Assert: recorder invoked exactly once before new-session fires.
- `"it does not invoke the barrier helper when session is absent in BootstrapPortalSaver"` — `HasSession` returns false on first probe. Stub helper recorder to t.Fatalf if called. Call `BootstrapPortalSaver(client, dir)` and assert it proceeds to new-session without invoking the barrier (no prior daemon to wait for).
- `"it does not invoke the barrier helper when daemon is alive in BootstrapPortalSaver"` — `HasSession` returns true, `BootstrapAliveCheck` stub returns true. Stub helper recorder to t.Fatalf if called. Call `BootstrapPortalSaver(client, dir)` and assert it returns nil without invoking the barrier.
- `"it preserves existing kill-session behaviour when the helper delegates to KillSession"` — restore the existing test `TestBootstrapPortalSaver_KillsAndRecreatesWhenSessionExistsButDaemonDead`'s expectation that exactly one kill-session, one new-session, and one set-option call occur; ensure the barrier wiring did not change this observable call count when the helper is left at its production behaviour (or stubbed to delegate).
- `"it preserves the kill-session-precedes-new-session ordering through the barrier"` — same scenario as above; assert kill-session index < new-session index in the mock's `Calls` log (existing test pattern at lines 197-213).
- `"it tolerates the barrier helper's WARN-on-timeout return path"` — stub `killSaverAndWaitForDaemonFn` to issue the kill and return nil after a simulated timeout (no real wait); assert `BootstrapPortalSaver` and `EnsurePortalSaverVersion` both proceed to new-session and set-option, returning nil overall (the spec mandates the timeout path is non-fatal at the call site).
- `"it routes WARN-on-timeout through the logger installed via SetBarrierLogger"` — install a recording `BarrierLogger` via `tmux.SetBarrierLogger(recorder)` with `t.Cleanup` resetting to the no-op default; seed the helper for the timeout path (PID never dies, shrunk clock); invoke `EnsurePortalSaverVersion` against a version-mismatch scenario; assert the recorder captured exactly one WARN entry under `ComponentBootstrap`. Guards against the no-op default persisting into production.

**Edge Cases**:
- `EnsurePortalSaverVersion` with mismatch=true but `HasSession` returns false — the existing guard at line 108 (`&& c.HasSession(PortalSaverName)`) means the barrier is **not** invoked when there is no session to kill. This is correct and preserved unchanged (covered by `TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists`).
- Tolerated `KillSession` errors from within the barrier helper — handled by Task 2.1; verified at the call site by the "tolerates a failing kill-session call" test from Task 2.1. The call site does not need additional tolerance because the helper already returns nil on failure.
- Concurrent bootstrap producing a race where one invocation reaches the kill site while another is mid-create — the spec accepts that concurrent bootstraps may produce one WARN on whichever loses the lock (Phase 1's responsibility). The barrier wiring does not introduce new ordering hazards.
- The retry loop in `createPortalSaverWithRetry` (line 138-158) is downstream of the barrier and unchanged — the barrier waits for the prior daemon to exit, then `createPortalSaverWithRetry` handles the create itself. Retry-on-transient-failure semantics are preserved.
- `@portal-restoring` marker lifetime under barrier timeout: bootstrap step 3 sets the marker and step 7 clears it; the barrier (executed inside step 4, EnsureSaver) extends the in-bracket window by up to 5 s on the timeout path. The spec explicitly accepts this — daemon `captureAndCommit` suppression remains in force throughout the bootstrap window, and the 5 s extension does not affect the correctness of steps 5 (Restore), 6 (EagerSignalHydrate), or downstream warning surfacing. **Do not** tighten the timeout, narrow the marker lifetime, or add a diagnostic for "marker still set while barrier running" — all three would regress against an explicitly accepted spec property.

**Context**:
> From Specification §"Fix Part 2: Synchronous Kill Barrier → Both kill sites use the barrier":
> > There are **two kill call sites** in the saver-bootstrap surface and **both** must use the barrier — these are not alternatives:
> > 1. **`EnsurePortalSaverVersion`** version-mismatch branch (`internal/tmux/portal_saver.go:108-112`) — fires when stored ≠ current version.
> > 2. **`BootstrapPortalSaver`** stale-pidfile recovery branch (`internal/tmux/portal_saver.go:66-70`) — fires when the session exists but `BootstrapAliveCheck` reports the daemon dead.
> > The two sites are factored into a **shared helper** rather than duplicating the poll loop.
>
> From Specification §"Acceptance Criteria → No regression on the steady-state critical path":
> > When the saver already exists and the version matches (no recycle needed), `EnsurePortalSaverVersion` does not invoke the kill barrier. Latency on this path is unchanged from current behaviour.
>
> From Specification §"Test Strategy → Unit tests — synchronous kill barrier → Required cases":
> > Barrier exercised through both call sites:
> > - Version-mismatch branch in `EnsurePortalSaverVersion`
> > - Stale-pidfile branch in `BootstrapPortalSaver`
> > - Both paths must invoke the shared helper. Asserted by triggering each path independently and recording barrier invocation.
>
> Existing call-site code shape (for reference during the edit):
> - `EnsurePortalSaverVersion` lines 106-114 — replace line 111 only.
> - `BootstrapPortalSaver` lines 63-83 — replace line 68 only; `stateDir` already in scope.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` §"Fix Part 2: Synchronous Kill Barrier → Both kill sites use the barrier", §"Acceptance Criteria → No regression on the steady-state critical path", §"Test Strategy → Unit tests — synchronous kill barrier".

## multiple-state-daemons-running-concurrently-2-3 | approved

### Task 2.3: Real-tmux integration test asserts singleton invariant after recycle

**Problem**: The bug is the conjunction of two structural defects (missing singleton lock + missing kill barrier) that compose only under real OS process semantics — the kernel's `flock` release behaviour, the daemon's tick loop reaching `ctx.Done()`, and tmux's `new-session` actually forking a child. Seam-level unit tests cannot model this composition: they fix a pidfile and probe it, but cannot model "what happens when the pidfile is overwritten while the prior daemon still runs." The spec explicitly names this asymmetry as the reason a real-tmux integration test is required and is not redundant with seam coverage. Without this test, a future regression to either Phase 1 (lock) or Phase 2 (barrier) could land green on unit tests and reintroduce the multi-daemon condition silently.

**Solution**: Add a new integration test file `internal/tmux/portal_saver_integration_test.go` using the `tmuxtest` real-tmux socket fixture. The test sets up an isolated tmux server via `tmuxtest.New(t, "ptl-saver-")`, calls `EnsurePortalSaverVersion` once to create the saver session with the current binary's version, then **directly writes a differing value into `<stateDir>/daemon.version`** between invocations (no new test seam — this exercises the real `portalSaverVersionMismatch` comparison logic with no mocking). It then calls `EnsurePortalSaverVersion` a second time to trigger the recycle and asserts via `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l` that exactly one live `portal state daemon` process exists after both calls return. The test skips when `tmux` is unavailable (`tmuxtest.SkipIfNoTmux(t)`) and isolates state via `t.TempDir()`.

**Outcome**: A load-bearing integration test that would have caught the bug in CI had it existed before. The test exercises the composed Phase 1 + Phase 2 fix end-to-end — Phase 1's lock prevents N > 1 even under barrier-timeout conditions, Phase 2's barrier keeps the common case silent. Future regressions to either guard fail this test loudly.

**Do**:
- Create new file `internal/tmux/portal_saver_integration_test.go` (build-tag-free; gate via `tmuxtest.SkipIfNoTmux(t)` at the top of each test function, matching the existing `internal/tmux/hooks_migration_test.go` convention).
- Test function: `TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle`.
- Test body steps:
  1. `tmuxtest.SkipIfNoTmux(t)` at the very top.
  2. `dir := t.TempDir()` — per-test isolated `stateDir`.
  3. `sock := tmuxtest.New(t, "ptl-saver-")` — isolated tmux server with auto-cleanup.
  4. `client := sock.Client()` — `*tmux.Client` wired to the isolated socket.
  5. First invocation: `if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil { t.Fatalf(...) }`.
  6. `sock.WaitForSession(t, tmux.PortalSaverName, 5*time.Second)` — confirm the saver session is created and queryable before proceeding.
  7. Capture the tmux server PID by parsing `sock.Run(t, "display-message", "-p", "#{pid}")` (or equivalent — `tmux display-message -p '#{pid}'` returns the server PID).
  8. Wait briefly (poll with bounded timeout, e.g. up to 2 s) for `daemon.pid` to exist at `state.DaemonPID(dir)` — the daemon must have started, won the lock (Phase 1), and written its pidfile. If `daemon.pid` is not present within the bound, t.Fatalf with a diagnostic listing the contents of `dir`.
  9. **Directly write a differing value** into `<stateDir>/daemon.version`: `if err := state.WriteVersionFile(dir, "v-test-0-old"); err != nil { t.Fatalf(...) }`. This is the load-bearing step — no new test seam is introduced; the test relies on `portalSaverVersionMismatch` comparing `"v-test-0-old"` (stored) vs `"v-test-1"` (current passed to the second call) and returning true.
  10. Second invocation: `if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil { t.Fatalf(...) }`. This must trigger the recycle: kill the old saver, run the barrier (Task 2.1/2-2), wait for the prior daemon to exit, then create a fresh saver.
  11. `sock.WaitForSession(t, tmux.PortalSaverName, 5*time.Second)` — confirm the recycled session is back.
  12. Wait briefly (poll with bounded timeout, e.g. up to 3 s — slightly above the 1 s tick + 100 ms barrier-poll cadence) for the prior daemon to actually exit. The probe: `pgrep -P <server_pid> -f 'portal state daemon'` count should converge to 1.
  13. **Assert singleton**: invoke `pgrep -P <server_pid> -f 'portal state daemon'` via `os/exec`, count the lines of output, assert exactly 1 (or t.Fatalf with diagnostic: full pgrep output, `daemon.pid` contents, `daemon.version` contents, and the recorded `server_pid`).
- Cleanup: `tmuxtest.New`'s `t.Cleanup` handler issues `kill-server` on the isolated socket, which SIGHUPs the saver session, which kills the daemon. No additional cleanup required.
- The test must use a real binary on PATH (`portal state daemon` is the command the saver session runs as its initial process — this is `portalSaverCommand` at `internal/tmux/portal_saver.go:32`). Add a precondition check at the top of the test: if `portal` is not on PATH (`exec.LookPath("portal")` errors), `t.Skip("portal binary not on PATH; skipping integration test")` — matching the `tmuxtest.SkipIfNoTmux` convention. CI environments without the built binary skip cleanly.
- Document the test's load-bearing role in a doc comment at the top of the test file (referencing the spec: "this is the test that would have caught the bug in CI had it existed before").
- Do **not** introduce a new seam to intercept `portalSaverVersionMismatch` — the spec explicitly requires the test to exercise the real comparison logic. The recycle is triggered by the on-disk version-marker write at step 9.
- Do **not** use `t.Parallel()` (project convention plus per-test `stateDir`/socket isolation makes parallel runs nontrivial).

**Acceptance Criteria**:
- [ ] New file `internal/tmux/portal_saver_integration_test.go` exists and contains `TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle`.
- [ ] Test calls `tmuxtest.SkipIfNoTmux(t)` and `t.Skip` for missing `portal` binary, so CI environments without either skip cleanly with informative messages.
- [ ] Test uses `t.TempDir()` for per-test `stateDir` isolation.
- [ ] Test uses `tmuxtest.New(t, "ptl-saver-")` for an isolated tmux server with automatic cleanup.
- [ ] Test calls `tmux.EnsurePortalSaverVersion` twice — first with `currentVersion = "v-test-1"`, then writes `"v-test-0-old"` directly to `daemon.version` via `state.WriteVersionFile`, then calls again with `currentVersion = "v-test-1"`.
- [ ] **No new test seams are introduced** for `portalSaverVersionMismatch` — the real comparison drives the recycle.
- [ ] Test captures the tmux server PID via `tmux display-message -p '#{pid}'` (or equivalent) and asserts `pgrep -P <server_pid> -f 'portal state daemon'` produces exactly one line of output after both `EnsurePortalSaverVersion` calls return and a bounded settle window elapses.
- [ ] On failure, the test prints a diagnostic that includes: the full `pgrep` output, contents of `daemon.pid` and `daemon.version`, and the captured server PID — so future regressions are debuggable from CI logs alone.
- [ ] Test does not use `t.Parallel()`.
- [ ] `go test ./internal/tmux/... -run TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle` passes against the implemented Phase 1 + Phase 2 fix; the test would fail (count > 1) against `main` prior to the fix landing — confirm this implicit property by reviewing how the assertion shape responds when only Phase 1 lands without Phase 2 (Phase 1 alone produces count == 1 but with one WARN; Phase 2 silences the common case). Both compositions should pass the singleton invariant.
- [ ] `go test ./...` is green.

**Tests**:

This task **is** the test. The deliverable is the integration test itself plus the supporting file structure.

- `"TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle"` — primary test, described in the Do section.
- (Optional, recommended) `"TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle_BarrierTimeoutPath"` — a variant or sub-test that shortens `tmux.killBarrierTimeout` via seam injection to force the barrier-timeout path, asserting the singleton invariant **still** holds (because Phase 1's `flock` is the floor). This sub-test validates that the timeout path does not degrade to N > 1 — it may produce one WARN line on the second invocation, but the daemon count remains 1. **If implementing**: use `t.Run` to isolate from the primary test's expectations and reset seams via `t.Cleanup`. If the test author judges this variant adds significant CI flakiness risk or duplicates Phase 1's regression coverage, it may be omitted — the primary test is the load-bearing assertion.

**Edge Cases**:
- `tmux` not on PATH → skipped via `tmuxtest.SkipIfNoTmux(t)`.
- `portal` binary not on PATH → skipped via `exec.LookPath("portal")` check + `t.Skip`. Note: the daemon launched inside the tmux session is `portalSaverCommand = "portal state daemon"`, so the test requires the user-installed `portal` binary to be on PATH. CI configurations that build `portal` into a temp dir must add that dir to PATH for this test to run.
- The first daemon's tick is mid-flight when the kill arrives → the barrier (Task 2.1/2-2) waits for it; the integration test allows up to a 3 s settle window after the second `EnsurePortalSaverVersion` returns to absorb this without flakiness.
- Race between the second `new-session` and the first daemon's `flock` release → if the new daemon loses the lock, it exits status 0; the (empty or dead-pane) saver session converges on the next bootstrap. For this test, the singleton invariant is "exactly one live `portal state daemon` process," not "exactly one healthy saver session" — the assertion is on `pgrep` count, not on session state. This is the correct shape per spec §"Acceptance Criteria → Singleton invariant".
- The pgrep count converges asynchronously after `EnsurePortalSaverVersion` returns (the prior daemon's tick must complete and `ctx.Done()` must be observed). The bounded polling window (up to 3 s) absorbs this; if it does not converge within the window the test fails with diagnostics — this would indicate the barrier is not waiting (regression) or the daemon is not exiting (separate bug, also worth catching).
- Stray daemons from prior failed test runs → mitigated by `tmuxtest`'s isolated socket (the daemon is parented to a per-test tmux server PID, not the user's tmux server, and `pgrep -P <test_server_pid>` scopes the count to that server only).
- CI environment with extremely slow process startup (>5 s for daemon to write `daemon.pid` after `new-session` returns) → test fatals with a diagnostic in step 8. This is acceptable — such environments are out of scope; bound the wait at a generous 5 s to avoid spurious failures on normal CI.

**Context**:
> From Specification §"Test Strategy → Integration test — singleton invariant under real tmux":
> > Location: new `internal/tmux/portal_saver_integration_test.go` using `restoretest`/`tmuxtest` conventions (real-tmux socket fixture).
> > Skip behaviour: skip on CI when tmux is not available.
> > Required case: **Back-to-back recycle produces N=1** — set up a real tmux server, run `EnsurePortalSaverVersion` to create the saver, then **directly write a different value into `<stateDir>/daemon.version`** between calls (no new test seam, exercises real `portalSaverVersionMismatch` comparison logic), then run `EnsurePortalSaverVersion` again to trigger the recycle, then assert `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l == 1` after both calls return.
> > This is the **load-bearing test** for the bug — it would have caught the issue in CI had it existed before. Crucially, the existing `BootstrapAliveCheck` seam-level unit tests cannot model the failure mode here: they fix a pidfile and probe it, but cannot model "what happens when the pidfile is overwritten while the prior daemon still runs." That asymmetry is the reason the integration test is required and is not redundant with seam-level coverage.
>
> From Specification §"Acceptance Criteria → Singleton invariant":
> > At most one `portal state daemon` process exists per state directory at any time, regardless of how many bootstrap invocations have run during the tmux server lifetime.
> > Verified by: an integration test (real-tmux fixture) that runs two back-to-back `EnsurePortalSaverVersion` calls — one must trigger a recycle via version mismatch — and asserts exactly one live `portal state daemon` process when both calls complete.
>
> From `tmuxtest` package conventions (`internal/tmuxtest/socket.go`, `internal/tmuxtest/skip.go`):
> - `tmuxtest.New(t, prefix)` returns an isolated `*Socket` with `t.Cleanup`-registered `kill-server`.
> - `sock.Client()` returns a `*tmux.Client` wired to the isolated socket via `socketCommander` so production code talks only to the test's server.
> - `sock.WaitForSession(t, name, timeout)` polls `has-session` until the target appears (used here for both the initial create and the post-recycle re-create).
> - `tmuxtest.SkipIfNoTmux(t)` is the canonical skip helper.
>
> From Project conventions (CLAUDE.md):
> - "Tests **must not** use `t.Parallel()`" — this applies even though tests in this file use a fresh socket per test; the cmd-package mutable-state caveat does not apply here, but the project-wide convention does.
> - Real-tmux integration tests in `internal/tmux` already exist (`hooks_migration_test.go` imports `tmuxtest`) — this test follows the same shape.

**Spec Reference**: `.workflows/multiple-state-daemons-running-concurrently/specification/multiple-state-daemons-running-concurrently/specification.md` §"Test Strategy → Integration test — singleton invariant under real tmux", §"Acceptance Criteria → Singleton invariant".
