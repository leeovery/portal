TASK: Wire production adapter in `internal/bootstrapadapter/` (2-6)

ACCEPTANCE CRITERIA:
- A new orchestrator seam interface with marker enumeration, live-pane enumeration, and marker-unset responsibilities each independently mockable.
- Production adapter wired in `internal/bootstrapadapter/`.
- Production-adapter wiring covered in `internal/bootstrapadapter/adapters_test.go`.
- Error-propagating `ListAllPanesWithFormat` used; `ListAllPanes` NOT used.
- `UnsetServerOption(SkeletonMarkerPrefix + paneKey)` used.

STATUS: Complete

SPEC CONTEXT: Spec §Fix Component B — Adapter Wiring requires three independently-mockable seam responsibilities. Phase 4 task 4-4 collapsed the StaleMarkerCleaner adapter inline (matching `saverAdapter`/`cleanStaleAdapter`), and Phase 5 task 5-1 made `*tmux.Client` directly satisfy all three seams.

IMPLEMENTATION:
- Status: Implemented (intentional architectural evolution per phase 4-4 / 5-1)
- Location:
  - Seam interfaces: `cmd/bootstrap/stale_marker_cleanup.go:24-32` (`LivePaneLister`, `MarkerUnsetter`); `internal/state/markers.go:26` (`ServerOptionLister`).
  - Core: `cmd/bootstrap/stale_marker_cleanup.go:61-73` (`MarkerCleanupCore`).
  - Cleanup logic: `cmd/bootstrap/stale_marker_cleanup.go:110-165` (uses `state.ListSkeletonMarkers(c.Markers)`, `Panes.ListAllPanesWithFormat(liveFormat)`, `Unsetter.UnsetServerOption(state.SkeletonMarkerPrefix + paneKey)`).
  - Production wiring: `cmd/bootstrap_production.go:123-128` — inline `&bootstrap.MarkerCleanupCore{Markers: client, Panes: client, Unsetter: client, Logger: logger}`.
- Notes: No `StaleMarkerCleaner` adapter in `internal/bootstrapadapter/` (collapsed inline). Error-propagating `ListAllPanesWithFormat` used; `ListAllPanes` NOT used.

TESTS:
- Status: Adequate
- Coverage:
  - Error propagation from `ListAllPanesWithFormat`: `cmd/bootstrap/stale_marker_cleanup_test.go:378-402` (sentinel-wrap via `errors.Is`).
  - `ShowAllServerOptions` error: `fakeMarkerLister.err` exercised across suite.
  - Canonical format pinning: `TestCleanStaleMarkers_requestsLivePanesWithCanonicalFormat`.
  - Marker name composition: `TestCleanStaleMarkers_composesOptionNameFromSkeletonMarkerPrefix`.
  - Mass-unset hazard guard preserved.
  - Orchestrator wiring/ordering: `cmd/bootstrap/bootstrap_test.go:646+`.
  - End-to-end production-shape: `cmd/bootstrap/scrollback_resumption_test.go:54` (`newProductionMarkerCleaner` constructs identical `MarkerCleanupCore` against real `*tmux.Client`).

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Three independently-mockable seams.
- Complexity: Low.
- Modern idioms: Yes. `errors.Join`, `errors.Is`, `strings.LastIndex`.
- Readability: Good. Package-level docstring at `cmd/bootstrap_production.go:3-28` explains two-home split.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] No explicit `var _ bootstrap.MarkerCleaner = (*MarkerCleanupCore)(nil)` assertion. Structural assignment at `cmd/bootstrap_production.go:123` is sufficient compile-time enforcement, but explicit assertion would lock contract at type's declaration site.
