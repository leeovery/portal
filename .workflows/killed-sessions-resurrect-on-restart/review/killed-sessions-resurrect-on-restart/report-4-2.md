TASK: killed-sessions-resurrect-on-restart-4-2 — Flip integration-orchestrator builder default for EagerSignaler from NoOp to a real adapter

ACCEPTANCE CRITERIA:
- Integration-orchestrator builder defaults EagerSignaler to a real *EagerSignalCore when a real Restore adapter is wired.
- NoOp default retained when no Restore adapter is supplied.
- Explicit WithEagerSignaler(NoOp) opt-out honoured.
- Sibling buildReattachOrchestrator inherits the flipped default via delegation.
- Tests that previously passed only because EagerSignaler was NoOp surface as explicit opt-outs.

STATUS: Complete

SPEC CONTEXT: Cycle 1 finding: previous test default of NoOp hid the eager pipeline in every integration test that didn't explicitly opt in — a regression in the eager step would not surface in CI. Flipping forces tests to exercise production-shape wiring; manual harnesses opt out explicitly.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap/defaults.go:181-196 — conditional default-selection branch.
  - /Users/leeovery/Code/portal/cmd/bootstrap/defaults.go:82-89 — restoreSet / eagerSignalerSet latches.
  - /Users/leeovery/Code/portal/cmd/bootstrap/defaults.go:51-54 — ServerSeam union interface.
  - /Users/leeovery/Code/portal/cmd/bootstrap/orchestrator_builder_test.go:68-109 — buildIntegrationOrchestrator delegates.
  - /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:164-174 — buildReattachOrchestrator delegates.
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:123-131 — production wiring unchanged.
- Notes: Dual-builder design is intentional and well-justified (Go test-file symbols not cross-package visible). Each opt-out site (phase5_integration_test.go:181,:273; reboot_roundtrip_test.go:344,:949,:1200) carries a "(task 4-2)" comment with concrete reason.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/bootstrap/orchestrator_builder_eager_default_test.go: three branches at helper level.
  - cmd/bootstrap/defaults_test.go:165-214: three branches at NewWithDefaults level + spot-checks real EagerSignalCore fields.
  - cmd/reattach_orchestrator_builder_test.go:29-44: sibling cmd-package builder.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Functional-options pattern; restoreSet/eagerSignalerSet latches address caller intent vs post-defaulting field value.
- Complexity: Low.
- Modern idioms: Yes. ServerSeam union avoids cmd/bootstrap importing internal/tmux.
- Readability: Excellent. Opt-out sites carry multi-line rationale with "(task 4-2)" tag.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Dual-builder rationale comment chain repeats the same justification three times. Could consolidate.
- [idea] EagerSignalCore type assertion is the same shape in two test files with slightly different error messages. Small shared helper could DRY.
