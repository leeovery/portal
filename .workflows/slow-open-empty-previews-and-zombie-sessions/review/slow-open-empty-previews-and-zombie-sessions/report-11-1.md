TASK: Fix TestStateDaemon_ReturnsErrorOnNonContentionLockFailure To Assert WARN

ACCEPTANCE CRITERIA:
- Targeted test passes.
- go test ./... clean.
- Test asserts exactly one matching log line (silent/noisy invariant preserved).
- All ERROR-referencing comments updated to WARN.

STATUS: Complete

SPEC CONTEXT:
Component C (cmd/state_daemon.go:213) emits WARN on non-EWOULDBLOCK lock-acquire failure to mirror the WARN-on-contention sibling path so the fatal path is not noisier than legitimate contention. Spec § Fix Part 1 → Lock-file create/open semantics requires non-contention open(2)/flock failures to emit WARN per Component C. T9-7 flipped production Error → Warn; T11-1 aligns the test.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon_test.go:592–655
- Notes:
  - Function renamed to `TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure` (line 592).
  - `PORTAL_LOG_LEVEL` set to `"warn"` (line 597), with comment explaining WARN-threshold rationale and Component C citation (lines 593–596).
  - `strings.Contains` matchers flipped to "WARN": top-level check at line 638, per-line check at line 647.
  - Error messages updated: "expected a WARN log line" (line 639), "expected exactly one WARN line" (line 652).
  - Spec-citation comment updated to reference WARN-level emission and Component C / sibling-contention parity (lines 627–632).
  - Single-match invariant preserved (matches counter, lines 645–654).
  - Production source at cmd/state_daemon.go:213 confirmed to emit `deps.Logger.Warn`.
  - Grep across cmd/state_daemon_test.go shows zero `ERROR` tokens remaining.
  - Sibling `TestStateDaemon_EmitsWarnOnLockContention` (line 678) uses identical pattern — consistent.

TESTS:
- Status: Adequate
- Coverage: WARN present; "acquire daemon lock" content present; exactly-one-line invariant; wrapped sentinel error; daemon.pid not written; tick loop not reached.
- Notes: Tight scope; not over-tested, not under-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, t.Setenv, t.Cleanup).
- SOLID: Good (single-responsibility test).
- Complexity: Low.
- Modern idioms: Yes (errors.Is, os.IsNotExist).
- Readability: Good — comments cite spec section and rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
