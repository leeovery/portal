TASK: Narrow the exported SweepLogs surface to drop its dead gated mode and ignored parameter (portal-observability-layer-8-4)

ACCEPTANCE CRITERIA:
- internal/log exports SweepLogsForClean(stateDir string) error (or equivalent) and no longer exports the three-parameter SweepLogs.
- cleanRotatedLogs calls the new entry point; portal clean --logs behaviour byte-for-byte preserved.
- No exported retention function carries a gated bool or a conditionally-ignored retentionDays parameter.
- The gated per-process dayRoll sweep path unchanged, still resolves its window from PORTAL_LOG_RETENTION_DAYS.
- runRetentionSweepWithDays remains the single shared walk/delete/prune implementation.

STATUS: Complete

SPEC CONTEXT:
Spec § Retention policy (446-480). Step 0 single-winner gate (per-process startup). clean --logs (473): cutoff=today, bypasses gate, removes stale sentinels, line 475 "calls into the same sweep function" (shared algorithm, no duplication).

IMPLEMENTATION:
- Status: Implemented (no drift)
- Location: retention.go:114-135 (SweepLogsForClean: today := nowFunc(), cleanCutoffDays := 0, delegates to runRetentionSweepWithDays(stateDir, today, false, &cleanCutoffDays)); cmd/clean.go:178 (cleanRotatedLogs calls log.SweepLogsForClean, was SweepLogs(stateDir,0,false)). Old three-parameter SweepLogs removed (grep: zero references in prod/test). Gated path untouched (sink.go:97 dayRoll → runRetentionSweep(...true) → forcedDays=nil → env resolution via resolveSweepRetentionDays). runRetentionSweepWithDays (81-95) the single shared core; both paths delegate.
- Notes: Narrower SweepLogsForClean(stateDir) (vs plan-text SweepLogs(stateDir, retentionDays, gated)) is the explicitly-intended cycle-2 cleanup outcome; spec line 475 satisfied. Reconciled in report-2-9.

TESTS:
- Status: Adequate
- Coverage: sweeplogs_test.go (deletes prior-day + segment keeps today; bypasses gate when sentinel present; removes ALL swept.* incl today; forces cutoff=today even with PORTAL_LOG_RETENTION_DAYS=365; fixedClock). retention_test.go (gated path via runRetentionSweep(...true) with 30/0/banana env, single-winner gate honoured + UngatedAlwaysRunsRegardlessOfSentinel). cmd/clean_logs_test.go (full cleanRotatedLogs → SweepLogsForClean through Cobra, isolated PORTAL_STATE_DIR: flag default false, no-flag preservation, prior-day deletion keeps today, gate-bypass + all-sentinel prune).
- Notes: Asserts exact file presence/absence post-sweep. Unit + cmd layers test distinct seams. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (nowFunc/removeFunc seams; no t.Parallel; cleanDeps mock + t.Cleanup; documented export).
- SOLID: Good — thin adapter over shared core; gating encapsulated behind unexported path; narrowing improves interface segregation.
- Complexity: Low (3 statements, no public-API branching).
- Modern idioms: Yes (*int forcedDays sentinel distinguishes "explicit 0" from "resolve from env").
- Readability: Good — doc precise about gated-vs-ungated split.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] SweepLogsForClean always returns nil (error reserved for future failure surfacing, documented); a reader may wonder why cleanRotatedLogs checks an always-nil error. Forward-compatible signature; no change. (Carried from report-2-9.)
- [idea] Local cleanCutoffDays := 0 then &cleanCutoffDays is slightly verbose but the clearest idiomatic way to pass a non-nil *int sentinel; no change.
