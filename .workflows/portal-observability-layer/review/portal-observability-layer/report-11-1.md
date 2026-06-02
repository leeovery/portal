TASK: Migrate tui previewCaptureSink onto the shared logtest.Sink helper (portal-observability-layer-11-1)

ACCEPTANCE CRITERIA:
- previewCaptureSink and all its methods removed from internal/tui/preview_attach_test.go.
- newTestLogger returns a logger routed through logtest.NewCaptureLogger(t) bound to component=preview, plus a *logtest.Sink.
- readLog reads the body via logtest.Sink.Body().
- No production (*.go non-_test.go) file modified.
- Rendered-body shape (<LEVEL> <msg> key=value..., bound attrs first) unchanged.

STATUS: Complete

SPEC CONTEXT:
Phase 11 (cycle-5) cleanup from the c5 duplication analysis (analysis-duplication-c5.md:9-13): c3/c4 extraction into internal/logtest missed the internal/tui preview-attach test surface. Shared logtest.Sink is the single source of truth for the <LEVEL> <msg> key=value body shape with component bound on every line.

IMPLEMENTATION:
- Status: Implemented (matches all ACs)
- Location: internal/tui/preview_attach_test.go:81-85 (newTestLogger returns (*slog.Logger, *logtest.Sink) via logtest.NewCaptureLogger(t) + logger.With("component","preview")); :88-91 (readLog takes *logtest.Sink, calls sink.Body()); :12 (logtest import added); :3-14 (now-unused context/fmt/sync imports removed). previewCaptureSink struct + all methods (owner/Enabled/WithAttrs/WithGroup/Handle/body) fully deleted (grep: zero remaining references in Go source).
- Notes: Other call sites compile against the new return shape (preview_attach_wiring_test.go:114, preview_attach_pipeline_handoff_test.go:18,42 use logger, _ := newTestLogger(t)). Inert WithGroup divergence (old return s vs shared &Sink{...}) fixed as a side effect. internal/logtest leaf (no portal-internal imports in capture.go) → zero cycle risk for package tui. No production file modified.

TESTS:
- Status: Adequate
- Coverage: Test scaffolding — existing preview-attach suite (8 tests + consumers in two other files) preserved unchanged as the verification surface. Substring assertions still verify level label (WARN) + bound component=preview (lines 178-181/197-200/216-219/236-243); meaningful because shared Sink.Handle renders the .With("component","preview") binding on every line.
- Notes: Would fail if broken (failed bound-attr render → Contains "preview" fails; dropped level prefix → Contains "WARN" fails). No new tests (correct — pure consolidation). multiple-WARN + silent-logger no-panic tests retained.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; internal/logtest leaf pattern; doc comments updated).
- SOLID/DRY: The point — removes a ~55-line verbatim duplicate of the shared handler core.
- Complexity: Low (net deletion; two trivial pass-throughs retained).
- Modern idioms: Yes (shared logtest.NewCaptureLogger(t) (*slog.Logger, *Sink) constructor).
- Readability: Good — doc comments describe the single-sourced contract.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The c5 analysis (analysis-duplication-c5.md:15-19) records a separate low-severity finding: the component-binding WithAttrs/WithGroup/owner recorder skeleton is still re-authored across four tmux/bootstrap test recorders (bootstrap_test.go:121-219, portal_saver_test.go:2625-2666, hooks_register_test.go:693-747, hooks_migration_test.go:39-99). Explicitly out of scope for 11-1 (last text-rendering re-author only); the analysis recommends NOT forcing a shared abstraction over the divergent storage tails yet. Noted for traceability.
