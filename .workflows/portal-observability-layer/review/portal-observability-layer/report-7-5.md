TASK: Make SweepOrphanFIFOs' caller-vs-self component attribution explicit at the boundary (portal-observability-layer-7-5)

ACCEPTANCE CRITERIA:
- SweepOrphanFIFOs' signature unambiguously conveys its logging behavior (one logger, or a clearly-named/documented caller-WARN-sink parameter).
- Component attribution of every emitted line unchanged from current behavior unless option (a) chosen (then WARNs move to clean + documented).
- No other state cycle function silently carries the same undocumented split.
- go build / go test ./internal/state/... pass.

STATUS: Complete

SPEC CONTEXT:
Spec concrete cycle catalog (857): orphan-FIFO sweep summary owned by clean. Component taxonomy (173): tmux-call detail rides as DEBUG breadcrumbs under the caller's component — the spec-backed rule that per-item detail correlates with the caller's component while cycle summaries are owned by their catalog component. Phase-7 architecture cleanup: the bare logger param made the boundary contract implicit/fragile.

IMPLEMENTATION:
- Status: Implemented (option b chosen)
- Location: internal/state/fifo_sweep.go:13-102. Parameter renamed logger → callerLogger (:65). 27-line doc comment (:38-64) states callerLogger is the CALLER-component WARN sink (spec-backed correlation) while cycle-summary INFO + per-reaped DEBUG are on package-level cleanLogger (component=clean) by design, and why neither dropping nor consolidating is safe. cleanLogger var doc (:13-21) cross-references.
- Notes: Production wiring unchanged in behaviour (bootstrapadapter.FIFOSweeper.Logger = bootstrap logger → per-item WARNs under component=bootstrap, summary under clean). Criterion 3 verified: internal/state declares exactly two package-level log.For loggers (cleanLogger, signalLogger); all other injected-logger cycle functions use injected logger uniformly; WriteFIFOSignal uses only signalLogger (no injected logger, documented). SweepOrphanFIFOs is the only function with the split, now explicit. loggerOrDiscard(callerLogger) preserves nil-safety.

TESTS:
- Status: Adequate
- Location: internal/state/fifo_sweep_summary_test.go:284-348
- Coverage: BoundaryContract_CallerWarnSinkVsCleanSummary (arbitrary caller component "restore", forces per-item remove failure via chmod 0500, asserts per-item WARN under restore NOT clean, summary stays clean NOT caller) — turns red on either re-attribution. Existing summary/regression tests still exercise with nil + captured loggers.
- Notes: Serial (mutates process-wide handler, no t.Parallel). chmod EACCES setup guards Windows + root. Behavioral assertions.

CODE QUALITY:
- Project conventions: Followed (package-component-logger naming avoids shadowing the pervasive logger param; SetTestHandler captures it; no t.Parallel).
- SOLID: Good — explicit named dependency seam.
- Complexity: Low (rename + doc, control flow unchanged).
- Modern idioms: Yes (slog, loggerOrDiscard).
- Readability: Exemplary — doc comment encodes the exact anti-regression intent (the point of option (b)).
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] RemoveFailureWarnsOnLoggerAndCountsAsSkipped (:166) and BoundaryContract (:297) overlap (both force remove failure + assert WARN under injected logger's component + clean summary); the boundary test is the stronger superset. Consider folding the older test's distinct value (error+path attrs, skipped=2) into the boundary test or a one-line "intentional split of concerns" comment.
- [idea] The SweepOrphanFIFOs doc comment is large (:38-64); justified as the explicit boundary contract, but if the caller-vs-self pattern recurs, extracting the rationale into a shared doc the per-function comments reference would keep it uniform without copy-paste.
