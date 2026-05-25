TASK: Fix T7-5 Stale Misleading Comment In TestStateDaemon_DoesNotWritePIDFileWhenLockHeld (T11-4)

ACCEPTANCE CRITERIA:
- Comment accurately describes post-T7-5 behaviour.
- Test still passes.
- Optional daemon.version stat assertion (if added) passes.

STATUS: Complete

SPEC CONTEXT: Cycle-1 remediation. T7-5 changed `defaultDaemonRun` ordering to `acquireDaemonLock → WritePIDFile → WriteVersionFile`, so a lock-held early-return short-circuits BOTH writes. A stale comment incorrectly claimed daemon.version IS written under lock contention. T4-8 AST adjacency test structurally pins the acquire-then-write ordering; this test spot-checks filesystem invariants.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon_test.go:540-552
- Notes: Comment fully rewritten (540-546) to describe correct post-T7-5 ordering and cross-reference T4-8 AST adjacency test. Optional symmetric `os.Stat` assertion for `daemon.version` added at 550-552, matching the daemon.pid shape directly above.

TESTS:
- Status: Adequate
- Coverage: Asserts both daemon.pid AND daemon.version absent post lock-contention exit. Symmetric assertion captures the actual T7-5 invariant rather than relying solely on AST adjacency.
- Notes: No over-testing — two stat calls mirror the two files that must not exist. Overlap with T4-8 is intentional defence-in-depth (AST pins structure, stat pins filesystem outcome).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; idiomatic `os.IsNotExist` guard matching existing daemon.pid block).
- SOLID principles: N/A (test code).
- Complexity: Low — two parallel stat blocks, clear intent.
- Modern idioms: Yes — `filepath.Join`, `os.IsNotExist` consistently used.
- Readability: Good — comment factually accurate, links T4-8.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
