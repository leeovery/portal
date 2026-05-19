# Specification: Saver Kill-Respawn Loop Leaks Daemons

## Specification

### Problem Statement

Every portal bootstrap that runs step 4 (`EnsureSaver`) fires an unnecessary kill-respawn cycle on the `_portal-saver` session, leaking orphan `portal state daemon` processes and leaving no live saver session after bootstrap completes. The user-visible consequences are:

- **~520ms wasted per portal invocation** on a kill-respawn block that should not run when the daemon is healthy and the binary version matches.
- **Accumulating orphan daemons** — observed three concurrent `portal state daemon` processes parented to the same tmux server (oldest 5 days old, each ~40MB RSS). Only the most recent holds `daemon.lock`; the rest are stranded.
- **Silently-paused saves between portal invocations** — bootstrap ends with `_portal-saver` destroyed, so no daemon runs to capture state until the next bootstrap recreates it. The resurrection guarantee is silently broken.
- **Noisy diagnostic log** — three WARN lines emitted on every bootstrap (`prior daemon … did not exit within 5s` → `another daemon holds the lock; exiting` → `step 4 (EnsureSaver) failed: … no such session: _portal-saver`).

**Reproducibility:** Always, on any environment where (a) `daemon.version` is missing OR (b) the daemon's per-tick wall time exceeds the 5s kill-barrier window. The latter applies to any user with non-trivial scrollback volume (~23 panes × ~1.2MB rendered text was sufficient on the affected machine).

**Scope of impact:** Performance regression and silent reliability regression — portal still works functionally, but the resurrection daemon's continuity guarantee is violated between invocations and startup latency is degraded.

### Root Cause

The bug is the conjunction of two independent defects in the saver-bootstrap and daemon-startup pair, plus a third open question whose user-visible effect is neutralised by fixing the first two.

#### Defect 1 — Version-mismatch false positive when `daemon.version` is absent

`portalSaverVersionMismatch` (`internal/tmux/portal_saver.go`) collapses three distinct conditions into a single "mismatch" result: (a) genuine version disagreement, (b) dev-build workflows (stored or current is `dev`/empty), and (c) "version file absent". Case (c) is the false positive — file absence does not imply version mismatch; it merely means we cannot confirm the version, while the daemon may still be perfectly healthy.

`EnsurePortalSaverVersion` (`internal/tmux/portal_saver.go:249`) consults the mismatch predicate without first checking daemon aliveness. So any condition that removes `daemon.version` while leaving the daemon alive triggers an unnecessary kill on every subsequent bootstrap.

#### Defect 2 — Daemon SIGHUP-unresponsive within the 5s kill-barrier window

`defaultDaemonRun` (`cmd/state_daemon.go`) runs `tick()` synchronously inside the ticker's `select` arm:

```go
for {
    select {
    case <-ticker.C:
        tick(deps)               // synchronous; no ctx awareness inside
    case <-ctx.Done():
        return daemonShutdownFunc(deps)
    }
}
```

`ctx.Done()` is structurally unreachable during a tick. The expensive work inside `tick → captureAndCommit` iterates every live pane and invokes `tmux capture-pane -e -p -S -` (unbounded scrollback) per pane for the hash check. On the affected user's profile (~23 panes × ~1.2MB rendered text), measured wall time exceeds the 5s `killBarrierTimeout` sized by the prior fix.

When the barrier gives up early, the new daemon spawns, immediately collides with the still-held `daemon.lock`, exits cleanly **without writing `daemon.pid` or `daemon.version`**, destroys the just-created `_portal-saver` pane process, and triggers the `SetSessionOption(_portal-saver, destroy-unattached, off)` "no such session" cascade.

#### Defect 3 — `daemon.version` disappearance (open, instrumentation only)

Code-trace exhaustively enumerated every production file-removal path; **no production code path removes `daemon.version` individually**. The disappearance therefore originates from outside portal's production code (manual `--purge`, dev-build escape, or external process). Fixing Defect 1 makes the disappearance non-load-bearing for the user-visible symptom — Defect 3 becomes a follow-up question, not a blocker.

#### Why It Wasn't Caught

- The prior fix (`multiple-state-daemons-running-concurrently`, 2026-05-11) sized `killBarrierTimeout` at 5s against a measured 3.9s cold sweep with margin. The user's profile grew past that bound within months; no telemetry exposed the relationship.
- `portalSaverVersionMismatch`'s existing unit test pins the false-positive "absent → mismatch" behaviour as correct, codifying the bug as contract.
- No alive-daemon-with-missing-version-file integration test exists. The closest test verifies kill-respawn under explicit version mismatch, not under absent version.
- The orphan-leak symptom is invisible without `ps | grep portal`; the WARN cascade only lands in `portal.log`.

### Change 1 — Alive-check first in `EnsurePortalSaverVersion`

**Target:** `internal/tmux/portal_saver.go` — `EnsurePortalSaverVersion`, `portalSaverVersionMismatch`.

**Required behaviour:**

Rework the kill decision in `EnsurePortalSaverVersion` to consult `BootstrapAliveCheck(stateDir)` **before** the version-mismatch branch. The new decision matrix:

| Daemon alive? | Version file state | Versions match? | Action |
|---|---|---|---|
| Yes | Absent | (unknowable) | **No kill.** Write daemon.version defensively from bootstrap, then proceed to BootstrapPortalSaver. |
| Yes | Present, reads cleanly | Match | **No kill.** Proceed to BootstrapPortalSaver. |
| Yes | Present, reads cleanly | Mismatch (real upgrade) | **Kill.** Run `killSaverAndWaitForDaemon`, then BootstrapPortalSaver. |
| Yes | Read error (non-absent I/O failure) | (unknowable) | **Kill.** Conservative — treat unknown I/O failure as needing recycle. |
| No | (any) | (any) | **No kill needed.** No daemon to recycle. Proceed to BootstrapPortalSaver. |

`portalSaverVersionMismatch` keeps its current external shape but is **no longer the lone gate**. The alive-check classifies the situation first; the mismatch predicate is consulted only on the alive-with-readable-version branch.

**Defensive complement:** when bootstrap observes "alive daemon + absent `daemon.version`" on the survived path, write `daemon.version` from the bootstrap side before proceeding. This closes the lock-loser lifecycle hole (lock-loser daemons return cleanly before writing `daemon.version`, leaving the file observably absent until the next bootstrap repairs it).

**What stays unchanged:**

- The `daemon.lock` flock primitive and `killSaverAndWaitForDaemon` machinery from the prior bugfix (`multiple-state-daemons-running-concurrently`).
- `BootstrapPortalSaver` itself — only the gate in front of `killSaverAndWaitForDaemon` changes.
- The no-daemon path (alive-check returns false) — already correct, no behavioural change.
- Dev-build handling (`stored == "dev"` or `currentVersion == "dev"`) — preserve current "always recycle on dev" behaviour for development workflows.

**Rejected alternative:** distinguishing `ErrVersionFileAbsent` inside `portalSaverVersionMismatch` only (smaller change). Rejected because it narrows the symptom (file absent → no kill) but misses the broader invariant: a healthy daemon should never be killed for a missing version marker regardless of *why* the file is missing. The alive-check ordering captures the broader invariant.

### Change 2 — Context-aware `captureAndCommit`

**Target:** `cmd/state_daemon.go` — `defaultDaemonRun`, `tick`, `captureAndCommit`. Signature updates may propagate into `internal/state/capture.go` if the per-pane callers live there.

**Required behaviour:**

Thread `ctx` from `defaultDaemonRun` through `tick` into `captureAndCommit` and the per-pane loop. Between per-pane iterations of the structural-index loop, check `ctx.Done()` and return early on cancellation.

This caps worst-case daemon-exit latency at **one pane's `capture-pane` wall time** rather than "all panes' aggregated wall time" — bounded by per-pane scrollback size, no longer by the user's total pane count.

**Cancellation semantics:**

- Cancellation is observed **between per-pane iterations**, not mid-`capture-pane` invocation. An in-flight `tmux capture-pane` call completes before the cancel is honoured.
- On cancellation, return early **without committing partial state**. The current tick is abandoned cleanly — no half-applied scrollback writes, no partial commit.
- The outer `select { ticker.C / ctx.Done() }` loop continues to handle the no-tick-in-progress cancellation path. Its semantics are unchanged.
- Shutdown flush behaviour (`daemonShutdownFunc`) is unchanged — it still runs on the cancelled-context path after the tick-loop returns.

**What stays unchanged:**

- The `killSaverAndWaitForDaemon` barrier on the bootstrap side. The barrier was correct against its contract; the fix completes the daemon side of that contract.
- `killBarrierTimeout` stays at 5s. Raising the timeout was considered and rejected — it defers the next profile-growth failure without resolving the underlying contract violation.
- The per-tick capture algorithm itself — pane enumeration, hashing, dedup, commit — is unchanged. Only the loop becomes interruptible.
- `tmux capture-pane` invocation is **not** bounded to a fixed line count. Per-pane scrollback semantics are preserved (capture-pane bounding is out of scope, see Out of Scope).

**Rejected alternatives:**

- **Raise `killBarrierTimeout` from 5s to 10s.** Rejected — defers the next failure rather than fixing it. The prior bugfix already shipped 5s with margin against a measured 3.9s sweep; the user's profile grew past that bound within months. Re-sizing without making the daemon ctx-aware repeats the same mistake.
- **Bound `tmux capture-pane -S` to a fixed line count.** Rejected — changes scrollback semantics (less history saved than the user expects) and is the wrong layer for this fix.
- **Move per-pane work onto a goroutine with a cancellable channel.** Rejected — heavier than the in-line `ctx.Done()` check, introduces new concurrency surface, doesn't improve worst-case exit latency over the inline approach.

### Change 3 — Debug breadcrumb on `daemon.version` writes

**Target:** `internal/state` — the package that owns `WriteVersionFile`.

**Required behaviour:**

Add a single DEBUG-level log entry inside `state.WriteVersionFile` capturing:

- The version string being written
- The caller's pid (`os.Getpid()`)
- The destination path

No behavioural change. No additional file I/O, no new error paths, no return-shape changes. Pure instrumentation.

**Why it ships now:** Defect 3 (`daemon.version` keeps disappearing on the affected user's machine) was not pinned to a production code path by the investigation. Fixing Defect 1 makes the disappearance non-load-bearing for the user-visible symptom, but the underlying mechanism remains unknown. The breadcrumb provides a paper trail in `portal.log` so the next disappearance — if it recurs — can be correlated against daemon lifecycle events without launching a fresh investigation.

**Acceptance for this change alone:**

- Every call to `WriteVersionFile` produces exactly one DEBUG log line.
- Existing call sites are unchanged in their semantics.
- No new test surface required — the existing tests for `WriteVersionFile` must remain green.

**Not in scope for this change:**

- Reproducing the disappearance. The investigation could not, and this spec does not commit to doing so.
- Identifying the deleter. The breadcrumb gives instrumentation for future diagnosis; it does not itself answer the question.

### Acceptance Criteria

The fix is complete when **all** of the following observable conditions hold:

#### Steady-state bootstrap

1. **No kill-respawn on healthy bootstrap.** On any bootstrap where the daemon is alive (regardless of `daemon.version`'s presence or readability), the three WARN lines are absent from `portal.log`:
   - `prior daemon (pid=N) did not exit within 5s`
   - `another daemon holds the lock; exiting`
   - `step 4 (EnsureSaver) failed: … no such session: _portal-saver`

2. **`_portal-saver` survives bootstrap.** Immediately after `portal hooks list` (or any bootstrap-only command) completes, `tmux has-session -t _portal-saver` returns success. The session remains running with the daemon process attached.

3. **Single live daemon, no orphans.** `pgrep -f "portal state daemon"` returns exactly one PID after bootstrap, and that PID matches the one holding `daemon.lock` (verifiable via `lsof daemon.lock`).

4. **`daemon.version` is repaired defensively.** If a bootstrap encounters "alive daemon + absent `daemon.version`", the file exists after bootstrap completes and contains the current binary version.

5. **~520ms reclaimed.** Wall-time of `portal hooks list` against an already-running tmux server with a healthy saver no longer includes the kill-respawn block. (Informational — not a hard regression test, but the developer should verify the improvement empirically.)

#### Version-upgrade bootstrap

6. **Real version mismatch still triggers kill.** When `daemon.version` reads cleanly and disagrees with the current binary (and neither side is `dev`/empty), `killSaverAndWaitForDaemon` runs and a fresh daemon spawns. The prior-bugfix kill-respawn path is preserved end-to-end.

#### Daemon responsiveness

7. **SIGHUP-to-exit latency is bounded by one pane's capture wall time.** When `tmux kill-session -t _portal-saver` is issued mid-tick on a real-tmux fixture with many panes, the daemon process exits within "one pane's `capture-pane` wall time" of receiving SIGHUP — empirically verifiable inside the 5s `killBarrierTimeout` on the affected user's profile.

8. **No partial commits on cancellation.** When the daemon is cancelled mid-tick, no half-applied scrollback writes or partial commits land on disk. Either the tick committed fully before cancellation, or it abandoned cleanly.

#### Diagnostic & regression

9. **`daemon.version` writes produce a DEBUG breadcrumb.** Each `state.WriteVersionFile` call emits one DEBUG log line containing version, caller pid, and destination path.

10. **No regression of `multiple-state-daemons-running-concurrently`.** All tests from the prior bugfix remain green. The `daemon.lock` flock and `killSaverAndWaitForDaemon` barrier are unchanged.

### Testing Requirements

#### Unit tests

**`portalSaverVersionMismatch` table tests** — the existing test pinning false-positive behaviour must be **replaced** (it codifies the bug as contract). New cases:

| Stored | Current | Read error | Expected |
|---|---|---|---|
| `0.5.0` | `0.5.0` | nil | `false` (match) |
| `0.5.0` | `0.5.1` | nil | `true` (real mismatch) |
| `""` | `0.5.0` | `ErrVersionFileAbsent` | `true` (predicate alone — alive-check happens in caller) |
| `""` | `0.5.0` | other I/O error | `true` |
| `dev` | `0.5.0` | nil | `true` (dev preserved) |
| `0.5.0` | `dev` | nil | `true` (dev preserved) |

**`EnsurePortalSaverVersion` ordering tests:**

- Assert `BootstrapAliveCheck` is consulted before any kill decision.
- Alive + absent version file → no kill, daemon.version written defensively.
- Alive + readable + matching version → no kill.
- Alive + readable + mismatching version → kill barrier runs.
- Not alive → no kill regardless of version state.

**`captureAndCommit` ctx-cancellation tests:**

- Cancel context before first per-pane iteration → early return, no commits.
- Cancel context between per-pane iterations on a multi-pane fixture → early return, no partial commits written.
- Uncancelled context → identical behaviour to current implementation (regression guard for the happy path).

#### Integration tests (real-tmux fixture)

1. **"Alive daemon, daemon.version absent, versions match"** → bootstrap completes without firing the kill barrier. `_portal-saver` survives; `daemon.version` is present and correct post-bootstrap. Pins Defect 1's user-visible contract.

2. **"Daemon mid-tick, SIGHUP arrives"** → on a fixture with multiple panes loaded with synthetic scrollback, send SIGHUP while a tick is in progress. The daemon process exits within a bounded window (target: under 2s on the test fixture). Pins Defect 2's responsiveness contract.

3. **"Lock-loser daemon's pane exit destroys `_portal-saver` session"** → force the lock contention scenario (spawn a second daemon while the first holds the lock) and assert the chain: lock-loser exits → pane process terminates → `_portal-saver` session is destroyed → `SetSessionOption` fails with "no such session". Closes synthesis-flagged gap from investigation: confirms the cascade is what we believe before the fix lands.

#### Regression preservation

- All tests from `multiple-state-daemons-running-concurrently` remain green. Listed by the planning phase via `git log --all --grep=multiple-state-daemons-running-concurrently` if needed.
- The existing `portal_saver_integration_test.go` "kill-respawn under explicit version mismatch" test stays green (Criterion 6 protects this path).

#### Out of testing scope

- **Reproducing the `daemon.version` disappearance.** Defect 3 is instrumentation-only; no test asserts the file is never deleted.
- **Empirical ~520ms perf measurement.** Informational acceptance criterion (#5), not a guarded regression test — measurement floor varies by machine.

### Out of Scope

The following are explicitly **not** part of this bugfix and should not be addressed by planning or implementation:

1. **Hook-registration redundancy.** `internal/tmux/hooks_register.go` runs ~1.5s of redundant `tmux show-hooks` work during bootstrap step 2 (RegisterPortalHooks). Orthogonal mechanism, orthogonal symptom (no orphan leak, no save pause). Logged separately at `.workflows/.inbox/bugs/2026-05-18--redundant-show-hooks-during-bootstrap-hook-registration.md`. Bundling would muddy review scope.

2. **`tmux capture-pane` line-count bounding.** Capping per-pane capture work via `capture-pane -S -N` is the wrong layer for this fix and would change scrollback semantics (less history saved than the user expects). Worth a separate scoping discussion if per-pane wall time becomes a problem in its own right.

3. **Raising `killBarrierTimeout` above 5s.** Considered and rejected — defers the next profile-growth failure without fixing the underlying contract (daemon must exit promptly on SIGHUP). The structural fix in Change 2 makes the timeout-sizing question moot.

4. **Identifying the root cause of Defect 3 (`daemon.version` disappearance).** No production code path was found that removes the file individually. Change 3's breadcrumb provides instrumentation for future diagnosis if the file disappears again — but reproducing or pinning the deleter is not in scope. If the breadcrumb captures evidence of a recurring deleter in production logs, a separate investigation is launched then.

5. **Goroutine-based concurrency restructure of the daemon tick loop.** Considered and rejected in favour of the simpler inline `ctx.Done()` check. Out of scope.

6. **TUI loading-floor (`LoadingMinDuration = 1.2s`) reduction.** Contributes to user-perceived startup time on the TUI path but is unrelated to the kill-respawn loop and orphan leak. Independent of this bugfix.

7. **Migration of `daemon.version` to a different storage mechanism** (e.g. server option, sqlite, etc.). The file-based approach is fine once Defect 1 makes the kill decision resilient to its transient absence.

### Risk & Rollout

#### Fix complexity

- **Change 1:** ~30 lines across `internal/tmux/portal_saver.go` — predicate refactor + alive-check threading + defensive `WriteVersionFile` call on the survived path.
- **Change 2:** ~20 lines in `cmd/state_daemon.go` plus signature updates that may propagate into `internal/state/capture.go` per-pane callers.
- **Change 3:** ~1 line — a `logger.Debug(...)` call inside `state.WriteVersionFile`.
- **Test updates** dominate the diff (table-test expansion, new integration fixtures).

#### Regression risk

**Low.** The fix is local refactors of decision logic (Change 1) and loop interruptibility (Change 2). Specifically:

- The `daemon.lock` flock primitive stays unchanged.
- The `killSaverAndWaitForDaemon` polling loop stays unchanged.
- The version-mismatch alive-check ordering is **additive** — it gates the existing kill branch with one new condition; the no-daemon path is unchanged.
- `BootstrapPortalSaver` itself is unchanged.
- The per-tick capture algorithm is unchanged; only the loop becomes interruptible.

Main risk surfaces to watch during review:

- The defensive `WriteVersionFile` from bootstrap on the "alive + absent" path must not race with the daemon's own `WriteVersionFile`. The bootstrap write happens before `BootstrapPortalSaver` proceeds; the daemon's own write happens after lock acquisition. They don't overlap on the survived-daemon path because the daemon's already past that point. Confirm via the integration test.
- Ctx cancellation between per-pane iterations must not introduce a deadlock where the shutdown flush waits for a tick that just got cancelled. Verify that `daemonShutdownFunc` does not depend on the cancelled tick's output.

#### Rollout

**Regular release.** No hotfix required:

- The symptom is recoverable (orphan daemons can be killed manually; `daemon.version` is repaired on next clean bootstrap once the fix lands).
- Impact is performance + silent save pause rather than data corruption.
- Behaviour-changing surface is small and well-tested at unit + integration level.

#### Coordination with prior bugfix

- Builds directly on `multiple-state-daemons-running-concurrently` (completed 2026-05-11). That spec's `daemon.lock` and `killSaverAndWaitForDaemon` machinery is treated as a frozen prior layer. Implementation must not touch the lock primitive or the barrier's polling loop.
- The prior spec sized the barrier timeout to a measured 3.9s + margin. This spec doesn't resize the timeout — it makes the daemon-side contract correct so the existing timeout never has to be exceeded under healthy conditions.

---

## Working Notes

