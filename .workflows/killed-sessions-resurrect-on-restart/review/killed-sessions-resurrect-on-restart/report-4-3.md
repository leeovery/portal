TASK: killed-sessions-resurrect-on-restart-4-3 — Promote NoOp-defaulted orchestrator builder helper to non-test location to eliminate dual builders

ACCEPTANCE CRITERIA:
- Single non-test helper centralises NoOp-defaulting policy.
- Previously duplicated builders both delegate to that helper.
- Edge case: NoOp types must not leak into production callers.
- Edge case: Mandatory seams positional, not options.

STATUS: Complete

SPEC CONTEXT: Phase 4 cycle 1 finding "Dual integration-orchestrator builders forced into manual lock-step" — adding a new step interface required touching every literal across both helpers. Fix: promote helper to non-test file in cmd/bootstrap.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/bootstrap/defaults.go (new) — public NewWithDefaults + Option type + seven With* constructors + ServerSeam union.
  - /Users/leeovery/Code/portal/cmd/bootstrap/orchestrator_builder_test.go:68-109 — buildIntegrationOrchestrator delegates.
  - /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:164-174 — buildReattachOrchestrator delegates.
  - /Users/leeovery/Code/portal/cmd/bootstrap_production.go:117-150 — production path unchanged; direct construction.
- Notes:
  - Mandatory seams positional: server (ServerSeam), stateDir, logger (nil tolerated), restoring. Matches edge case.
  - Degradable seams variadic.
  - EagerSignaler default-selection preserved via restoreSet/eagerSignalerSet latches.
  - ServerSeam (ServerBootstrapper + state.ServerOptionLister) defined in cmd/bootstrap avoids forcing tests to import internal/tmux.
  - NoOp leakage: cmd/bootstrap_production.go does NOT call NewWithDefaults; constructs Orchestrator literal directly. No leak introduced.

TESTS:
- Status: Adequate
- Coverage (cmd/bootstrap/defaults_test.go, 259 lines):
  - TestNewWithDefaults_DefaultsAllDegradableStepsToNoOp
  - TestNewWithDefaults_WiresPositionalSeams
  - TestNewWithDefaults_HonorsAllWithOptions
  - TestNewWithDefaults_EagerSignalerDefaultsToRealWhenRestoreReal
  - TestNewWithDefaults_EagerSignalerDefaultsToNoOpWhenRestoreUnset
  - TestNewWithDefaults_EagerSignalerExplicitOptOutHonored
  - TestNewWithDefaults_RunCallableSmokeTest

CODE QUALITY:
- Project conventions: Followed. defaults.go imports only internal/state for ServerOptionLister + DefaultFIFOSignaler.
- SOLID: SRP — defaults.go owns composition; ordering lives in bootstrap.go. OCP — adding step needs one With* + one default branch. DIP — accepts abstractions at every positional seam.
- Complexity: Low.
- Modern idioms: Functional options + private defaultsConfig carrier.
- Readability: Excellent.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] defaults.go hardcodes state.DefaultFIFOSignaler{} in the conditional real EagerSignalCore default. Future test wanting to inject a recording FIFOSignaler would need a new With* constructor.
- [idea] ServerSeam union couples helper signature to EagerSignalCore.Markers shape. Splitting "positional server" into two positional args would scale better.
- [quickfix] orchestrator_builder_test.go:33-54 orchestratorOpts comment still describes legacy "default-selection has one branch" logic; could trim to one-line pointer at NewWithDefaults.
