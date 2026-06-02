TASK: Emit restore phase A summary restore: skeleton complete over the per-session create loop (portal-observability-layer-5-3)

ACCEPTANCE CRITERIA:
- Restore creating N skeletons → one INFO restore: skeleton complete sessions=N windows=sum-saved-windows panes=sum-saved-panes took.
- Live-skipped session (name in liveSet) excluded from all three counts.
- Underscore-prefixed + invalid-topology session (zero windows / window with zero panes) excluded.
- Session whose sr.Restore errors excluded; its per-session WARN still fires.
- no-sessions.json, zero-saved-sessions, list-sessions-failure early returns emit no summary.
- corrupt-sessions.json returns (true, wrapped ErrCorruptIndex) before loop, no summary.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary. Catalog: restore: skeleton complete sessions=N windows=N panes=N took=T, component restore. Closed keys sessions/windows/panes/took. Component bound via log.For("restore").

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/restore.go:70-89 (start, loop tally, emit); :139-163 (restoreOne returns bool, counted only on full success); :99-108 (corrupt/absent early returns); :51-58 (zero-sessions + list-failure early returns); wiring bootstrap_production.go:158-161 + state_common.go:23.
- Notes: start captured immediately before loop (after early returns). restoreOne returns true only when passes underscore-skip + live-skip + validateTopology + sr.Restore nil; every skip/Restore-error returns false (per-session WARN at :156 still fires). windows/panes from SAVED topology (len(sess.Windows), sum len(w.Panes)) not live re-query. Only closed keys + log.Took. Uses o.logger() nil-safe forwarder (safer than literal o.Logger).

TESTS:
- Status: Adequate
- Location: internal/restore/restore_test.go
- Coverage: EmitsSkeletonCompleteSummaryAfterRestoringSessions (2/3/4, INFO, exactly-one via skeletonSummaryLine); ExcludesLiveSkippedSession (2-pane live session excluded); ExcludesUnderscorePrefixedSession; ExcludesInvalidTopologySessions (zero-window + zero-pane-window); ExcludesRestoreErroredSessionButKeepsWarn (sessions=1, WARN fires); EmitsNoSkeletonSummaryOnPreLoopEarlyReturns (table absent/zero/list-fails); EmitsNoSkeletonSummaryOnCorruptIndex.
- Notes: Asserts rendered line; skeletonSummaryLine enforces exactly-one. Behaviour-focused. Each exclusion class distinct path.

CODE QUALITY:
- Project conventions: Followed (log.For/Took; terse message; closed keys; nil-safe forwarder; no t.Parallel).
- SOLID: Good — restoreOne returns bool driving tally (loop owns aggregation, restoreOne owns per-session decision); no counter-pointer threading.
- Complexity: Low.
- Modern idioms: Yes (slog attrs, log.Took bookend).
- Readability: Good — SAVED-vs-live distinction + count-only-on-full-success documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Skeleton-summary tests assert attr presence via substring matching rather than the exact ordered-key-set assertion the phase-B geometry tests use (recordsWithMessage); substring wouldn't catch an accidental extra key. Low value (spec forbids new keys; phase-B already exercises exact-key-set), but adopting recordsWithMessage here would make "no new keys" explicit for phase A too.
