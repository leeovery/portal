TASK: Mass-unset hazard guard for zero-panes and enum errors (2-3)

ACCEPTANCE CRITERIA:
- Cleanup skips unset pass and emits soft warning when `ListAllPanesWithFormat` returns error OR returns zero panes.
- Markers untouched in both branches.
- Guard precedes any unset call.
- Phase 5 reclassification: zero-panes-with-markers returns `nil` + `Logger.Warn(state.ComponentBootstrap)`.

STATUS: Complete

SPEC CONTEXT:
Spec §Fix Component B (Soft-Warning Posture, Mass-unset hazard guard): never treat empty live-pane result as authoritative. Phase 5 task 3-6 reclassified zero-live-panes deferral from sentinel error to `nil` + Warn.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/stale_marker_cleanup.go:110-165`
  - Lines 123-126: `ListAllPanesWithFormat` error returned verbatim, no unset.
  - Lines 128-145: Mass-unset hazard guard — when `len(live) == 0`:
    - Returns `nil` immediately if zero markers (clean no-op).
    - Otherwise emits `Logger.Warn(state.ComponentBootstrap, ...)` and returns `nil`.
  - Line 147+: per-marker unset only reached when `len(live) > 0`.
- Notes:
  - Phase 5 reclassification honoured — no `ErrZeroLivePanesWithMarkers` sentinel.
  - Markers untouched in both deferral branches.
  - Whitespace-only output handled.
  - Genuine errors still propagate.

TESTS:
- Status: Adequate
- Location: `cmd/bootstrap/stale_marker_cleanup_test.go`
- Coverage in `TestStaleMarkerCleanup_MassUnsetHazardGuard` (378-529):
  - Enum-error path → `errors.Is(err, sentinel)` + zero unsets.
  - Zero-live-panes-with-markers → `nil` + Warn body containing `"stale-marker cleanup"` and `"2 marker(s)"`, plus `state.ComponentBootstrap`.
  - Zero+zero no-op.
  - Guard runs before any unset.
  - Never mass-unsets on tmux failure.
- `TestStaleMarkerCleanup_GenuineFailurePropagation` (843-887) reinforces error-channel-only-carries-genuine-failures invariant.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — single responsibility.
- Complexity: Low (~6 branches).
- Modern idioms: Yes — `errors.Join`, `errors.Is`, `map[string]struct{}`.
- Readability: Good. Doc explains rationale for `nil`+Warn.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `liveFormat` literal duplicated in test assertion and producer doc. Intentional pinning; no action.
- [idea] Deferral Warn body asserted via substring. Appropriately loose.
