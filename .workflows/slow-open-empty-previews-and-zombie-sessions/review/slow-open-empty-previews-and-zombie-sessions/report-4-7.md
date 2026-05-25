TASK: 4-7 — Add post-flock fstat-vs-stat inode cross-check with bounded retry

STATUS: Complete

SPEC CONTEXT: Component C step 3 — secondary defence against unlink+recreate races between open(2) and flock(2). 3-attempt bound, fixed 10ms sleep, no jitter. Persistent mismatch → wrapped error → daemon exits status 1 (distinct from ErrDaemonLockHeld exit 0).

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/daemon_lock.go:115-176` (AcquireDaemonLock); seams at 41-49; constants at 51-59
- Loop `for attempt := 1; attempt <= lockAcquireInodeRetryAttempts; attempt++` (line 118)
- Pre-check (4-6) hoisted INTO loop at line 121
- fstat (144), stat (149) — both: close fd + wrapped err immediately on syscall error
- Mismatch branch (154-163): close fd, sleep 10ms, continue; final attempt returns wrapped error
- FD_CLOEXEC (165) only on match path
- Wrapped error distinct from ErrDaemonLockHeld; `errors.Is` returns false

TESTS:
- Status: Adequate
- Coverage in `daemon_lock_test.go` — all 10 plan-listed:
  - HappyPath (464), MismatchThenMatch (500), ExhaustsRetries (536), FstatSyscallError (577), StatSyscallError (614), MismatchClosesFDBeforeRetry (655), NoFDCLOEXECOnPersistentMismatch (705), RetryBoundedWallTime (734, <100ms), NotReachedOnEWOULDBLOCK (758), NotReachedOnPreCheckRefusal (783)
- Seams reset via `t.Cleanup`; `inodeSequence` helper for deterministic driving; no `t.Parallel`
- Close-call observation proxied via flock-count (plan-equivalent given no OpenFile/Close seam)

CODE QUALITY:
- Project conventions: Followed; matches existing `lockAcquire` seam pattern
- SOLID: Good; single responsibility; seams isolate side effects
- Complexity: Acceptable; moderate branching, each branch well-commented
- Modern idioms: `unix.Stat_t`, `fmt.Errorf %w`, `errors.Is`
- Readability: Good; comments explain intent; constants documented

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Pre-check invoked at head of EVERY retry — spec Do-list most naturally maps to single pre-check before loop; author justified as detecting "slow daemon that wins lock mid-retry"; worth confirming with spec author
- [quickfix] Post-loop `return nil, fmt.Errorf(...)` at lines 173-175 is unreachable (final iteration's mismatch returns first); duplicate format string; convert to `panic("unreachable: bounded retry loop fell through")` or extract constant
- [idea] No integration test exercising real unlink+recreate race against real flock; seam-driven units prove logic but real-syscall race would catch seam/syscall drift (spec doesn't mandate)
- [idea] `TestAcquireDaemonLock_InodeCheck_MismatchClosesFDBeforeRetry` uses flock-call-count as proxy for fd-close observation; acceptable proxy
