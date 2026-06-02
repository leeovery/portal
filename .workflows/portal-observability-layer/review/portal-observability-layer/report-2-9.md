TASK: portal clean --logs gate-bypassing sweep with cutoff=today (portal-observability-layer-2-9)

ACCEPTANCE CRITERIA:
- portal clean (no --logs) does not trigger the sweep — rotated logs preserved.
- portal clean --logs deletes every portal.log.<date>[.N] with date < today, leaving today's file intact.
- portal clean --logs bypasses the portal.log.swept.<today> gate.
- portal clean --logs removes stale portal.log.swept.* sentinels.
- Both --logs path and per-process path call the same sweep function (no duplication).
- --logs flag registered on cleanCmd, defaults false.

STATUS: Complete

SPEC CONTEXT:
Spec § Retention policy (446-475). clean --logs is the explicit user-invoked variant: cutoff=today (delete every rotated file leaving the current one), BYPASSES the single-winner gate, removes stale swept.* sentinels. Spec line 475: both paths call the same sweep function. [needs-info] resolved: remove ALL swept.* incl today (clean slate).

IMPLEMENTATION:
- Status: Implemented (documented sound deviation from suggested signature)
- Location: cmd/clean.go:71-74 (reads --logs, calls cleanRotatedLogs only when true); :166-181 (cleanRotatedLogs resolves state dir via state.EnsureDir, calls log.SweepLogsForClean, best-effort WARN under bootstrap); :202 (Bool("logs", false) registered in init). retention.go:114-135 (SweepLogsForClean computes today from nowFunc, delegates to runRetentionSweepWithDays gated=false forcedDays=0); :81-95 (shared algorithm for both paths); :210-228 (pruneStaleSentinels honours gated: ungated removes all incl today); :184 (strict !Before(cutoff), retention=0 → cutoff=today → date<today deleted, today survives; .N handled by pastDayLogDate).
- Notes: Exposes narrower SweepLogsForClean(stateDir) threading forcedDays *int internally rather than suggested SweepLogs(stateDir, retentionDays, gated) — cleaner (clean path only needs one config), task hedged "e.g.", spec "same sweep function" satisfied via shared runRetentionSweepWithDays. No behaviour drift.

TESTS:
- Status: Adequate
- Coverage: sweeplogs_test.go (log layer: deletes prior-day + .N keeping today's base+segment; bypasses gate when sentinel present; removes ALL swept.* incl today; forces cutoff=today even with PORTAL_LOG_RETENTION_DAYS=365; fixedClock pins nowFunc). cmd/clean_logs_test.go (cmd layer: flag defaults false; no-flag preserves logs+sentinel; --logs deletes prior keeps today; --logs bypasses gate + removes all sentinels; PORTAL_STATE_DIR isolation + mockCleanPaneLister). "same sweep function" verified structurally + behaviourally.
- Notes: All six required tests + five edge cases across two layers. Behaviour-focused (filesystem state). cmd-layer tests verify flag→state-dir→sweep wiring through real cobra (not duplicates). env-override test not redundant.

CODE QUALITY:
- Project conventions: Followed (bootstrapLogger best-effort WARN; state.EnsureDir; no t.Parallel; cleanDeps t.Cleanup restore; PORTAL_STATE_DIR isolation).
- SOLID: Good — single shared core; SweepLogsForClean thin adapter; forcedDays *int clean separation.
- Complexity: Low.
- Modern idioms: Yes (CutPrefix, injectable nowFunc, O_CREATE|O_EXCL gate).
- Readability: Excellent — gated-vs-ungated + cutoff=today + [needs-info] reading documented in source + test names.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Exported API is SweepLogsForClean(stateDir) not the suggested SweepLogs(stateDir, retentionDays, gated); sound/narrower. Flag for reviewers reconciling plan text vs code; no change recommended.
- [idea] SweepLogsForClean always returns nil; error return reserved for future failure surfacing (documented); a reader may wonder why cmd checks an always-nil error.
