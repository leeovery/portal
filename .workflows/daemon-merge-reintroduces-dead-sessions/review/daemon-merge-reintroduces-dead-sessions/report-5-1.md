TASK: Re-type MarkerCleanupCore.Markers to state.ServerOptionLister, eliminating closure adapter glue (5-1)

ACCEPTANCE CRITERIA:
- `MarkerCleanupCore.Markers` typed as `state.ServerOptionLister`.
- `CleanStaleMarkers` invokes `state.ListSkeletonMarkers(c.Markers)` internally.
- Both `markerListerFunc` declarations deleted (production + integration test).
- Inline construction collapses to `Markers: client,`.
- `MarkerLister` interface removed if no remaining consumers.
- Unit-test fakes ported to satisfy `ShowAllServerOptions() (string, error)`.

STATUS: Complete

SPEC CONTEXT: Cycle-3 architecture cleanup. Pre-task seam was asymmetric vs `FIFOSweeper`: bespoke single-method `MarkerLister` satisfied via duplicated `markerListerFunc` closures. Fix re-types field to canonical `state.ServerOptionLister`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/stale_marker_cleanup.go:69` — `Markers state.ServerOptionLister`.
  - `cmd/bootstrap/stale_marker_cleanup.go:118` — `state.ListSkeletonMarkers(c.Markers)` invoked internally.
  - `cmd/bootstrap_production.go:123-128` — inline construction is `Markers: client,`.
  - `cmd/bootstrap/scrollback_resumption_test.go:54-61` — `newProductionMarkerCleaner` builds struct directly with `Markers: client,`.
  - `internal/state/markers.go:26-28` — `state.ServerOptionLister` interface.
  - `internal/tmux/tmux.go:337` — `*tmux.Client.ShowAllServerOptions()` satisfies interface.
- Notes: All six acceptance bullets land. `MarkerLister` interface fully removed. Residual `markerListerFunc` mentions in comments are documentary only.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/bootstrap/stale_marker_cleanup_test.go:22-43` — `fakeMarkerLister` satisfies `state.ServerOptionLister`, synthesising raw tmux line format.
  - `fakeMarkerLister.err` propagation in `TestStaleMarkerCleanup_GenuineFailurePropagation` (843-887).
  - All happy-path / mass-unset / soft-warning / paneKey-normalisation tests updated.

CODE QUALITY:
- Project conventions: Followed. Field type mirrors sibling `FIFOSweeper.Client state.ServerOptionLister`.
- SOLID: Good. Interface segregation improved.
- Complexity: Low. `state.ListSkeletonMarkers(c.Markers)` invoked at call site.
- Modern idioms: Yes. Implicit interface satisfaction.
- Readability: Good. Field-level docstring explains migration rationale.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `cmd/bootstrap/stale_marker_cleanup.go:65` and `stale_marker_cleanup_test.go:18` retain `markerListerFunc` references in migration-narrative docstrings. Could collapse to one-line "Markers mirrors FIFOSweeper.Client" comment.
