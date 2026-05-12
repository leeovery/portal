# Review Report — Task 1.3

TASK: Regression test — kernel releases lock fd on abrupt daemon exit

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- [x] Test exists in `internal/state/daemon_lock_test.go` (spec-preferred location).
- [x] Uses real `unix.Flock` (no `lockAcquire` seam injection).
- [x] Asserts intermediate contention while first fd is held.
- [x] Asserts re-acquire succeeds after first fd is closed.
- [x] No manual unlink / stale-file dance.
- [x] `t.TempDir()` isolation; no `t.Parallel()`.
- [x] No platform skip; works on darwin + linux.

SPEC CONTEXT:
Spec § Acceptance Criteria → "Lock cleanup on crash" requires a regression test that simulates abrupt exit and confirms next acquisition succeeds without a stale-lockfile dance. Guards against future refactors that (a) install a premature-close finalizer or (b) swap `flock` for a lockfile-based primitive whose semantics leak on abrupt exit.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/daemon_lock_test.go:143-193` — `TestAcquireDaemonLock_KernelReleasesOnFDClose`
- Notes:
  - Spec-preferred location (helper-adjacent in `internal/state`, not `cmd`).
  - Three-step flow: acquire f1, attempt f2 (expect ErrDaemonLockHeld), close f1, acquire f3 (expect success).
  - `t.Cleanup(func() { _ = f3.Close() })` at line 192 prevents fd leak.
  - Docstring (lines 143-152) explains both the invariant and the two refactor footguns the test catches.

TESTS:
- Status: Adequate
- Coverage:
  - Intermediate contention: lines 166-173 (proves f1 is genuinely held before close).
  - Re-acquire after close: lines 185-191 (load-bearing assertion).
  - Real `unix.Flock`: confirmed.
  - No manual unlink: confirmed.
  - `t.TempDir()` / no `t.Parallel()`: confirmed.
- Notes:
  - SIGKILL subprocess variant not implemented; task explicitly defers it as a strength bonus.

CODE QUALITY:
- Project conventions: Followed.
- Complexity: Low — linear three-step flow.
- Modern idioms: `errors.Is`, `t.TempDir()`, `t.Cleanup`.
- Readability: Good. Block docstring is unusually thorough.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Optional strengthening: a subprocess-based SIGKILL variant would test SIGKILL literally rather than via `Close()` simulation. Task explicitly defers this; `Close()` is a faithful simulation per spec.
