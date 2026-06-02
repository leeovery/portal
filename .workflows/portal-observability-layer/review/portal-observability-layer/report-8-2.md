TASK: Extract a shared emitCleanStaleSummary helper for the hooks and project stores (portal-observability-layer-8-2)

ACCEPTANCE CRITERIA:
- A single emitCleanStaleSummary helper owns the clean-stale terminal Warn/Info emission; neither store constructs the terminal summary attr list inline.
- Success → INFO op=clean-stale entries=N via=internal took=<dur>; save-failure → WARN additionally carrying error + error_class. Output byte-identical (keys/values/ordering/message).
- Zero-removal early return still skips Save and any summary in both stores.
- Per-entry DEBUG loops retain store-specific keys (hook_key for hooks; project+path for projects).
- Wrapped error returns ("failed to save after cleaning stale hooks/projects") unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec lines 680-723 (Batch operations): success ONE INFO, failure ONE WARN carrying error_class from AtomicWrite phase space (write-failed-*, never unexpected for whole-batch persist); op closed (clean-stale); via=internal; took always present; per-entry DEBUG; one INFO summary entries=N. Line 860 maps Hooks CleanStale to the batch-summary shape.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/storelog/clean_stale.go:48-60 (EmitCleanStaleSummary); consumed hooks/store.go:289(Warn)/295(Info), project/store.go:227(Warn)/233(Info).
- Notes: Task said lowercase emitCleanStaleSummary; impl correctly chose EXPORTED EmitCleanStaleSummary in shared internal/storelog (necessary — consumed across two packages). Placement rationale documented (log must stay leaf, fileutil must not import log → thin composition package importing both). storelog imported only by hooks/project (+ own test) — no cycle. Only remaining clean-stale Warn/Info sites are inside the helper (grep clean). Attr ordering byte-identical: op, entries, via, [error, error_class], took. took via log.Took(start). Each store retains Load, partition predicate, zero-removal early return, per-entry DEBUG with correct store-specific key. Wrapped error returns unchanged.

TESTS:
- Status: Adequate
- Coverage: storelog/clean_stale_test.go (SuccessInfo: INFO/op/component=hooks/entries=2/via/took Duration, error+error_class absent; FailureWarn: WARN/op/component=projects/entries=3/via/error_class=write-failed-temp-create via ClassifyWriteError/took Duration/error=raw saveErr). Both stores' existing CleanStale logging tests pass unmodified through the helper (hooks store_test.go:726-945, project store_logging_test.go:423-617 — per-entry DEBUG, summary, zero-removal mtime, WARN error_class). Helper tests exercise BOTH component bindings.
- Notes: Behaviour-focused. Save-failure realistically triggers AtomicWrite temp-create via 0500-locked dir + errors.Is end-to-end. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (log.For component loggers; storelog composition w/ documented cycle rationale; logtest.Sink; no t.Parallel; doc comments).
- SOLID: Good — helper owns only terminal branch; partition/Load/DEBUG-loop stay per-store. DRY without over-abstraction (differing per-entry loops correctly NOT pulled in).
- Complexity: Low (single if/else, CC 2).
- Modern idioms: Yes (log.Took, %w wrapping preserved at call sites, sentinel classification).
- Readability: Good — helper doc + call-site comments explain write-failed-* (not "unexpected") + entries_failed-always-absent.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The resolved [needs-info] rationale (no reachable per-entry unexpected WARN under single batched Save) is now triplicated verbatim across hooks/store.go:242-248, project/store.go:182-189, and helper doc clean_stale.go:57-58 (also raised in report-3-3). Now that emission is centralized, the two store-side "entries_failed is omitted" comments sit next to a call that no longer constructs that attr — consider replacing with a "see storelog.EmitCleanStaleSummary" pointer to single-source the rationale. Documentation only.
