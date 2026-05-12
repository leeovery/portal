# Review Report — Task 1.2

TASK: Wire lock acquisition into daemon startup before WritePIDFile

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA (all met):
- AcquireDaemonLock called after `os.Remove(state.SaveRequested(dir))` and before `state.WritePIDFile` — verified at `cmd/state_daemon.go:241` (remove) → `:255` (acquire) → `:266` (WritePIDFile).
- `daemonLockFile *os.File` package-level retention — `cmd/state_daemon.go:52-61` with explicit load-bearing doc comment forbidding any finalizer that closes the fd.
- No `runtime.SetFinalizer` anywhere (grep clean).
- ErrDaemonLockHeld → WARN + `return nil` — `cmd/state_daemon.go:257-260`.
- Other errors → ERROR + wrapped non-nil return — `cmd/state_daemon.go:261-262`.
- No new WritePIDFile seam — ordering is asserted via observable filesystem state.
- CLAUDE.md updated — `CLAUDE.md:43` state-row notes `AcquireDaemonLock(stateDir)` / `ErrDaemonLockHeld` and `daemon.lock` singleton primitive.

SPEC CONTEXT:
Spec § Fix Part 1 — singleton lock must precede WritePIDFile; fd retention is load-bearing via package-level var; contention = WARN + exit 0; other errors = ERROR + non-zero; pidfile is never written by a loser; observability budget caps WARN lines at two across the bug surface (this task contributes one).

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon.go:46-50` (seam), `:52-61` (retention var), `:255-264` (acquire + branches), `:266` (WritePIDFile); `CLAUDE.md:43` (doc).
- Notes: Ordering is exactly as the spec demands. Comment block on `daemonLockFile` documents the finalizer-suppression contract.

TESTS:
- Status: Adequate
- Coverage (all in `cmd/state_daemon_test.go`):
  - `TestStateDaemon_AcquiresLockBeforeWritePIDFile` (430-460) — ordering via observable filesystem state (pidfile absent at seam entry).
  - `TestStateDaemon_AcquireLockCalledAfterEnsureDir` (462-484) — stateDir exists at seam entry.
  - `TestStateDaemon_ExitsCleanlyWhenLockHeld` (486-511) — `RunE` returns nil, daemonRunFunc not invoked.
  - `TestStateDaemon_DoesNotWritePIDFileWhenLockHeld` (513-542) — pidfile + version file absent.
  - `TestStateDaemon_DoesNotOverwritePIDFileWhenLockHeld` (544-579) — pre-seeded sentinel PID survives.
  - `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` (581-640) — wrapped sentinel returned, exactly one ERROR line.
  - `TestStateDaemon_RetainsLockFdAcrossDaemonLifetime` (642-661) — `daemonLockFile != nil` post-RunE, fd still open.
  - `TestStateDaemon_EmitsWarnOnLockContention` (663-701) — exactly one WARN line.
  - All existing happy-path tests augmented with `withDaemonLockFileReset(t)`.
- Notes: No over-testing. Helpers (`withAcquireDaemonLockFake`, `withDaemonLockFileReset`) follow existing seam idioms.

CODE QUALITY:
- Project conventions: Followed (seam style mirrors `daemonRunFunc`/`daemonShutdownFunc`; no `t.Parallel()`; `t.Cleanup` for restore).
- SOLID principles: Good.
- Complexity: Low — flat three-way switch.
- Modern idioms: Yes — `errors.Is`, `fmt.Errorf("%w", err)`.
- Readability: Good — comments at `cmd/state_daemon.go:243-254` and `:52-61` carry the rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The task brief preferred re-using the Task 1.1 `lockAcquire` seam rather than introducing a new cmd-level seam. The implementation added a cmd-level seam anyway (`cmd/state_daemon.go:46-50`). Justified: lets `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` inject an arbitrary sentinel error without crafting a real open(2) failure.
- [quickfix] `daemonLockFile` doc comment paraphrases the spec's "Fd retention is load-bearing" — quoting verbatim would tighten the link. Pure taste.
