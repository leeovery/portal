# Review Report — Task 1.1

TASK: Add seam-injectable flock helper for daemon.lock

STATUS: Complete
FINDINGS_COUNT: 0

SPEC CONTEXT:
Per spec § Fix Part 1: helper must use `unix.Flock(LOCK_EX|LOCK_NB)`, mode 0600, not create `<stateDir>`, distinguish EWOULDBLOCK from fatal open(2) errors via an exported sentinel, set `FD_CLOEXEC` on the returned fd, and seam the flock call via a package-level `var` matching `daemonRunFunc` / `daemonShutdownFunc`. Fd retention is load-bearing.

IMPLEMENTATION:
- Status: Implemented
- Files:
  - `internal/state/daemon_lock.go:16` — `ErrDaemonLockHeld` sentinel
  - `internal/state/daemon_lock.go:25` — `lockAcquire = unix.Flock` seam
  - `internal/state/daemon_lock.go:55-77` — `AcquireDaemonLock`
  - `internal/state/paths.go:19` — `daemonLockName` const
  - `internal/state/paths.go:84` — `DaemonLock(dir)` accessor
- Notes:
  - `os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)` at L58 — correct flags/mode, no `MkdirAll`.
  - `lockAcquire(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)` at L63 — exclusive non-blocking via seam.
  - EWOULDBLOCK branch (L65-67) closes fd, returns sentinel; non-EWOULDBLOCK wrapped with `%w`.
  - FD_CLOEXEC asserted via `unix.FcntlInt(f.Fd(), unix.F_SETFD, unix.FD_CLOEXEC)` at L71; on fcntl failure helper closes + wraps (treats invariant as load-bearing).
  - Doc comment L31-54 documents fd-retention contract end-to-end.
  - FIFO read-only confirmation: `SweepOrphanFIFOs` (`internal/state/fifo_sweep.go`) is the only FIFO write path and only runs from `cmd/bootstrap/` (single-shot per process). Daemon-side FIFO interaction confirmed read-only.

Acceptance criteria check (all met):
- `AcquireDaemonLock(stateDir)` exists, takes parameter — yes (L55).
- Path resolves to `<stateDir>/daemon.lock`, accessor exposed — yes (`paths.go:84`).
- Mode 0600 — yes (L58).
- No `MkdirAll` — confirmed.
- EWOULDBLOCK → `ErrDaemonLockHeld` via `errors.Is` — yes (L65).
- Open(2) errors wrapped, not matching sentinel — yes (L60).
- FD_CLOEXEC set on returned fd — yes (L71).
- `unix.Flock` via `lockAcquire` seam — yes (L25, L63).
- No new test pattern departures — confirmed.
- FIFO-sweep paths read-only — confirmed.
- No `t.Parallel()` — confirmed.

TESTS:
- Status: Adequate
- File: `internal/state/daemon_lock_test.go`
- Coverage map vs spec required cases:
  - EWOULDBLOCK → sentinel: `TestAcquireDaemonLock_ReturnsErrDaemonLockHeldOnEWOULDBLOCK` (L22)
  - Non-EWOULDBLOCK wrapped distinct from sentinel: `TestAcquireDaemonLock_WrapsNonEWOULDBLOCKFlockErrors` (L38)
  - Open(2) error wrapped, not sentinel: `TestAcquireDaemonLock_WrapsOpenErrorWhenStateDirMissing` (L60)
  - Mode 0600: `TestAcquireDaemonLock_CreatesLockFileWithMode0600` (L85)
  - FD_CLOEXEC: `TestAcquireDaemonLock_SetsFDCLOEXEC` (L105)
  - Does not create stateDir: `TestAcquireDaemonLock_DoesNotCreateStateDirIfMissing` (L124)
  - Arbitrary stateDir: `TestAcquireDaemonLock_AcceptsArbitraryStateDirParameter` (L195)
  - Bonus: `TestAcquireDaemonLock_KernelReleasesOnFDClose` (L153) — Task 1.3 regression test, located here per Task 1.3 planning preference.
- Under-tested: none.
- Over-tested: none.
- Patterns: `t.TempDir()` per test, no `t.Parallel()`, `withLockAcquireFake` restores via `t.Cleanup`.

CODE QUALITY:
- Project conventions: Followed. DI/seam pattern matches `daemonRunFunc`/`daemonShutdownFunc`. Path accessor family extended in `paths.go` alongside existing `DaemonPID`/`DaemonVersion`.
- SOLID: Good. Single responsibility, `stateDir` parameter (no env), no coupling to cobra/logger.
- Complexity: Low. Linear flow, three error branches.
- Modern idioms: Yes. `errors.Is` + `%w` wrapping, `os.OpenFile` octal literal.
- Readability: Good.
- Security: No issues. Mode 0600. No shell-out.
- Performance: Sub-ms.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Task 1.3's regression test (`TestAcquireDaemonLock_KernelReleasesOnFDClose`) is implemented in Task 1.1's test file by deliberate planning-aligned choice.
- [idea] `daemon_lock.go` imports `golang.org/x/sys/unix` directly — darwin/linux-only, consistent with spec scope and existing `signal_hydrate.go`.
- [quickfix] `_ = f.Close()` appears at L64 and L72 in error paths. Acceptable Go idiom for fail-closed cleanup; current form is more explicit than alternatives.
