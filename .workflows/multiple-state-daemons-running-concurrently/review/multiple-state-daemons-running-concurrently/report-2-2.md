# Review Report — Task 2.2

TASK: Wire kill-barrier helper into both kill call sites (EnsurePortalSaverVersion + BootstrapPortalSaver stale-pidfile branch) and route production `*state.Logger` into `killBarrierLogger` via `tmux.SetBarrierLogger`.

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- Both kill call sites in `internal/tmux/portal_saver.go` invoke the barrier helper via `killSaverAndWaitForDaemonFn` seam.
- Bare `_ = c.KillSession(PortalSaverName)` removed from both call sites; present only inside `killSaverAndWaitForDaemon`.
- BootstrapPortalSaver / EnsurePortalSaverVersion signatures unchanged.
- Helper not invoked on version-match steady-state path.
- Helper not invoked when BootstrapAliveCheck returns true.
- Helper not invoked when session absent.
- `tmux.SetBarrierLogger` exported and called from `internal/bootstrapadapter/adapters.go`. WARN reaches recorder verified by unit test.
- Existing portal_saver_test.go tests continue to pass.
- `go test ./internal/tmux/...` and `go test ./...` green.
- No `t.Parallel()`.

SPEC CONTEXT:
Spec §"Fix Part 2: Synchronous Kill Barrier → Both kill sites use the barrier" mandates the two distinct kill call sites both route through the shared barrier. Spec §"Acceptance Criteria → No regression on steady-state critical path" requires the helper is not entered when no kill is needed. Spec §"Observability" requires WARN-on-timeout reaches a real logger in production.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver.go:115-123` — `killSaverAndWaitForDaemonFn` package-level seam.
  - `internal/tmux/portal_saver.go:207-213` — BootstrapPortalSaver stale-daemon branch.
  - `internal/tmux/portal_saver.go:249-258` — EnsurePortalSaverVersion mismatch branch. HasSession guard preserved.
  - `internal/tmux/portal_saver.go:99-113` — exported `SetBarrierLogger` setter with nil-guard.
  - `internal/bootstrapadapter/adapters.go:75-78` — `HookRegistrar.RegisterPortalHooks` calls `tmux.SetBarrierLogger(r.Logger)` immediately before `tmux.RegisterPortalHooks`. Step 2, before Step 4 EnsureSaver.
  - `cmd/bootstrap_production.go:127` — production wiring passes real `*state.Logger` into HookRegistrar.
- Grep for `KillSession(PortalSaverName)` finds it only inside `killSaverAndWaitForDaemon` (portal_saver.go:154, :160, :165).
- Function signatures unchanged.
- `*state.Logger` structurally satisfies BarrierLogger.

TESTS:
- Status: Adequate
- Coverage (all in `internal/tmux/portal_saver_test.go`):
  - `TestEnsurePortalSaverVersion_InvokesBarrierHelperOnVersionMismatch` (1300)
  - `TestEnsurePortalSaverVersion_DoesNotInvokeBarrierHelperOnVersionMatch` (1337)
  - `TestBootstrapPortalSaver_InvokesBarrierHelperOnStaleDaemon` (1363)
  - `TestBootstrapPortalSaver_DoesNotInvokeBarrierHelperWhenSessionAbsent` (1402)
  - `TestBootstrapPortalSaver_DoesNotInvokeBarrierHelperWhenDaemonAlive` (1433)
  - `TestBootstrapPortalSaver_PreservesKillSessionWhenRealHelperRuns` (1465)
  - `TestBootstrapPortalSaver_PreservesKillBeforeNewSessionOrderThroughBarrier` (1499)
  - `TestBootstrapPortalSaver_ToleratesBarrierWarnOnTimeoutPath` (1540)
  - `TestSetBarrierLogger_RoutesWarnOnTimeoutThroughInstalledLogger` (1577)
  - `TestSetBarrierLogger_IgnoresNilLogger` (1615)
- Test seam pattern: `installKillSaverFn` mirrors existing patterns with `t.Cleanup` restoration.
- No `t.Parallel()`.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. BarrierLogger is single-method.
- Complexity: Low.
- Modern idioms: Idiomatic structural interface satisfaction.
- Readability: Good.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `SetBarrierLogger` nil-guard does not catch a typed-nil `*state.Logger` boxed inside the BarrierLogger interface. No bug today because `(*state.Logger).Warn` has its own nil-receiver guard. Already noted and accepted by the team during prior analysis cycles.
- [idea] `time.NewTicker(killBarrierPollInterval)` delays the first probe by one full pollInterval after `KillSession`. Inherited from Task 2.1, accepted.
- [idea] Tests use a "swap fn + assert recorder" pattern — one indirection deeper than strictly necessary, but is the spec-prescribed mechanism.
