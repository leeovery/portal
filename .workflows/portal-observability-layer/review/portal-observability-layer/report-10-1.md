TASK: Extend internal/logtest.Sink to expose attr values and collapse the 11 structured-attr-map capture handlers onto it (portal-observability-layer-10-1)

ACCEPTANCE CRITERIA:
1. logtest.Record carries a new Attrs map[string]slog.Value populated in Sink.Handle within the existing bound+call attr loop (no second traversal); existing Keys field unchanged.
2. All eleven listed test files no longer declare a local structured-attr-map capturing slog.Handler; each routes through logtest.Sink, retaining only its genuinely-divergent convenience accessor as a thin wrapper.
3. The four store packages no longer carry their own installCapture/onlyRecord/attrString copies; "mirrors the hooks/project store test sinks" comments removed.
4. No production file modified; internal/logtest imported only from _test.go.
5. go build/test pass with the same assertions (no test weakened/deleted).

STATUS: Complete

SPEC CONTEXT:
Spec:81 pins the test strategy (in-process SetTestHandler capture, no subprocess). Pure test-scaffolding consolidation, no production behavior touched. Phase-10 (cycle 4) duplication cleanup building on c3 task 9-1.

IMPLEMENTATION:
- Status: Implemented
- Location: capture.go:46-54 (Record gains Attrs map[string]slog.Value, Keys unchanged); :149-173 (Handle populates attrs in the SAME single loop building line+keys, no second pass; last-write-wins via bound-then-call); :58-99 (value accessors AttrString/IntAttr/RequireDuration/HasAttr on Record); :204-211 (Sink.OnlyRecord); :34-37 (TestingT interface). Eleven files migrated to embed/use *logtest.Sink (alias/hooks/project/storelog store logging tests; cmd/bootstrap clean_sweep_summary; cmd state_daemon_cycle_summary; cmd config_migrate_logging; cmd state_hydrate_timeout/replayed; internal/state fifo_sweep_summary; internal/tmux portal_saver_lifecycle_events). cmd/logging_capture_test.go:42-52 shared wrapper.
- Notes: Five remaining `type …Sink struct` are genuinely-divergent convenience wrappers embedding *logtest.Sink (not duplicated cores). Enabled/WithAttrs/WithGroup/Handle/all exist in exactly one place. No local attrString/onlyRecord/all declarations remain. Store copy-acknowledgment comments gone. Leaf invariant holds.

TESTS:
- Status: Adequate
- Coverage: capture_test.go (RecordsAttrValues: bound component via WithAttrs + per-call int/duration land with correct Kind, Attrs key set agrees with Keys; LastWriteWins; AttrString/IntAttr/RequireDuration/HasAttr/OnlyRecord incl *testing.T-failing paths via fakeT/expectFail recover harness). The eleven migrated suites are the regression net, retaining prior assertions.
- Notes: Distinct concerns, no redundant happy-path. Keys-vs-Attrs agreement is a well-chosen invariant. Behaviour-focused.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; leaf test-only package documented + enforced; *testing.T-first accessor signatures match originals).
- SOLID: Good — single source of truth; divergent accessors stay in owning packages (TestingT is a 2-method subset). DRY past Rule-of-Three (eleven collapsed) without over-abstraction.
- Complexity: Low (Handle single linear traversal; zero-extra-traversal met).
- Modern idioms: Yes (struct embedding for wrappers; slog.Value.Kind() discrimination).
- Readability: Good — doc explains dual Keys+Attrs view + last-write-wins.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] main_panic_test.go:19-35 declares a local captureHandler (raw []slog.Record, different shape — passthrough WithAttrs, no bound-attr accumulation, no attrs map); intentionally outside this task's eleven-file scope, not a structured-attr-map duplicate. A future consolidation could decide whether to fold the main-package marker assertions onto logtest.Sink.
- [idea] The four store packages each retain a near-identical 4-line installCapture(t) *logtest.Sink helper (only doc-comment component differs); task permitted leaving per-package install helpers; a future micro-consolidation could lift a single logtest.Install(t) helper but would be premature given trivial body + cross-package import churn.
