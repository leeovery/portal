# Implementation Review: Multiple State Daemons Running Concurrently

**Plan**: multiple-state-daemons-running-concurrently
**QA Verdict**: Approve

## Summary

All 13 plan tasks across 4 phases were verified independently against their acceptance criteria and the specification. Every task returned STATUS: Complete with 0 blocking findings. The structural bugfix — daemon-side `flock` singleton (Phase 1) plus synchronous kill barrier (Phase 2) — is implemented correctly and conforms to the spec's "N ≤ 1 daemons per state directory" invariant. The load-bearing real-tmux integration test asserts `pgrep -P <server-pid> -f 'portal state daemon' | wc -l == 1` after a forced version-mismatch recycle. Test coverage is adequate at all three tiers (unit / integration / regression) without over-testing. Analysis cycles 1 and 2 cleanly closed identified coverage and consistency gaps. Implementation is ready to ship.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across every reviewed surface:

- **Fix Part 1 (Daemon-Side Singleton Lock)** — `AcquireDaemonLock` in `internal/state/daemon_lock.go` opens `<stateDir>/daemon.lock` mode 0600, runs `unix.Flock(LOCK_EX|LOCK_NB)` via the `lockAcquire` seam, distinguishes EWOULDBLOCK (sentinel `ErrDaemonLockHeld`) from fatal open errors, and sets `FD_CLOEXEC`. Wired into `cmd/state_daemon.go:RunE` between `os.Remove(state.SaveRequested(dir))` and `state.WritePIDFile` with fd retained in package-level `daemonLockFile` for process lifetime. Contention → WARN + exit 0; other errors → ERROR + non-zero exit.
- **Fix Part 2 (Synchronous Kill Barrier)** — `killSaverAndWaitForDaemon` helper in `internal/tmux/portal_saver.go` reads the prior PID before issuing `KillSession`, polls `IsProcessAlive` at 50 ms cadence bounded by a 5 s timeout, returns silently on clean exit, emits one WARN on timeout. Both kill call sites (`EnsurePortalSaverVersion` version-mismatch + `BootstrapPortalSaver` stale-pidfile) route through the shared helper via the `killSaverAndWaitForDaemonFn` seam. Production `*state.Logger` is wired via `SetBarrierLogger` from `bootstrapadapter.HookRegistrar.RegisterPortalHooks` (Step 2, before Step 4 EnsureSaver).
- **Observability** — exactly two new WARN-class log lines across the bug surface (lock contention + barrier timeout). Steady-state path emits no new logs.
- **CLAUDE.md** — state package row updated to note the `daemon.lock` singleton primitive and lock-fd retention contract.

### Plan Completion

- [x] Phase 1 (Daemon-Side Singleton Lock) — 4/4 tasks complete
- [x] Phase 2 (Synchronous Kill Barrier and Singleton Invariant Verification) — 3/3 tasks complete
- [x] Phase 3 (Analysis Cycle 1) — 5/5 tasks complete
- [x] Phase 4 (Analysis Cycle 2) — 1/1 task complete
- [x] No scope creep — out-of-scope items (scrollback bounding, `portalSaverVersionMismatch` tightening, tick-loop restructure) correctly deferred

### Code Quality

No issues found.

- Seam-pattern conventions matched throughout (`daemonRunFunc` / `BootstrapAliveCheck` style); no new test-pattern departures.
- Idiomatic Go: `errors.Is` / `%w` wrapping, `time.NewTicker` + `defer ticker.Stop()`, structural interface satisfaction (`*state.Logger` satisfies `BarrierLogger`).
- Single-responsibility helpers (`captureTmuxServerPID`, `countDaemonChildren`, `buildPortalBinaryInto`).
- Doc comments document the load-bearing fd-retention contract and barrier control-flow explicitly.

### Test Quality

Tests adequately verify requirements at all three tiers without over-testing.

- **Unit (daemon lock)** — every spec-required case covered in `internal/state/daemon_lock_test.go` (EWOULDBLOCK → sentinel, non-EWOULDBLOCK wrapped, open error wrapped, mode 0600, FD_CLOEXEC, no stateDir creation, arbitrary stateDir).
- **Unit (kill barrier)** — every spec-required case covered in `internal/tmux/portal_saver_test.go` (PID dies within timeout / never dies / absent / corrupted / unreadable / already-dead; tolerates failing KillSession; does not mutate state directory; SetBarrierLogger routes WARN).
- **Integration** — `TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle` builds the real portal binary, runs back-to-back recycles via on-disk `daemon.version` overwrite (no new seam), asserts `pgrep -P <post-recycle-server-pid> -f 'portal state daemon'` count == 1.
- **Regression** — `TestAcquireDaemonLock_KernelReleasesOnFDClose` exercises real `unix.Flock` to lock down kernel-cleanup-on-abrupt-exit semantics.
- **Flock-loser recovery** — both aftermath shapes (empty session, dead-pane-under-remain-on-exit) covered at unit-seam level.
- All tests use per-test `t.TempDir()` isolation; no `t.Parallel()` anywhere.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. [3.4] Comment at `cmd/state_daemon_test.go:582-583` reads "ERROR is above the default INFO threshold" — the default threshold is WARN, not INFO. Two-character fix.
2. [1.1] `_ = f.Close()` repeated at lines 64 and 72 of `internal/state/daemon_lock.go` in error paths. Acceptable as-is; current form is more explicit than alternatives.
3. [1.2] `daemonLockFile` doc comment paraphrases the spec's "Fd retention is load-bearing" — quoting verbatim would tighten the link.
4. [3.5] `withDaemonLockFileReset` lives in `state_daemon_test.go:423` but is consumed by `state_test.go` and `version_guard_test.go`. Works because all share `package cmd`; cross-file dependency worth noting if helper is ever moved.

### Ideas

5. [2.1] Add a dedicated test for the degenerate-config edge case `killBarrierPollInterval == killBarrierTimeout` (Task 2.1 Edge Cases bullet 4).
6. [2.1] The never-dies test's upper-bound assertion `elapsed < 1*time.Second` (with timeout=20ms) is loose. Tightening to e.g. `< 200*time.Millisecond` would catch double-wait regressions.
7. [1.3] Optional strengthening: a subprocess-based `TestAcquireDaemonLock_KernelReleasesOnSIGKILL` variant would test SIGKILL literally rather than via `Close()` simulation. Task explicitly defers this as a strength bonus.
8. [1.4] Both new flock-loser tests pass literal `"/tmp/portal-state"` to `BootstrapPortalSaver`. With Phase 2's `killSaverAndWaitForDaemonFn` wiring, the dead-pane test's correctness depends on `/tmp/portal-state/daemon.pid` not existing at test-run time. Safer: pass `t.TempDir()` or install `tmux.KillSaverAndWaitForDaemonFnSeam()` to a recording stub.
9. [2.2] `SetBarrierLogger` nil-guard does not catch a typed-nil `*state.Logger` boxed inside the BarrierLogger interface. No bug today because `(*state.Logger).Warn` has its own nil-receiver guard.
10. [2.2] `time.NewTicker(killBarrierPollInterval)` delays the first probe by one full pollInterval after `KillSession`. Median recycle eats ~50ms tax even on sub-ms SIGHUP propagation. Inherited from Task 2.1, accepted.
11. [2.3] `waitForNewLiveDaemon` short-circuits on `pid != prior && IsProcessAlive(pid)`. Subsequent `state.IsProcessAlive(priorPID)` check is the actual singleton gate; early return is benign — a one-line comment would aid future maintainers.
12. [2.3] Test relies on env inheritance through `tmux → daemon`. If `tmuxtest.New` were ever made explicit-env, this test would silently lose its `PORTAL_STATE_DIR`.
13. [2.3] `countDaemonChildren` hardcodes argv string `"portal state daemon"`. A future rename of `portalSaverCommand` would silently make this test pass with count=0.
14. [3.1] In `dumpDiagnostics` the `serverPID` parameter is the post-recycle PID at line 245 but the pre-recycle PID at lines 211/215. Consider clarifying label as "tmux server PID (at capture time)".
15. [3.1] The "hand-mutated kill-barrier skip" verification step is a manual implementer check. Encoding the kill-barrier-skip negative case as a separate test would harden long-term confidence.
16. [3.4] Test asserts daemon.pid absence but not daemon.version absence on ERROR path. The contention-path sibling checks both. Strictly redundant; symmetry would be marginally cleaner.
17. [3.5] The cmd package now has several `with*` test helpers following the same prev/restore pattern. If this surface grows, a generic `swapPkgVar[T](t, &target, value)` could collapse them.
