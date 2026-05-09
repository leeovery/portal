TASK: Introduce stale-marker cleanup seam and happy-path implementation (2-1)

ACCEPTANCE CRITERIA:
- A new bootstrap step is inserted in the orchestrator (`cmd/bootstrap/`).
- The step enumerates markers via `state.ListSkeletonMarkers` and live panes via the error-propagating `(*tmux.Client).ListAllPanesWithFormat`.
- A new orchestrator seam interface is exposed with marker enumeration, live-pane enumeration, and marker-unset responsibilities each independently mockable.
- Unit tests (co-located) cover stale marker unset and live marker preservation.

STATUS: Complete

SPEC CONTEXT:
Fix Component B introduces a bootstrap step defending against stale `@portal-skeleton-*` markers. Seam exposes three independently mockable responsibilities, uses error-propagating `ListAllPanesWithFormat` (NOT `ListAllPanes`), and normalises live-pane entries via `state.SanitizePaneKey`.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - `cmd/bootstrap/stale_marker_cleanup.go` — `MarkerCleanupCore` (lines 61-73), `CleanStaleMarkers()` (110-165), `parseLivePaneSet` (177-210), `LivePaneLister` (24-26), `MarkerUnsetter` (31-33), `liveFormat` constant (39).
  - `cmd/bootstrap/bootstrap.go:88-104` — `MarkerCleaner` orchestrator-level seam interface.
  - `cmd/bootstrap/bootstrap.go:262-272` — dispatch the step at position 7.
  - `cmd/bootstrap/noop.go:46-54` — `NoOpMarkerCleaner` honours convention.
- Notes:
  - Three independently mockable seams: `state.ServerOptionLister`, `LivePaneLister`, `MarkerUnsetter`.
  - `liveFormat` constant pins canonical literal `#{session_name}:#{window_index}.#{pane_index}`.
  - `state.SanitizePaneKey` applied to live-side entries.
  - Untouched paths confirmed: `internal/state/capture.go`, `internal/restore/session.go`, `cmd/state_hydrate.go`.

TESTS:
- Status: Adequate
- Location: `cmd/bootstrap/stale_marker_cleanup_test.go`
- Coverage:
  - `TestCleanStaleMarkers_unsetsMarkerWhosePaneKeyIsNotInLiveSet` — stale unset.
  - `TestCleanStaleMarkers_leavesLiveMarkerAlone` — live preservation.
  - `TestCleanStaleMarkers_composesOptionNameFromSkeletonMarkerPrefix` — prefix composition.
  - `TestCleanStaleMarkers_requestsLivePanesWithCanonicalFormat` — pins literal format string.
  - Empty/full-overlap/no-overlap edges covered.
- Notes: Fakes scoped tightly. Reuses package's `recordingLogger`. Not over-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Each seam is single-purpose.
- Complexity: Low (~6).
- Modern idioms: `errors.Join`, `errors.Is`.
- Readability: Excellent. Numbered algorithm steps cross-referenced to spec.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `Logger` field mutated inside `CleanStaleMarkers` (`c.Logger = noopLogger{}` when nil) — same pattern as orchestrator's Run.
- [idea] `parseLivePaneSet` requires `logger` non-nil. A breadcrumb-callback parameter would tighten the contract.
- [quickfix] Doc comment on `MarkerCleanupCore` still describes "later tasks layer on..." — could be tightened to current state.
