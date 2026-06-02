TASK: Emit the orphan-FIFO sweep summary clean: orphan-fifo sweep complete in SweepOrphanFIFOs (portal-observability-layer-5-6)

ACCEPTANCE CRITERIA:
- Sweep → one INFO clean: orphan-fifo sweep complete reaped=N skipped=N took=T under component clean.
- Missing state dir / empty glob → reaped=0 skipped=0.
- Non-FIFO sibling matching glob preserved, counted skipped.
- Per-file os.Lstat/os.Remove failure → WARN (wrapped error attr), counted skipped, loop continues.
- Live-marker-protected FIFO left in place, counted skipped.
- Existing per-removal INFO demoted to per-item DEBUG.
- filepath.Glob failure returns before the summary.

STATUS: Complete

SPEC CONTEXT:
Spec § Cycle-level summary. Catalog (857): clean: orphan-fifo sweep complete reaped=N skipped=N took=T, component clean. Sweeps report outcome keys reaped/skipped + took (no iteration keys). Per-item DEBUG silent at INFO; per-item WARN only anomalous.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/fifo_sweep.go:65-102; cleanLogger = log.For("clean") :21.
- Notes: start :67; reaped/skipped :68; Glob failure returns "glob fifos in %s: %w" before loop (:70-73, no summary); Lstat failure WARN on callerLogger + skipped++ continue (:76-81); non-FIFO (Mode()&ModeNamedPipe==0) preserved skipped++ continue (:82-86); live-marker-protected skipped++ continue (:87-91); Remove failure WARN + skipped++ continue (:92-96); success reaped++ + demoted DEBUG orphan fifo reaped (:97-98, old Info gone); Info("orphan-fifo sweep complete","reaped","skipped",log.Took(start)) :100. Two-component split (per-item WARN under caller's component, summary+DEBUG under clean) documented; production wiring bootstrap_production.go:194 passes bootstrapLogger.

TESTS:
- Status: Adequate
- Location: internal/state/fifo_sweep_summary_test.go
- Coverage: EmitsCleanSummaryCountingReapedAndSkipped (one clean INFO, reaped=2/skipped=1, took Duration via RequireDuration); EmitsZeroReapedZeroSkippedForMissingStateDir; PreservedNonFIFOCountsAsSkipped; RemoveFailureWarnsOnLoggerAndCountsAsSkipped (chmod-0500, 2 WARNs under bootstrap w/ error+path, reaped=0 skipped=2); LiveMarkerProtectedCountsAsSkippedAndIsLeftInPlace; DemotesPerRemovalInfoToDebugUnderClean (old message gone, one DEBUG orphan fifo reaped under clean); BoundaryContract_CallerWarnSinkVsCleanSummary (arbitrary caller component, WARN tracks caller, summary stays clean); NoSummaryWhenGlobFails.
- Notes: Structured-record assertions, not substrings. No t.Parallel (mutates process-wide handler). Root/Windows skips on chmod tests appropriate.

CODE QUALITY:
- Project conventions: Followed (log.For/Took; loggerOrDiscard nil-guard; no t.Parallel; logtest.Sink).
- SOLID: Good — caller-vs-self logger split is an explicit documented boundary contract.
- Complexity: Low.
- Modern idioms: Yes (slog attrs, %w, Glob, ModeNamedPipe).
- Readability: Excellent — 40-line doc comment justifies the two-component split + warns against consolidation regressions; boundary-contract test enforces it.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] fifo_sweep.go boundary-contract test + prod doc comment reference "Task 7-6 made explicit at the signature" but this is Phase 5 Task 5-6 — harmless cross-phase reference (later phase refined the doc) but the stale task-number citation could confuse a future reader.
- [idea] Three tests separately drive SweepOrphanFIFOs with chmod-0500 remove-failure scaffolding; extracting a seedRemoveFailure(t, dir) helper would reduce duplication.
