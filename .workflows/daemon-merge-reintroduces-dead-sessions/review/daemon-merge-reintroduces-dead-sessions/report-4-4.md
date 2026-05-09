TASK: Eliminate redundant StaleMarkerCleaner adapter pass-through (4-4)

ACCEPTANCE CRITERIA:
- StaleMarkerCleaner adapter type in `internal/bootstrapadapter/` deleted.
- `staleClientStub` and related tests deleted.
- Production wiring uses inline `&bootstrap.MarkerCleanupCore{...}` matching `cleanStaleAdapter`/`saverAdapter`.
- Inventory comment lists exactly four currently-exported adapters.
- `*MarkerCleanupCore`-level coverage subsumes adapter-level cases.

STATUS: Complete

SPEC CONTEXT: Cycle-4 deduplication identified `StaleMarkerCleaner` as pure pass-through to `bootstrap.MarkerCleanupCore`. Since `*tmux.Client` directly satisfies every seam, the adapter was redundant glue.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/bootstrapadapter/adapters.go` — only `RestoringMarker`, `HookRegistrar`, `RestoreAdapter`, `FIFOSweeper` remain.
  - `cmd/bootstrap_production.go:123-128` — inline `&bootstrap.MarkerCleanupCore{Markers: client, Panes: client, Unsetter: client, Logger: logger}`.
  - `cmd/bootstrap_production.go:17` — inventory lists exactly four adapters.
  - `cmd/bootstrap_production.go:23-28` — rationale comment explains MarkerCleanupCore inline construction.
- Notes: No `.go` source file references `StaleMarkerCleaner`.

TESTS:
- Status: Adequate
- Coverage:
  - `staleClientStub` absent from all `.go` source files.
  - `adapters_internal_test.go` does not exist (deleted as planned).
  - `internal/bootstrapadapter/adapters_test.go` retains FIFOSweeper / RestoringMarker / HookRegistrar tests.
  - `cmd/bootstrap/stale_marker_cleanup_test.go` covers MarkerCleanupCore directly:
    - Normal cleanup: stale-unset, live-preservation, empty marker set, full overlap, no overlap.
    - Zero-panes guard Warn: `TestStaleMarkerCleanup_MassUnsetHazardGuard`.
    - List-skeleton-markers / show-options error path: `TestStaleMarkerCleanup_GenuineFailurePropagation`.
    - Plus soft-warning posture, paneKey normalisation, format pinning.

CODE QUALITY:
- Project conventions: Followed. Inline construction matches `cleanStaleAdapter`/`saverAdapter`.
- SOLID: Good. Removing redundant adapter improves SRP and ISP.
- Complexity: Low. Net deletion of glue code.
- Modern idioms: Yes.
- Readability: Good. Inventory comment explicitly documents WHY MarkerCleanupCore is inline.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
