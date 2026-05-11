TASK: killed-sessions-resurrect-on-restart-6-2 — Extract bootstrapadapter.NewRestoreAdapter constructor and adopt at four new integration-test sites

ACCEPTANCE CRITERIA:
- internal/bootstrapadapter exposes `NewRestoreAdapter(client *tmux.Client, stateDir string, logger *state.Logger) *RestoreAdapter` returning wrapped restore.Orchestrator.
- Four new sites no longer declare local restoreInner; each calls bootstrapadapter.NewRestoreAdapter inline.
- Seven pre-existing sites remain unchanged at task-completion time.

STATUS: Complete

SPEC CONTEXT: Phase 6 (Cycle 3), low-severity duplication. Every new bootstrap-integration test re-emits a five-line preamble. Cycle 3 extracts single logic-free constructor and migrates four new sites only.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Constructor: /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:107-115. Signature matches spec verbatim.
  - Four targeted sites adopted:
    - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:203
    - /Users/leeovery/Code/portal/cmd/bootstrap/eager_signal_hydrate_integration_test.go:329
    - /Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go:174
    - /Users/leeovery/Code/portal/cmd/bootstrap/phase5_marker_suppression_integration_test.go:118
  - Production wiring preserved at cmd/bootstrap_production.go:111-122.
  - Zero-value sentinel sites at orchestrator_builder_eager_default_test.go:50, :88 correctly left as &restore.Orchestrator{} literals.
- Notes: Current tree also shows adoption at reboot_roundtrip_test.go (×3), phase5_integration_test.go (×2), reattach_integration_test.go (×1) — migrated by later tasks 8-2 and 9-1, not regressions against 6-2 scope.

TESTS:
- Status: Adequate
- Coverage: Constructor is logic-free; transitively exercised by every migrated integration test. AC explicitly waives new test coverage.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Docstring explains role + production-exemption rationale.
- Issues: None blocking. Docstring inaccuracy targeted by task 7-2.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] After tasks 8-2 and 9-1 land, cmd/bootstrap_production.go becomes the sole open-coded site. Worth re-reading constructor docstring to confirm rationale still reads correctly.
