TASK: killed-sessions-resurrect-on-restart-9-1 — Collapse open-coded RestoreAdapter preamble and openTestLogger shim across cmd/bootstrap tests

ACCEPTANCE CRITERIA:
- All five RestoreAdapter open-coded preambles in phase5_integration_test.go and reboot_roundtrip_test.go collapsed to bootstrapadapter.NewRestoreAdapter.
- grep "restore.Orchestrator{" over those two files returns no hits.
- openTestLogger shim in orchestrator_builder_test.go deleted.
- restoretest.OpenTestLogger is the only named call site for the helper across cmd/bootstrap.
- Production wiring at cmd/bootstrap_production.go untouched.
- Zero-value sentinels in orchestrator_builder_eager_default_test.go:50,88 preserved.
- Orphaned restore import pruned.

STATUS: Complete

SPEC CONTEXT: Phase 9 cycle 6 — pure test-code DRY cleanup. Cycle-6 duplication agent flagged five sites still open-coding the two-step preamble that NewRestoreAdapter (added in 8-2) exists to replace, plus residual openTestLogger shim.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/bootstrap/phase5_integration_test.go:166-169, 254-257 — both sites collapsed.
  - cmd/bootstrap/reboot_roundtrip_test.go:325-328, 931-934, 1178-1186 — three sites collapsed.
  - cmd/bootstrap/orchestrator_builder_test.go — wrapper deleted; docstring at line 116 references restoretest.OpenTestLogger.
  - cmd/bootstrap/orchestrator_builder_eager_default_test.go:50, :88 — zero-value sentinels preserved.
  - cmd/bootstrap_production.go — untouched.
  - internal/restore import removed from phase5_integration_test.go and reboot_roundtrip_test.go; retained in orchestrator_builder_eager_default_test.go for sentinels.
- Notes: 12 restoretest.OpenTestLogger call sites; 9 NewRestoreAdapter adoptions across cmd/bootstrap (broader than the five cited — eager_signal_hydrate_integration_test.go, phase5_marker_suppression_integration_test.go, phase2_hook_fire_integration_test.go also benefited).

TESTS:
- Status: Adequate (covered transitively)
- Coverage: Setup-refactor only — TestPhase5_*, TestRebootRoundTrip_*, TestPhase1Integration_EagerSignalHydrate_* suites continuing to pass.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel introduced.
- SOLID: Good. Small DIP win at the seam.
- Complexity: Low. Net deletion.
- Modern idioms: Yes.
- Readability: Improved. 5-line preamble → 1-line constructor.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Cycle-6 finding cited five sites; migration touched nine NewRestoreAdapter call sites. Worth noting for future audit reconciliation.
- [idea] orchestrator_builder_eager_default_test.go still imports internal/restore for sentinels. A future bootstrapadapter.NewZeroRestoreAdapter() could drop the last restore import in cmd/bootstrap tests.
