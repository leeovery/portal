TASK: Reclassify mass-unset hazard guard from error to warn-and-return-nil (3-6)

ACCEPTANCE CRITERIA:
- Zero-live-panes-with-markers branch returns `nil` and emits `Logger.Warn` with `ComponentBootstrap`.
- `ErrZeroLivePanesWithMarkers` sentinel deleted.
- Orchestrator no longer special-cases the sentinel.
- Mass-unset assertion preserved.
- Genuine `ListAllPanesWithFormat` error still propagates.

STATUS: Complete

SPEC CONTEXT: Phase 2 originally returned typed sentinel `ErrZeroLivePanesWithMarkers` for zero-live-panes-with-markers, requiring orchestrator-side `errors.Is` special-casing. Cycle-1 analysis flagged this as conflating soft deferral with genuine dependency failure. Task 3-6 reclassifies to `nil` + `Logger.Warn`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/stale_marker_cleanup.go:110-165` — `CleanStaleMarkers` returns `nil` for both zero-live-with-markers and zero-live-zero-markers branches; emits `c.Logger.Warn(state.ComponentBootstrap, ...)` at lines 141-144.
  - `cmd/bootstrap/errors.go` — no `ErrZeroLivePanesWithMarkers` sentinel.
  - `cmd/bootstrap/bootstrap.go:262-272` — orchestrator step 7 logs+swallows non-nil err with no sentinel special-casing.
  - Genuine errors propagate: `ListSkeletonMarkers` (line 120), `ListAllPanesWithFormat` (line 125), per-marker unset failures via `errors.Join` (161-163).
- Notes: Codebase-wide grep for `ErrZeroLivePanes` returns no matches in `cmd/` or `internal/`.

TESTS:
- Status: Adequate
- Coverage in `cmd/bootstrap/stale_marker_cleanup_test.go`:
  - lines 404-453 — `nil` + Warn entry containing deferral signature with component `state.ComponentBootstrap`.
  - lines 473-497 — guard runs before any unset (whitespace-only output).
  - lines 763-812 — guard fires when all lines malformed and markers exist.
  - lines 455-471 — clean no-op when zero markers + zero panes.
  - lines 379-402, 499-528 — `ListAllPanesWithFormat` error propagation via `errors.Is`.
  - lines 843-887 — `TestStaleMarkerCleanup_GenuineFailurePropagation` table-driven test for both error paths.
  - Mass-unset assertions ("zero unset calls") preserved in all branches.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. `MarkerCleanupCore` single responsibility.
- Complexity: Low. Linear control flow.
- Modern idioms: Yes. `errors.Join`.
- Readability: Excellent. Docstring explicitly justifies deferral-vs-error distinction.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `c.Logger` lazily mutated to `noopLogger{}` inside method. Matches package convention but could use local-variable pattern.
- [idea] Deferral Warn message format string not factored to constant. Tests grep for substrings, coupling test-to-implementation textually.
