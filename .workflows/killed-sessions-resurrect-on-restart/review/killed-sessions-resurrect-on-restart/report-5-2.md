TASK: killed-sessions-resurrect-on-restart-5-2 — Drop redundant explicit EagerSignaler wiring in three integration tests

ACCEPTANCE CRITERIA:
- Three integration tests stop wiring EagerSignaler explicitly.
- Every modified site still passes WithRestore with non-nil RestoreAdapter (precondition for auto-default).
- Remove any newly unused imports.

STATUS: Complete

SPEC CONTEXT: Phase 5 cycle 2 — with task 4-2's flipped default, three integration tests still wired EagerSignaler explicitly. Redundant.

IMPLEMENTATION:
- Status: Implemented
- Location (three sites with explicit EagerSignaler literal removed):
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:202-205 (AC1 sub-test)
  - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:325-331 (AC4 sub-test)
  - /Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:173-176
- All three call buildIntegrationOrchestrator with `Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)` — auto-default precondition satisfied.
- Imports verified clean: phase2 has no state/bootstrap package import; eager_signal retains state (used for state.ScrollbackFile/TailScrollback/SanitizePaneKey in AC4) but no bootstrap.EagerSignalCore code references.
- Repo-wide grep for `EagerSignaler: &bootstrap.EagerSignalCore` returns one hit at cmd/bootstrap_production.go:131 (production, out of scope).
- Out-of-scope NoOp opt-out sites (reboot_roundtrip_test.go:344/949/1200, phase5_integration_test.go:181/273) retained with multi-line task-4-2 rationale.

TESTS:
- Status: Adequate
- Auto-default contract pinned by:
  - cmd/bootstrap/orchestrator_builder_eager_default_test.go:35, :64, :83.
  - cmd/bootstrap/defaults_test.go:165.
- End-to-end coverage retained by the three modified integration tests themselves.

CODE QUALITY:
- ~18 lines deleted; single source of truth for default-selection in defaults.go.
- Explanatory comments at each modified site document reliance on auto-default.
- SOLID/DRY/readability improved.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] eager_signal_hydrate_integration_test.go:192-201 has a slightly verbose explanatory block; AC4 sub-test at line 325 already cross-references back. Could tidy to single canonical comment.
