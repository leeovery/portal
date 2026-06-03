TASK: Fold recordingMigrationLogger onto the pre-existing recordingSlogHandler base (state-notify-cascade-on-binary-upgrade-2-4)

STATUS: Complete

ACCEPTANCE CRITERIA: WithAttrs/WithGroup/owner/shared scaffolding declared once (in recordingSlogHandler); recordingMigrationLogger embeds/wraps it; portal_saver_test.go / recordingSlogHandler UNCHANGED; typed component/reaped projections remain with identical behavior; full internal/tmux suite passes.

SPEC CONTEXT: Phase 2 test-only DRY cleanup; two package tmux_test slog.Handler capture helpers converged on identical scaffolding.

IMPLEMENTATION:
- Status: Implemented
- hooks_register_test.go:907-985 — recordingMigrationLogger is now a one-field struct embedding recordingSlogHandler (917-919); Enabled/Handle/WithAttrs/WithGroup/owner/shared/bound inherited (declared once). Prior near-duplicate handler methods gone. Logger() returns slog.New(&r.recordingSlogHandler). Typed projections preserved as read-time filters: infos() → "[<component>] <message>"; infoReaped() → reaped int64 positionally (-1 absent); warns() → LevelWarn. Behaviourally identical (project on read vs eager store).
- portal_saver_test.go:2626-2666 recordingSlogHandler UNCHANGED; all direct consumers untouched (still construct &recordingSlogHandler{} directly). Commit c28a7c5e was a workflow-state artifact, not a base-source edit.

TESTS:
- Status: Adequate. All six recordingMigrationLogger sites + migration suite + real-tmux suite exercise the embedded helper via infos()/infoReaped()/warns() with unchanged assertions. Full suite green. Read-time filtering equivalent to prior eager projection.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; test-only). SOLID: Good (base = capture-only; wrapper = projection-only). Complexity: Low. Readability: Good (doc comment documents embed + component-merge provenance + -1-when-absent reaped contract).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] infos()/infoReaped()/warns() re-scan r.records per call. Irrelevant for small test paths; no action.
