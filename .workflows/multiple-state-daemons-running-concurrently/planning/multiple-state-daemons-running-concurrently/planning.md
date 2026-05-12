# Plan: Multiple State Daemons Running Concurrently

## Phases

### Phase 1: Daemon-Side Singleton Lock

status: approved
approved_at: 2026-05-11

**Goal**: Establish a structural N ≤ 1 invariant on the daemon-startup path. Acquire an exclusive non-blocking `flock` on `<stateDir>/daemon.lock` before any state-directory write, so no two `portal state daemon` processes can ever write the same state directory concurrently — regardless of how they were started or whether the bootstrap synchronisation in Phase 2 is in place.

**Why this order**: The lock is the floor that holds even if every other guard fails. It must land before the kill barrier (Phase 2) because Phase 2's failure mode (timeout, or concurrent bootstraps) relies on the lock as the safety net — without the lock, the barrier's WARN-and-proceed path would re-introduce the very corruption it is meant to prevent. Landing Part 1 first means Phase 2 can be merged without ever reopening the race window mid-implementation. It also gives the integration test in Phase 2 something real to assert against.

**Acceptance**:
- [ ] A seam-injectable lock acquisition runs before `state.WritePIDFile` on the daemon-startup path in `cmd/state_daemon.go`.
- [ ] On successful acquisition the lock fd is retained for the lifetime of the daemon process (cannot be GC'd, no finaliser closes it) and is marked `FD_CLOEXEC` so it does not leak to forked children.
- [ ] On `EWOULDBLOCK` (lock held) the daemon emits one WARN-level log line and exits status 0; `daemon.pid` is **not** overwritten by the loser.
- [ ] On `open(2)` failures other than contention (`EACCES`, `ENOSPC`, `ENOENT`, `EMFILE`, `ENFILE`), the daemon logs an ERROR-level line and exits non-zero.
- [ ] The lock file is created mode `0600`; the helper does **not** create `<stateDir>` itself.
- [ ] The lock helper accepts a state directory parameter (no hardcoded path), enabling per-test `t.TempDir()` isolation.
- [ ] Unit tests in `cmd/state_daemon_test.go` cover: acquire-succeeds (proceeds to `WritePIDFile` and tick loop), acquire-fails-EWOULDBLOCK (one WARN line, exit 0, pidfile unchanged on disk), FD_CLOEXEC asserted, and acquire-before-WritePIDFile ordering asserted via observable filesystem state (no new `WritePIDFile` seam introduced).
- [ ] Regression test confirms a daemon that crashes (abrupt exit / SIGKILL simulation) releases the lock via kernel fd cleanup so the next acquisition succeeds with no stale-lockfile dance.
- [ ] Flock-loser recovery test confirms that when a second daemon loses the lock and exits status 0 as the initial process of `_portal-saver`, the next bootstrap recovers via the tolerant-kill-and-recreate branch in `BootstrapPortalSaver`.
- [ ] `go test ./...` is green; no regressions in existing `BootstrapAliveCheck` / pidfile tests.
- [ ] Tests do not use `t.Parallel()`.

#### Tasks
status: approved
approved_at: 2026-05-11

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| multiple-state-daemons-running-concurrently-1-1 | Add seam-injectable flock helper for daemon.lock | EWOULDBLOCK surfaces as distinct sentinel, open(2) errors (EACCES/ENOSPC/ENOENT/EMFILE/ENFILE) wrapped, FD_CLOEXEC asserted on returned fd, lock file mode 0600, helper does not create stateDir, accepts stateDir parameter |
| multiple-state-daemons-running-concurrently-1-2 | Wire lock acquisition into daemon startup before WritePIDFile | Ordering asserted via observable filesystem state (no new WritePIDFile seam), EWOULDBLOCK → one WARN log + exit 0 + pidfile unchanged, open errors → ERROR log + non-zero exit, fd retained for daemon lifetime with no finaliser closure |
| multiple-state-daemons-running-concurrently-1-3 | Regression test: kernel releases lock fd on abrupt daemon exit | Abrupt-exit / SIGKILL simulation, no stale-lockfile dance required, next acquisition succeeds cleanly against real unix.Flock |
| multiple-state-daemons-running-concurrently-1-4 | Flock-loser recovery via tolerant-kill-and-recreate | Loser exits status 0 as initial process of _portal-saver, empty-session aftermath when tmux closes the window, dead-pane aftermath under remain-on-exit, next bootstrap converges via BootstrapPortalSaver |

### Phase 2: Synchronous Kill Barrier and Singleton Invariant Verification

status: approved
approved_at: 2026-05-11

**Goal**: Eliminate the bootstrap kill-respawn race by making the common-case recycle synchronous — the prior daemon's process is observed dead (or a bounded 5 s timeout elapses) before the new daemon is forked. Wire the shared barrier helper into both kill sites (`EnsurePortalSaverVersion` and `BootstrapPortalSaver`) and validate the composed system (lock + barrier) with the load-bearing real-tmux integration test asserting `pgrep` count == 1 after a back-to-back recycle.

**Why this order**: Builds on Phase 1's lock as its safety net for the timeout path. The barrier's purpose is to keep the **common case** silent — without it, every recycle would produce a WARN from Phase 1's lock-contention path. The integration test belongs here because it is the load-bearing assertion of the **composed** fix: it exercises both the Phase 1 lock and the Phase 2 barrier together, and only passes when both are correct. The spec explicitly identifies this integration test as the test that would have caught the bug in CI had it existed.

**Acceptance**:
- [ ] A shared `killSaverAndWaitForDaemon` helper (or equivalent) lives in `internal/tmux/portal_saver.go`, takes the tmux client + state directory, reads the prior PID via `state.ReadPIDFile`, issues `KillSession`, and polls `state.IsProcessAlive` until false or until a 5 s timeout elapses.
- [ ] Polling clock, `IsProcessAlive`, and `ReadPIDFile` are seamed for injection so tests complete without real processes or real waits.
- [ ] Both kill call sites use the helper: the version-mismatch branch in `EnsurePortalSaverVersion` (`internal/tmux/portal_saver.go:108-112`) and the stale-pidfile recovery branch in `BootstrapPortalSaver` (`internal/tmux/portal_saver.go:66-70`).
- [ ] On clean exit within timeout the helper returns silently (no log); on timeout it emits one WARN-level log line and proceeds (does not block indefinitely, does not return fatal).
- [ ] Defensive PID handling: missing, unreadable, empty, malformed (non-numeric), or already-dead PID skips polling and returns immediately with no log.
- [ ] Steady-state path is untouched: when the saver exists and the version matches, `EnsurePortalSaverVersion` does **not** invoke the barrier.
- [ ] Unit tests in `internal/tmux/portal_saver_test.go` cover: prior PID dies within timeout (no WARN), prior PID never dies (WARN + bounded wall time via injected clock), no prior PID file, dead prior PID, unreadable/corrupted PID file, and both call sites invoking the shared helper.
- [ ] New integration test in `internal/tmux/portal_saver_integration_test.go` (real-tmux fixture, skipped when tmux unavailable) writes a differing value into `<stateDir>/daemon.version` between two `EnsurePortalSaverVersion` calls and asserts `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l == 1` after both calls return. No new test seam is introduced for the version-mismatch comparison — the test exercises the real `portalSaverVersionMismatch` logic.
- [ ] No new logs are emitted on the common-case clean-handover path; the only WARN-class additions across the full fix surface are the two specified (lock contention, barrier timeout).
- [ ] `go test ./...` is green; the singleton invariant test passes; pidfile / `BootstrapAliveCheck` regression tests still pass.
- [ ] Tests do not use `t.Parallel()`.

#### Tasks
status: approved
approved_at: 2026-05-11

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| multiple-state-daemons-running-concurrently-2-1 | Add seam-injectable killSaverAndWaitForDaemon helper | Prior PID dies within timeout (no WARN), prior PID never dies (one WARN + bounded wall time via injected clock), missing PID file, dead prior PID, unreadable/empty/malformed PID file, polling clock + IsProcessAlive + ReadPIDFile all seamed for injection, 5 s timeout sized above 3.9 s cold-sweep ceiling, helper returns non-fatally on timeout |
| multiple-state-daemons-running-concurrently-2-2 | Wire barrier into both kill call sites (EnsurePortalSaverVersion + BootstrapPortalSaver) | Steady-state path untouched when version matches and saver alive (helper not invoked), both call sites verified to invoke shared helper via injection recorder, kill failures still tolerated as today, no behaviour change beyond barrier wait |
| multiple-state-daemons-running-concurrently-2-3 | Real-tmux integration test asserts singleton invariant after recycle | Skipped when tmux unavailable, exercises real portalSaverVersionMismatch (no new seam), daemon.version written directly between two EnsurePortalSaverVersion calls, pgrep -P <tmux-server-pid> -f 'portal state daemon' count == 1 asserted after both calls return, per-test t.TempDir() isolation, no t.Parallel() |

### Phase 3: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| multiple-state-daemons-running-concurrently-3-1 | Restore spec-mandated pgrep server-children assertion in singleton integration test | pgrep -P <serverPID> -f 'portal state daemon' returns exactly 1, existing alive/dead assertions retained, diagnostic message includes raw pgrep output on mismatch, hand-mutated kill-barrier skip confirms new assertion fails |
| multiple-state-daemons-running-concurrently-3-2 | Replace literal "bootstrap" string with state.ComponentBootstrap constant at kill-barrier WARN site | No literal "bootstrap" remains as log component arg in portal_saver.go or hooks_register.go, all three call sites reference state.ComponentBootstrap, internal/state already imported, constant value unchanged so log-matching tests still pass |
| multiple-state-daemons-running-concurrently-3-3 | Extract ProjectRoot + buildPortalBinary helpers from restoretest into an untagged file for default-lane test reuse | New build.go has no build tag and exports ProjectRoot + BuildPortalBinary, restoretest.go retains only integration-tagged surface, inlined helpers removed from portal_saver_integration_test.go, both default and integration lanes build, ~30 line net deletion |
| multiple-state-daemons-running-concurrently-3-4 | Cover the ERROR-level log assertion for non-contention lock-acquire failure | PORTAL_LOG_LEVEL=error set via t.Setenv, portal.log read post-run, exactly one line contains both "ERROR" and "acquire daemon lock", non-zero exit + no state writes still asserted, hand-mutated removal of ERROR log call confirms the new assertion fails |
| multiple-state-daemons-running-concurrently-3-5 | Reset daemonLockFile package var in every cmd-package test that runs the real lock path | Tests at lines 47, 65, 88, 112, 151, 174, 196, 355 add withDaemonLockFileReset(t), tests at 419/451/601 already correct (no change), daemonLockFile reset between tests, go test ./cmd/... passes, future tests would not observe leaked package-var state |

### Phase 4: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| multiple-state-daemons-running-concurrently-4-1 | Route pre-recycle tmux server PID capture through captureTmuxServerPID helper | Pre-recycle capture site routes through captureTmuxServerPID helper, serverPID variable name preserved for dumpDiagnostics call sites, no unused strconv/strings imports left behind, helper doc comment rationale becomes truthful, no behavioural change to test |
