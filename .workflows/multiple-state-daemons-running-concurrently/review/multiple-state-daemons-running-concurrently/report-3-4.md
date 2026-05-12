# Review Report — Task 3.4

TASK: Cover the ERROR-level log assertion for non-contention lock-acquire failure

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- PORTAL_LOG_LEVEL=error set via t.Setenv.
- portal.log read post-run.
- Exactly one line contains both "ERROR" and "acquire daemon lock".
- Non-zero exit + no state writes still asserted.
- Hand-mutated removal of ERROR log call would cause the new assertion to fail.

SPEC CONTEXT:
Specification § Fix Part 1 → Lock-file create/open semantics (line 100) mandates non-EWOULDBLOCK open(2)/flock failures be surfaced loudly: ERROR-level log line + non-zero exit. Distinct from contention path. Analysis-cycle finding: coverage asymmetry — WARN-on-contention sibling test asserts log presence and exactly-one-line, while ERROR path test only asserted non-zero exit. Task 3-4 closes that gap.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_test.go:581-640` (TestStateDaemon_ReturnsErrorOnNonContentionLockFailure)
- Notes: Extended in place rather than as a sibling test — allowed by task wording. ERROR log emitter under test lives at `cmd/state_daemon.go:261` (`logger.Error(state.ComponentDaemon, "acquire daemon lock: %v", err)`).

Per-criterion verification:
1. PORTAL_LOG_LEVEL=error via t.Setenv — line 584.
2. portal.log read post-run — line 618 reads `filepath.Join(dir, "portal.log")`.
3. Exactly-one-line containing both "ERROR" and "acquire daemon lock" — lines 630-639.
4. Non-zero exit + no state writes — lines 601-612 assert err is non-nil, wraps sentinel via `errors.Is`, `daemon.pid` does not exist.
5. Hand-mutation property holds — removing the `logger.Error` call at `cmd/state_daemon.go:261` would emit zero ERROR-level lines.

TESTS:
- Status: Adequate
- Coverage: Five-criterion assertion block mirrors WARN-on-contention sibling at lines 663-701. Symmetry restored.
- Notes:
  - Three layered substring checks (lines 623-628 for individual presence; lines 630-639 for exact-one combined). Not over-tested.
  - `withDaemonLockFileReset(t)` present (line 587) — consistent with Task 3-5 discipline.
  - `withAcquireDaemonLockFake` seam injects the non-contention error (lines 590-592).

CODE QUALITY:
- Project conventions: Followed — no `t.Parallel()`; `t.Cleanup`; `t.Setenv`; `t.TempDir`.
- Complexity: Low — linear assertion block, single sentinel.
- Modern idioms: `errors.Is`, `strings.Split` / `strings.Contains`.
- Readability: Good — comment block cites Spec § Fix Part 1.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Test asserts daemon.pid absence but not daemon.version absence on ERROR path. The contention-path sibling checks both. Strictly redundant (impl cannot write daemon.version without first writing daemon.pid) but symmetry with WARN-path would be marginally cleaner.
- [quickfix] Comment at `cmd/state_daemon_test.go:582-583` reads "ERROR is above the default INFO threshold". The default threshold is WARN, not INFO. Functionally inert; two-character fix.
