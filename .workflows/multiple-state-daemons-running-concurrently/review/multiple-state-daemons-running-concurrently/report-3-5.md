# Review Report — Task 3.5

TASK: Reset daemonLockFile package var in every cmd-package test that runs the real lock path

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- Tests at lines 47, 65, 88, 112, 151, 174, 196, 355 add `withDaemonLockFileReset(t)`.
- Tests at 419/451/601 already correct (no change).
- daemonLockFile reset between tests.
- `go test ./cmd/...` passes.
- Future tests would not observe leaked package-var state.

SPEC CONTEXT:
Spec § Fix Part 1 mandates the daemon retain its lock fd in a package-level var (`cmd/state_daemon.go:61`) for the lifetime of the process. Because cmd-package tests run multiple daemon RunE invocations in the same process, the package var leaks across tests. A stale non-nil value in test N can mask a lock-acquisition regression in test N+1.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_test.go`
  - Helper definition: lines 420-428
  - Reset calls inserted at lines 51, 75, 104, 132, 161, 186, 205, 263, 298, 375, 397
- Pre-existing / sibling-task calls confirmed at lines 434, 467, 489, 516, 547, 587, 646, 669
- Cross-file call sites: `cmd/state_test.go:182`, `cmd/version_guard_test.go:152`, `cmd/state_daemon_run_test.go:594, 642, 670, 709`
- Pre-edit → post-edit line mapping: 47→47, 65→66, 88→90, 112→115, 151→155, 174→179, 196→202, 355→364. All 8 named insertions present.
- Correctly excluded tests:
  - `TestStateDaemon_DefaultRunReturnsOnContextCancel` calls `defaultDaemonRun` directly, bypassing RunE.
  - `TestStateDaemon_ReturnsErrorWhenStateDirNotWritable` fails at `EnsureDir` before `acquireDaemonLock`.
  - Tick / shutdown-flush isolation tests in `state_daemon_run_test.go` do not invoke `runStateDaemon`.

TESTS:
- Status: Adequate (this task is itself a test-hygiene fix).
- Coverage: Every cmd-package test that exercises the real RunE lock-acquisition path resets the package var.
- Helper semantics: `prev := daemonLockFile; daemonLockFile = nil; t.Cleanup(func() { daemonLockFile = prev })` — symmetric save/restore.
- No over-testing.

CODE QUALITY:
- Project conventions: Followed. Matches established cmd-package DI pattern (`t.Cleanup` restore).
- Complexity: Trivial — 3-statement body, clear intent.
- Modern idioms: Idiomatic Go `t.Helper` + `t.Cleanup`.
- Readability: Helper has a load-bearing doc comment naming the failure mode it prevents.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The cmd package now has several `with*` test helpers following the same prev/restore pattern. If this surface grows, a generic `swapPkgVar[T](t, &target, value)` could collapse them — not worth doing today.
- [idea] `TestStateDaemon_PassesPreparedDepsToRunFunc` (line 202) calls `withDaemonLockFileReset(t)` but does not stub `daemonRunFunc` until after — correct but subtle.
- [quickfix] `withDaemonLockFileReset` lives in `state_daemon_test.go:423` but is consumed by `state_test.go` and `version_guard_test.go`. Works because all share `package cmd`; noting cross-file dependency.
