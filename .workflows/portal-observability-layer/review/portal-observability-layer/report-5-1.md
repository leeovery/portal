TASK: Emit the daemon tick cycle summary capture: tick complete with sessions/panes/natural_churn/anomalous/took and per-pane DEBUG/WARN (portal-observability-layer-5-1)

ACCEPTANCE CRITERIA:
- Successful tick (capture work + commit) → exactly one INFO capture: tick complete with sessions/panes/natural_churn/anomalous/took.
- ctx-cancellation at any of three observation points returns nil before the summary (no line).
- Per-pane CaptureAndHashPane/WriteScrollbackIfChanged failure increments anomalous, one per-pane WARN with wrapped error, loop continues.
- Phase-boundary error (list markers/capture structure/commit) returns wrapped error without summary.
- Idle tick (!dirty && !gap) and restoring tick emit no summary.
- Per-pane DEBUG under capture (silent at INFO); summary is INFO.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary cadence. Concrete catalog pins daemon-tick to component capture, shape capture: tick complete sessions=N panes=N natural_churn=N anomalous=N took=T. natural_churn (user-closed mid-cycle) distinct from anomalous (failed without terminating, also per-item WARN). Per-tick events NOT in lifecycle catalog.

IMPLEMENTATION:
- Status: Implemented (exceeds minimum — natural_churn classification fully wired, not deferred to 0)
- Location: state_common.go:37-43 (captureLogger = log.For("capture"), promoted out of daemon); state_daemon.go:395 (start after obs-1); :414-415 counters; :448-449 per-pane DEBUG; :460-464 isPaneVanishedError→naturalChurn + DEBUG pane vanished error_class=expected; :465-474 anomalous capture/write WARN on daemonLogger continue; :386-441 three ctx.Done()→return nil; :484-486 commit error returns; :495-501 single captureLogger.Info("tick complete",...,log.Took(start)) post-Commit; :505-526 isPaneVanishedError classifier.
- Notes: Catalog shape exact. log.Took DRY. Per-pane WARN on daemonLogger (lowest-churn rationale); summary+DEBUG on captureLogger. Idle/restoring paths never reach captureAndCommit. defaultShutdownFlush also emits one tick complete (acknowledged-acceptable).

TESTS:
- Status: Adequate
- Location: cmd/state_daemon_cycle_summary_test.go
- Coverage: EmitsOneTickCompleteSummaryOnSuccess (INFO, counts, took KindDuration); NoSummaryWhenCtxCancelledAtObsPoint1/2/3 (each distinct obs point, zero summaries, nil); Anomalous capture/write (anomalous=1, continue, one WARN component=daemon); NoSummaryOnCommitPhaseError; CountsUserClosedPaneAsNaturalChurnNotAnomalous (natural_churn=1, DEBUG pane vanished error_class=expected, no WARN); EmitsPerPaneDebugBreadcrumbUnderCapture (DEBUG component=capture, canonical pane_key=work__0.0).
- Notes: Behaviour-focused, each isolates one AC. Sink records all levels (Enabled always true) — INFO filtering is a handler concern tested in internal/log. Idle no-op path structurally outside captureAndCommit (covered by broader suite).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; component logger; log.Took; errors.As classification).
- SOLID: Good — isPaneVanishedError single-responsibility classifier.
- Complexity: Acceptable (triple loop pre-existed; +3 counters +2 log calls).
- Modern idioms: Yes (errors.Is/As, slog attrs, log.Took).
- Readability: Good — counter semantics + emit-only-on-success documented.
- Issues: None functional.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] state_daemon.go:375-378 drift-mirror comment claims cmd/bootstrap/daemon_tick_test_helpers_test.go runDaemonTick "shadows this body byte-for-byte"; that helper was NOT updated and structurally should NOT mirror logging/natural-churn/counters (it's a topology/commit-pipeline sim for AC4). "byte-for-byte" wording is stale/overstated — soften to scope the mirror to the capture/commit pipeline, exclude logging.
- [idea] defaultShutdownFlush emits a tick complete in addition to the shutdown INFO; documented-acceptable. If shutdown-vs-tick disambiguation matters, a flush marker attr could help. Future-observability only.
