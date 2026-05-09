TASK: Re-type MarkerCleanupCore.Logger to the bootstrap.Logger interface (4-3)

ACCEPTANCE CRITERIA:
- `MarkerCleanupCore.Logger` field typed as `bootstrap.Logger` (interface), not `*state.Logger`.
- Production wiring untouched; `*state.Logger` satisfies `bootstrap.Logger` structurally; no compile break.
- Helper removal conditional on no remaining references.
- Warn assertions on component (`ComponentBootstrap`) and marker count preserved.
- Tests use `recordingLogger{}` convention.

STATUS: Complete

SPEC CONTEXT: Cycle-2 architecture analysis flagged `MarkerCleanupCore.Logger` typed as concrete `*state.Logger` as deviation from orchestrator-wide convention. Tests forced to plumb real on-disk logger via tempfile + readback. Fix re-types field so tests inject in-memory `recordingLogger{}`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/stale_marker_cleanup.go:72` — `Logger Logger` (typed as package-local `bootstrap.Logger` interface).
  - `cmd/bootstrap/stale_marker_cleanup.go:114-116` — nil-substitution to `noopLogger{}`.
  - `cmd/bootstrap/bootstrap.go:139-143` — `Logger` interface (Debug/Warn/Error).
  - `internal/state/logger.go:222,232,237` — `*state.Logger` exposes matching signatures.
  - `cmd/bootstrap_production.go:123-128` — production wiring unchanged.
  - `cmd/bootstrap/scrollback_resumption_test.go:54-61` — `newProductionMarkerCleaner` assigns concrete to interface field.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/bootstrap/stale_marker_cleanup_test.go:404-453` — injects `&recordingLogger{}`, asserts Warn body contains "stale-marker cleanup" AND "2 marker(s)" and `warnComponents[i] == state.ComponentBootstrap`.
  - lines 763-812 — same component + deferral-signature assertions for malformed lines branch.
  - lines 814-834 — nil-Logger no-panic case preserved.
- Notes: No `openZeroPanesGuardLogger` / `readLogFile` helpers remain in test code. `recordingLogger` defined at `cmd/bootstrap/bootstrap_test.go:86-108` is the existing in-package fake.

CODE QUALITY:
- Project conventions: Followed. Matches orchestrator-wide pattern.
- SOLID: Good. Dependency Inversion applied uniformly.
- Complexity: Low.
- Modern idioms: Yes. Structural interface satisfaction.
- Readability: Good. Field docstring explains contract.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] No explicit compile-time assertion `var _ bootstrap.Logger = (*state.Logger)(nil)`. Production wiring provides equivalent enforcement; explicit assertion would create reverse import.
