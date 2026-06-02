TASK: Emit the two cmd/bootstrap clean-sweep summaries (orphan-daemon killed, marker unset) (portal-observability-layer-5-5)

ACCEPTANCE CRITERIA:
1. Orphan-daemon sweep → one INFO clean: orphan-daemon sweep complete killed=N took=T under component clean, killed counts ONLY successful SIGKILLs.
2. Pre-existing sweep: killed orphan daemon pid=N INFO demoted to per-item DEBUG.
3. Identity-skips remain DEBUG; kill-failures/identity-check-failures remain WARN; killed excludes skipped/failed.
4. Pgrep failure returns before loop, no orphan-daemon summary; SaverPanePID error proceeds with empty legitimate set, still reaches summary.
5. Marker sweep → one INFO clean: marker sweep complete unset=N took=T under component clean, unset counts only successful unsets.
6. mass-unset-hazard deferral emits summary with unset=0 (never false unset), deferral WARN still fires; ListSkeletonMarkers/ListAllPanesWithFormat errors return before summary.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary. Catalog (858-859): clean: orphan-daemon sweep complete killed=N took=T + clean: marker sweep complete unset=N took=T, both component clean. Closed outcome keys killed/unset + took. Component clean owns sweep outcomes even though orchestration runs under bootstrap.

IMPLEMENTATION:
- Status: Implemented
- Location: bootstrap.go:53 (cleanLogger = log.For("clean")); orphan_sweep.go:151 start, :152 killed, :201 per-kill DEBUG orphan killed, :202 killed++, :208 Info("orphan-daemon sweep complete","killed",killed,log.Took(start)); Pgrep-fail early return :155-158 (no summary); SaverPanePID error :163-164 WARN on bootstrap logger, proceeds. stale_marker_cleanup.go:125 start, :126 unset, :132-134 summarise() closure Info("marker sweep complete","unset",unset,log.Took(start)) invoked at three non-error returns, list-error returns before it, :183 unset++ only on success. Wiring bootstrap_production.go:168,185-190.
- Notes: Per-kill DEBUG on cleanLogger (per task "Do"); identity-skip + kill/unset failures on bootstrap logger. Old "sweep: killed orphan daemon" string gone. Only closed keys.

TESTS:
- Status: Adequate
- Location: cmd/bootstrap/clean_sweep_summary_test.go
- Coverage: EmitsCleanSummaryCountingSuccessfulKills (killed=2, INFO, took Duration); DemotesPerKillInfoToDebug (no old INFO; one DEBUG orphan killed under clean w/ target_pid); ExcludesSkippedAndFailedFromKilled (killed=1, skip DEBUG + failure WARN under bootstrap); NoSummaryWhenPgrepFails; SummaryWithZeroKilledWhenSaverPanePIDErrors; EmitsCleanSummaryCountingSuccessfulUnsets; SummaryUnsetCountsOnlySuccessfulUnsets (aggregate error still returned); SummaryUnsetZeroOnMassUnsetHazardDeferral (deferral WARN retained); NoSummaryWhenListErrorReturns (both sub-cases); SummaryUnsetZeroOnEmptyMarkersNoOp.
- Notes: Structured logtest.Sink assertions (component/level/int-kind/Duration-kind), not string matching. errs{2} 1-based index deterministic against map ordering. Pre-existing orphan_sweep/stale_marker suites unaffected.

CODE QUALITY:
- Project conventions: Followed (log.For/Took; no t.Parallel; DI seam).
- SOLID: Good — summary added without changing seam interfaces; summarise closure DRY across three returns.
- Complexity: Low.
- Modern idioms: Yes (slog attrs, log.Took).
- Readability: Good — component flip + no-summary-on-early-return documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Per-kill DEBUG on cleanLogger while identity-skip/kill-failure on bootstrap logger → a single sweep's per-item lines land under two components; an operator grepping component=clean sees successes but not skips/failures. Matches the spec/task ("component flips to clean only on the summary", per-kill DEBUG a deliberate exception); worth a one-line confirmation it's intended. No code change implied.
