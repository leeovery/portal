TASK: Emit restore phase B summary restore: geometry complete over the geometry/active-pane/zoom replay (portal-observability-layer-5-4)

ACCEPTANCE CRITERIA:
- Session reaching geometry replay → one INFO restore: geometry complete panes=N took=T under component restore, panes == len(livePanes).
- Best-effort layout/select-pane/zoom failure increments anomalous, existing per-step WARN still fires, replay continues.
- Empty saved-window group skipped, not counted as anomalous.
- No scrollback-replay count in the summary (geometry only).
- Zero-pane session never reaches geometry, emits no geometry summary.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary. Catalog: restore: geometry complete panes=N took=T, component restore. anomalous = items failed anomalously without terminating (also per-item WARN). Closed keys panes/anomalous/took. restore: prefix from bound logger.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/restore/session.go:261-296 (ApplyWindowGeometry); applyLayoutWithFallback :345-355, applyActivePane :359-365, applyZoom :370-376 (each returns bool ok); binding state_common.go:23 → restore.go:60-64.
- Notes: Summary once after per-window loop: r.logger().Info("geometry complete","panes",panes,log.Took(start),"anomalous",anomalous), panes=len(livePanes). One-anomalous-per-degraded-op (double-layout failure returns false once → one increment). Per-step WARNs retained in apply* helpers (return bool, caller tallies). Empty group (len(group)==0) continue-skipped before any apply* call, not anomalous; panes still len(livePanes). No scrollback key (doc comment notes geometry-only, replay FIFO/helper-driven). Zero-pane gated upstream (validateTopology + sr.Restore reject before geometry). anomalous always present (even 0); log.Took helper; no new keys.

TESTS:
- Status: Adequate
- Location: internal/restore/session_geometry_summary_test.go
- Coverage: EmitsGeometryCompleteSummaryOnCleanReplay (panes=3, took=, anomalous=0, INFO); SummaryPanesEqualsLivePaneCount; SelectLayoutFailureIncrementsAnomalousAndRetainsWarn + DoubleLayoutFailureIsOneAnomalous (anomalous=1, both WARNs, continues); SelectPane/Zoom FailureIncrementsAnomalous; EmptySavedWindowGroupSkippedNotAnomalous (anomalous=0, panes=1, negative assertion mock not called); SummaryHasOnlyPanesTookAnomalousAttrs (exact key set {anomalous,panes,took}); EmitsExactlyOneSummaryPerCall.
- Notes: Behaviour-focused (line content, attr-key set, WARN presence, continuation). Zero-pane no-geometry-summary not a dedicated summary-level test but guaranteed by upstream guards (validateTopology tested at orchestrator level).

CODE QUALITY:
- Project conventions: Followed (log.For; nil-safe r.logger() forwarder; log.Took; closed keys; no t.Parallel; terse message).
- SOLID: Good — apply* helpers single responsibility returning bool ok; groupLivePanesBySavedWindow factors windowing.
- Complexity: Low.
- Modern idioms: Yes (slog attrs, log.Took).
- Readability: Good — one-summary/one-anomalous/no-scrollback contract documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] "No geometry summary for a zero-pane session" has no dedicated summary-level assertion (guaranteed structurally by upstream validateTopology/sr.Restore in restoreOne). An orchestrator-level test in restore_test.go running a zero-pane session through Orchestrator.Restore asserting no "geometry complete" line would lock the criterion explicitly.
