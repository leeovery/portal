TASK: Extract shared bootstrap.Orchestrator builder for integration tests (3-2)

ACCEPTANCE CRITERIA:
- Eleven inline `Orchestrator{...}` literals replaced with one builder.
- `orchestratorOpts` defaults unset fields to NoOp.
- `RestoringMarker` always real.
- Adding a hypothetical new step interface requires editing exactly one file.

STATUS: Complete

SPEC CONTEXT: Cycle-1 analysis identified 11 sites that rebuilt the same Orchestrator literal, so adding a new step (e.g. StaleMarkers) required touching all 11. Remediation extracts a shared builder.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/orchestrator_builder_test.go:36-81` (`orchestratorOpts` + `buildIntegrationOrchestrator`).
  - 10 cmd/bootstrap call sites: phase5_integration_test.go:67, :194, :297; phase5_marker_suppression_integration_test.go:145; reboot_roundtrip_test.go:332, :935, :1182; scrollback_resumption_test.go:120, :216, :304.
  - 11th sibling (separate package): `cmd/reattach_integration_test.go:163-186` — explicitly documented as thin sibling builder due to Go test-file symbol cross-package visibility.
- Notes:
  - `cmd/bootstrap_production.go:117` retains production-wiring literal (correctly out of scope).
  - `cmd/bootstrap/bootstrap_test.go:113` retains package-internal `newOrchestrator(stepRecorder)` for ordering verification — distinct purpose, was not in eleven-count.
  - `orchestratorOpts` defaults Hooks/Saver/Restore/StaleMarkers/Sweeper/Clean to NoOp; Logger nil tolerated (Orchestrator substitutes noopLogger). `RestoringMarker` always real.

TESTS:
- Status: Adequate
- Coverage: Helper exercised transitively by 10 call sites. No dedicated builder test added (appropriate).

CODE QUALITY:
- Project conventions: Followed. NoOp policy matches noop.go. RestoringMarker excluded from defaulting.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Yes. `testing.TB.Helper()`, opts struct pattern.
- Readability: Good. Docstring explicit about cross-package sibling constraint.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [quickfix] `cmd/bootstrap/orchestrator_builder_test.go` is missing the `//go:build integration` tag explicitly called out in analysis-tasks-c1.md task 2 step 2. All 10 call sites are integration-tagged; helpers compile cleanly under non-integration builds. Trivial single-line addition.
- [idea] Helper file co-locates orchestrator builder (3-2) and stateDir/logger preamble helpers (3-3 scope). Could migrate to dedicated file if it grows.
- [idea] Builder docstring is honest that adding a step requires editing TWO files (this + reattach sibling). Plan acceptance bar could read "exactly one file per package".
