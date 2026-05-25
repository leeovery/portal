TASK: 4-6 — Add pre-acquire daemon.pid liveness check to AcquireDaemonLock

STATUS: Complete

SPEC CONTEXT: Component C step 1 — `flock` excludes per-inode, not per-path. Pre-acquire `daemon.pid` check uses stable file with `state.IdentifyDaemon` as identity gate. Transient identity errors must NOT block startup; flock EWOULDBLOCK is safety net.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/daemon_lock.go:115-176` (AcquireDaemonLock), `:189-207` (preAcquirePIDIdentifiesLiveDaemon), `:28-39` (seams)
- Pre-check (line 121) runs at head of EVERY retry of the 4-7 outer inode-cross-check loop — exceeds minimum spec, justified by inline comment ("slow daemon that wins lock mid-retry is still detected next iteration")
- Helper `preAcquirePIDIdentifiesLiveDaemon` single responsibility; only `IdentifyIsPortalDaemon` with `err == nil` returns true
- `ReadPIDFile` error collapsed to single "no holder" branch covering `ErrPIDFileAbsent`, parse errors, arbitrary I/O; docstring explicit
- Transient `IdentifyDaemon` error → false (lines 200-205); spec bias toward legitimate succession
- Two seams (`lockAcquireReadPIDFile`, `lockAcquireIdentifyDaemon`) following existing convention
- Extensive docstring documents both `ErrDaemonLockHeld` return paths and layered-enforcement contract

TESTS:
- Status: Adequate
- Coverage: 8 pre-check tests in `daemon_lock_test.go`:
  - PID file absent / dead PID / live portal daemon → ErrDaemonLockHeld (asserts daemon.lock NOT created) / live non-portal / transient identity error / non-absent read error / no daemon.lock opened on refusal / EWOULDBLOCK pre-check sees no holder (layered enforcement regression guard)
- Helpers snapshot+restore seams via `t.Cleanup`; no `t.Parallel`
- Real `WritePIDFile` for happy path opportunistically exercises on-disk parse

CODE QUALITY:
- Project conventions: Followed; seam pattern mirrors `lockAcquire`
- SOLID: Good; helper is clean SRP boolean
- Complexity: Low; helper ~10 lines, 2 branches
- Modern idioms: package-level var function seams
- Readability: Good; unusually thorough docstrings citing spec rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `preAcquirePIDIdentifiesLiveDaemon` discards distinction between "no daemon.pid" and "non-daemon PID"; future Component-F WARN on stale PID would need tri-state
- [idea] Pre-check runs at head of every retry — up to 3× ReadPIDFile + IdentifyDaemon on persistent mismatch path
- [quickfix] "Unreachable" comment at lines 173-175 reads weaker than "compiler-required terminator; loop always returns"
- [idea] No AST-walk test pinning call-order ReadPIDFile → IdentifyDaemon → (return | OpenFile); structurally equivalent via fail-fast seams
