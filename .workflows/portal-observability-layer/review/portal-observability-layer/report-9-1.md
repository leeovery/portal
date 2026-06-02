TASK: Extract the shared test capture-handler ("captureSink") into one leaf test-helper package (portal-observability-layer-9-1)

ACCEPTANCE CRITERIA:
- The capture-handler base (sink struct, Enabled/WithGroup/Handle/body, newCaptureLogger) declared in exactly one place.
- The new helper package imports nothing portal-internal beyond internal/log itself, no import cycle.
- Rendered body contract <LEVEL> <msg> key=value byte-identical, every existing substring assertion passes unchanged.
- cmd's component-binding extension and restore's records/recordsWithMessage exact-key-set extension remain in owning packages as thin wrappers/embedded fields over the shared sink.
- No production (non-*_test.go) code imports the new helper.

STATUS: Complete

SPEC CONTEXT:
Phase-9 cycle-3 Rule-of-Three duplication finding: a ~40-line capturing slog.Handler re-authored in cmd, internal/state, internal/restore test surfaces; the rendered <LEVEL> <msg> key=value shape is the contract substring assertions key on. Spec line 81 sanctions the in-process SetTestHandler capture approach.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/logtest/capture.go (new leaf; Sink + Enabled/WithAttrs/WithGroup/Handle :127-173, Body :176-181, NewCaptureLogger :215-219; stdlib-only imports — stricter than the criterion's internal/log allowance). cmd/logging_capture_test.go:42-52 (newCaptureLoggerForComponent thin wrapper). internal/restore/logging_capture_test.go:14-45 (captureSink embeds *logtest.Sink + recordsWithMessage/capturedRecord). internal/state/fifo_sweep_summary_test.go:34-84 (fifoSummarySink embeds; state's local copy deleted).
- Notes: Rendering byte-identical (Level.String()+" "+Message, then " %s=%v" over bound-then-call attrs, joined with "\n"). Grep confirms the sink core exists in exactly the expected files (shared + two thin wrappers). No production .go imports internal/logtest. Doc rationale carried into capture.go:1-19.

TESTS:
- Status: Adequate
- Coverage: internal/logtest/capture_test.go (rendering shape; structured ordered keys; bound-component-on-every-line for cmd wrapper path; Lines snapshot copy semantics; structured Attrs map + Kind; bound-then-call last-write-wins; typed accessor failing paths via fakeT/expectFail). Existing cmd/state/restore assertion tests are the regression guard, unchanged.
- Notes: Focused, non-redundant. fakeT harness idiomatic for failing-path coverage. Some asserted surface (typed accessors) belongs to c4 extension but all correctly locked here.

CODE QUALITY:
- Project conventions: Followed (leaf test-only package mirroring portaltest/restoretest/tmuxtest; no t.Parallel; no-production-import documented).
- SOLID: Good — single-responsibility base; per-package divergences layered via struct embedding (open/closed), not folded into the base.
- Complexity: Low (single-traversal Handle; owner() routes derived handlers into one buffer).
- Modern idioms: Yes (slog.Handler; TestingT interface for testable failing paths; snapshot-returning accessors).
- Readability: Good — doc explains contract + buffer-routing + why extensions stay out of the base.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/logtest now carries Record/Attrs typed accessors beyond the minimal 9-1 base; sanctioned by the later approved analysis-tasks-c4 Task 1 (verified at 10-1), not 9-1 scope creep.
- [idea] restore's extension derives capturedRecord lazily from Sink.Records() instead of maintaining a parallel records slice (strictly better — no second buffer to sync); diverges from the literal Do-step-4 wording but preserves the exact-key-set assertion.
