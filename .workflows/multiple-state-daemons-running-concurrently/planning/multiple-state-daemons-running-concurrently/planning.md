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
