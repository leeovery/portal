# Plan: Saver Kill-Respawn Loop Leaks Daemons

## Phases

### Phase 1: Alive-check gating in EnsurePortalSaverVersion + daemon.version breadcrumb
status: approved
approved_at: 2026-05-19

**Goal**: Eliminate the unnecessary kill-respawn cascade on healthy bootstrap by gating the kill decision on daemon aliveness before the version-mismatch predicate, writing `daemon.version` defensively on the survived path, and adding a DEBUG breadcrumb to every `state.WriteVersionFile` call.

**Why this order**: This phase resolves the user-visible symptoms — orphan daemon leak, ~520ms wasted per invocation, silent save pause between bootstraps, and the three-line WARN cascade in `portal.log`. The specification states explicitly that fixing Defect 1 makes Defects 2 and 3 "non-load-bearing for the user-visible symptom," so this phase is the foundation that subsequent work builds on. Change 3 (the `WriteVersionFile` breadcrumb) folds in here because its instrumentation flows through the same helper that Change 1's defensive bootstrap-side write calls — bundling avoids a second pass over `internal/state` and keeps a single grep anchor (`ComponentDaemon`) regardless of caller.

**Acceptance**:
- [ ] `portalSaverVersionMismatch` table tests cover all six rows in the specification's decision matrix (match, real mismatch, absent + neither dev, other I/O error, stored=dev, current=dev); test is reframed so its documentation no longer claims "absent counts as version mismatch" as load-bearing contract.
- [ ] `EnsurePortalSaverVersion` consults `BootstrapAliveCheck(stateDir)` before any kill decision; unit tests assert the ordering across all branches: alive+dev-either-side → kill, alive+absent → no kill + defensive write, alive+readable+match → no kill, alive+readable+mismatch (neither dev) → kill barrier runs, alive+read-error → kill, not-alive → no kill regardless of version state.
- [ ] On the "alive + absent `daemon.version`" branch, `EnsurePortalSaverVersion` writes `currentVersion` via `state.WriteVersionFile` before proceeding to `BootstrapPortalSaver`; the file exists synchronously after the call returns.
- [ ] The function comment at `internal/tmux/portal_saver.go:232-241` is updated to reflect the new contract — it no longer documents absence as mismatch.
- [ ] `state.WriteVersionFile` emits exactly one DEBUG log line per call under `state.ComponentDaemon`, prefixed `daemon.version write:` and containing version, caller pid (`os.Getpid()`), and destination path; existing `WriteVersionFile` tests remain green.
- [ ] Integration test "alive daemon, `daemon.version` absent, versions match" passes — bootstrap completes without firing the kill barrier, `_portal-saver` survives (`tmux has-session -t _portal-saver` returns success), `daemon.version` is present and contains `currentVersion` post-bootstrap, and the three WARN lines are absent from `portal.log`.
- [ ] `pgrep -f "portal state daemon"` returns exactly one PID after bootstrap on the healthy-daemon path, and that PID matches the holder of `daemon.lock` (verifiable via `lsof daemon.lock`).
- [ ] The existing `portal_saver_integration_test.go` "kill-respawn under explicit version mismatch" test stays green — real version mismatch (both sides non-dev, non-empty, disagreeing) still triggers `killSaverAndWaitForDaemon`.
- [ ] All tests from `multiple-state-daemons-running-concurrently` remain green; the `daemon.lock` flock primitive and `killSaverAndWaitForDaemon` polling loop are untouched.

#### Tasks
status: approved
approved_at: 2026-05-19

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| saver-kill-respawn-loop-leaks-daemons-1-1 | Reframe `portalSaverVersionMismatch` table tests to cover all six matrix rows | stored=dev, current=dev, ErrVersionFileAbsent, non-absent I/O read error |
| saver-kill-respawn-loop-leaks-daemons-1-2 | Add DEBUG breadcrumb to `state.WriteVersionFile` under `ComponentDaemon` | existing WriteVersionFile tests stay green, no new error paths, no I/O side effects beyond logging |
| saver-kill-respawn-loop-leaks-daemons-1-3 | Gate kill decision on `BootstrapAliveCheck` in `EnsurePortalSaverVersion` before mismatch predicate | alive+dev short-circuit (either side dev), alive+absent → no-kill, alive+read-error → kill, not-alive → no-kill regardless of version state |
| saver-kill-respawn-loop-leaks-daemons-1-4 | Defensive `WriteVersionFile(currentVersion)` on alive+absent branch before `BootstrapPortalSaver` | write failure surfaces as error, no race with daemon's own write on survived path, pathological older-binary alive case (defensive write asserts going-forward version) |
| saver-kill-respawn-loop-leaks-daemons-1-5 | Revise function comment at `internal/tmux/portal_saver.go:232-241` to match new contract | none |
| saver-kill-respawn-loop-leaks-daemons-1-6 | Integration test: alive daemon + absent `daemon.version` survives bootstrap (real-tmux fixture) | single live daemon PID matches `daemon.lock` holder, three WARN lines absent from `portal.log`, existing kill-respawn-under-explicit-mismatch integration test stays green |

### Phase 2: Context-aware `captureAndCommit` (daemon side of the kill-barrier contract)
status: approved
approved_at: 2026-05-19

**Goal**: Thread `ctx` from `defaultDaemonRun` through `tick` into `captureAndCommit` and the per-pane loop in `cmd/state_daemon.go`, so the daemon honours cancellation between per-pane iterations. This caps worst-case SIGHUP-to-exit latency at one pane's `capture-pane` wall time rather than the aggregate of all panes.

**Why this order**: Phase 1 eliminates the *natural* trigger of the kill-barrier-gives-up-early cascade (the kill-respawn path no longer fires on healthy bootstrap), but the daemon's structural non-responsiveness to context cancellation remains a latent bug — it still surfaces on legitimate version-upgrade recycles and under the recycle-induced sweep pressure documented in the specification's "self-amplifying property" note. Landing this phase second lets the user-visible fix ship independently of the (larger) structural change to the daemon's tick loop, and lets Phase 2's responsiveness contract be verified against the fault-injection harness Phase 1's tests already establish.

**Acceptance**:
- [ ] `ctx` is threaded from `defaultDaemonRun` through `tick` into `captureAndCommit` and its per-pane loop; all signature changes are local to `cmd/state_daemon.go` and `internal/state/capture.go` (including `CaptureStructure` and `CaptureAndHashPane`) is unmodified.
- [ ] `ctx.Done()` is observed at three points inside `captureAndCommit`: at function entry before pane enumeration, after enumeration before the first per-pane iteration, and between per-pane iterations.
- [ ] On cancellation, `captureAndCommit` returns early **without committing partial state** — no half-applied scrollback writes, no partial commits.
- [ ] Unit tests cover: cancel before first per-pane iteration → early return + no commits; cancel between iterations on a multi-pane fixture → early return + no partial commits; uncancelled context → identical behaviour to current implementation (happy-path regression guard).
- [ ] Integration test "daemon mid-tick, SIGHUP arrives" passes — on a real-tmux fixture with multiple panes loaded with synthetic scrollback, the daemon process exits within a bounded window after SIGHUP. The threshold (initially 2s heuristic) is confirmed or adjusted against a fresh wall-time measurement of one pane's `capture-pane` invocation taken during implementation.
- [ ] Integration test "lock-loser daemon's pane exit destroys `_portal-saver` session" passes via the fault-injection harness — a sentinel holding `daemon.lock` forces the lock-contention scenario, the new daemon exits within ~1s, `tmux has-session -t _portal-saver` returns failure, and the immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing `no such session`. The cascade chain remains observable via forced contention; only the natural trigger is eliminated.
- [ ] `killBarrierTimeout` remains at 5s; the `killSaverAndWaitForDaemon` polling loop is unchanged.
- [ ] `daemonShutdownFunc` does not depend on a cancelled tick's output — no deadlock between cancellation and the shutdown flush.
- [ ] Tests from `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, and `killed-sessions-resurrect-on-restart` all remain green.

#### Tasks
status: approved
approved_at: 2026-05-19

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| saver-kill-respawn-loop-leaks-daemons-2-1 | Thread `ctx` from `defaultDaemonRun` through `tick` into `captureAndCommit` (signature change + happy-path regression) | `defaultShutdownFlush` keeps calling `captureAndCommit` (uses `context.Background()` so non-cancellable shutdown flush is preserved), no signature changes propagate outside `cmd/state_daemon.go`, `internal/state/capture.go` untouched, `daemonShutdownFunc` does not depend on cancelled tick output |
| saver-kill-respawn-loop-leaks-daemons-2-2 | Add `ctx.Done()` check at `captureAndCommit` entry (pre-enumeration) with cancel-before-first unit test | already-cancelled ctx at entry → early return, no `ListSkeletonMarkers` call, no commit, no `PrevIndex` mutation, `LastSaveAt` unchanged |
| saver-kill-respawn-loop-leaks-daemons-2-3 | Add `ctx.Done()` check post-enumeration, pre-first-iteration with unit test | cancellation observed after `CaptureStructure` returns but before loop starts → early return, no per-pane work, no `Commit()`, no `PrevIndex` replacement |
| saver-kill-respawn-loop-leaks-daemons-2-4 | Add `ctx.Done()` check between per-pane iterations with cancel-mid-loop unit test on multi-pane fixture | mid-loop cancel after k panes processed → no `Commit()`, no `PrevIndex` replacement, scrollback writes already done by `WriteScrollbackIfChanged` for completed panes are not rolled back (per-pane writes are atomic; spec requires no *partial commit* of sessions.json, not rollback of per-pane scrollback files), `anyScrollbackChanged` discarded |
| saver-kill-respawn-loop-leaks-daemons-2-5 | Integration test: daemon mid-tick + SIGHUP exits within bounded window (real-tmux fixture, multi-pane synthetic scrollback) | threshold confirmed/adjusted from fresh wall-time measurement of one pane's `capture-pane`, recycle-induced sweep pressure (back-to-back `session-closed`/`session-created` hooks firing `save.requested`), exit bounded under 5s `killBarrierTimeout`, `killBarrierTimeout` stays at 5s |
| saver-kill-respawn-loop-leaks-daemons-2-6 | Fault-injection integration test: lock-loser daemon's pane exit destroys `_portal-saver` (cascade regression guard) | sentinel holds `daemon.lock` via `state.AcquireDaemonLock` in test goroutine, new daemon exits within ~1s, `has-session` poll (100ms tick, 2s ceiling) returns failure, `SetSessionOption(_portal-saver, destroy-unattached, off)` returns `exit status 1` containing `no such session`, regression-watch suites (`multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`) remain green |
