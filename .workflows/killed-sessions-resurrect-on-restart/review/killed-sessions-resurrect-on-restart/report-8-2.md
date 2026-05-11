TASK: killed-sessions-resurrect-on-restart-8-2 — Migrate buildReattachOrchestrator to NewRestoreAdapter and shared OpenTestLogger helper

ACCEPTANCE CRITERIA:
- buildReattachOrchestrator no longer constructs inline restore.Orchestrator literal — uses bootstrapadapter.NewRestoreAdapter.
- buildReattachOrchestrator no longer inline-creates the logger — uses shared OpenTestLogger helper.
- Helper importable from both cmd and cmd/bootstrap test packages.
- Delegate path preferred to minimise churn.
- Unused imports (restore, path/filepath, state) pruned if orphaned.

STATUS: Complete

SPEC CONTEXT: Phase 8 cycle 5 — structural cleanup of integration-test scaffolding around Phase 5 reattach AC. Collapses OpenLogger + Cleanup preambles and open-coded restore.Orchestrator two-step.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:164-174 — uses restoretest.OpenTestLogger(t, stateDir) and bootstrap.WithRestore(bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)).
  - /Users/leeovery/Code/portal/internal/restoretest/logger.go:28-36 — OpenTestLogger helper (exported, untagged).
  - /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:107-115 — NewRestoreAdapter.
- Notes:
  - `internal/restore` import pruned from cmd/reattach_integration_test.go.
  - path/filepath and internal/state imports retained — still legitimately used by surviving test bodies.
  - Helper reachable from both packages: cmd/reattach_integration_test.go:166 (cmd) and 12 sites under cmd/bootstrap/.
  - Delegate path preserved.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/internal/restoretest/logger_test.go:15-27 — TestOpenTestLogger_CreatesPortalLogUnderStateDir.
  - Seven TestReattachIntegration_* test functions continue to exercise the rebuilt helper end-to-end against real tmux.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel in cmd-package tests. Untagged file allowing both build tags.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Yes. t.Helper() / t.Cleanup() used correctly.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/bootstrap_production.go still uses open-coded form for parity with surrounding inline adapters — out of scope.
- [idea] newIntegrationStateDir split from OpenTestLogger could be revisited if every remaining site pairs them.
