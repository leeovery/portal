AGENT: standards
STATUS: findings
FINDINGS_COUNT: 3
SUMMARY: Implementation closely follows the spec; one moderate spec deviation in the integration test (pidfile-life check substituted for spec-mandated pgrep server-children count) plus two minor consistency / coverage gaps.

FINDINGS:

- FINDING: Integration test substitutes pidfile-PID-life check for spec's pgrep server-children count
  SEVERITY: medium
  FILES: internal/tmux/portal_saver_integration_test.go:213-222
  DESCRIPTION: Spec § Acceptance Criteria → Singleton invariant (lines 192, 327) requires the load-bearing integration test to assert `pgrep -P <tmux-server-pid> -f 'portal state daemon' | wc -l == 1` after both `EnsurePortalSaverVersion` calls return. The test was deliberately rewritten to assert `priorPID dead && currentPID alive` instead. These are not equivalent: the pgrep form counts every `portal state daemon` process whose parent is the tmux server PID, catching any orphan whose PID is no longer recorded in `daemon.pid` (the exact original-bug shape — pidfile overwritten, prior daemon still running but unreachable through `daemon.pid`). The pidfile-life check only sees PIDs the pidfile mentions; a third orphan attached to the tmux server but not in `daemon.pid` would slip past. The test comment claims the two are equivalent under the "exactly one daemon per stateDir" property, but that reasoning relies on the lock invariant (which is the thing under test) rather than on the observable process tree. In this single-recycle test at most one prior daemon can be orphaned so the assertion is sufficient in practice — but the spec's pgrep form is what gives the test its forward-compat / regression-guard value (catching multi-orphan accumulation shapes the current assertion cannot model). Spec line 329 is explicit that this integration test is "the load-bearing test for the bug — it would have caught the issue in CI had it existed before."
  RECOMMENDATION: Add (alongside the current alive/dead pair) a `pgrep -P <serverPID> -f 'portal state daemon'` invocation and assert the live-process count is exactly 1. The serverPID is already captured at line 171. This restores the spec's intended observation surface without removing the existing assertions, which remain valuable for showing the recycle actually displaced the prior daemon.

- FINDING: Kill-barrier WARN uses literal "bootstrap" string instead of state.ComponentBootstrap constant
  SEVERITY: low
  FILES: internal/tmux/portal_saver.go:177
  DESCRIPTION: `killSaverAndWaitForDaemon` emits its WARN-on-timeout line with a hard-coded literal `"bootstrap"` as the component argument. Every other production log site in the codebase uses the typed constants (`state.ComponentDaemon`, `state.ComponentBootstrap`). The file already imports `internal/state`, so the constant is reachable. Purely a consistency / future-rename-safety issue, not a behaviour bug.
  RECOMMENDATION: Replace the literal `"bootstrap"` argument with `state.ComponentBootstrap` so the constant is the single source of truth.

- FINDING: ERROR-level log line on non-contention lock-acquire failure is not asserted by any test
  SEVERITY: low
  FILES: cmd/state_daemon_test.go:570-599, cmd/state_daemon.go:261
  DESCRIPTION: Spec § Fix Part 1 → Lock-file create / open semantics (line 100) requires non-EWOULDBLOCK open(2)/flock failures to log an ERROR-level line describing the failure and exit non-zero. The implementation does log it at ERROR and propagates the wrapped error. However, `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` only asserts the non-zero exit and that state files were not written — it never asserts the ERROR-level log line was emitted. The WARN-on-contention sibling test does assert log presence and exactly-one-line. The ERROR path needs equivalent coverage to honour the spec's "loud surfacing" requirement.
  RECOMMENDATION: Extend `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` (or add a sibling) to set `PORTAL_LOG_LEVEL=error`, read portal.log post-run, and assert exactly one line contains "ERROR" and "acquire daemon lock".
