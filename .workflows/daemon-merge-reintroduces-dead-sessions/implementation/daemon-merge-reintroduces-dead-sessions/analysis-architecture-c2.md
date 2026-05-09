AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 3

FINDINGS:

- FINDING: MarkerCleanupCore.Logger typed as *state.Logger breaks the orchestrator's Logger-interface convention
  SEVERITY: low
  FILES: cmd/bootstrap/stale_marker_cleanup.go:62, cmd/bootstrap/bootstrap.go:139-159
  DESCRIPTION: Every other step seam in cmd/bootstrap depends on the abstract `Logger` interface defined in bootstrap.go (Debug/Warn/Error). The Orchestrator carefully substitutes `noopLogger{}` so step sites can call Logger methods unconditionally. `MarkerCleanupCore` is the only piece of bootstrap-package logic that types its Logger field as the concrete `*state.Logger`, relying on `*state.Logger`'s nil-receiver no-op. The asymmetry imposes a directly observable testability cost: `stale_marker_cleanup_test.go` must spin up a real `state.OpenLogger` to a tempfile (`openZeroPanesGuardLogger` helper) and re-read the file to assert Warn fired — instead of injecting a `recordingLogger{}` like the orchestrator-level tests do. The bootstrap package already imports `internal/state` for `ComponentBootstrap`, so switching to the abstract interface costs nothing on the import graph.
  RECOMMENDATION: Re-type `MarkerCleanupCore.Logger` as `bootstrap.Logger`. Replace `openZeroPanesGuardLogger`/`readLogFile` plumbing in tests with the existing `recordingLogger{}` and assert the formatted Warn message contents in-memory.

- FINDING: bootstrapadapter.StaleMarkerCleaner is a redundant pass-through over bootstrap.MarkerCleanupCore
  SEVERITY: low
  FILES: internal/bootstrapadapter/adapters.go:201-245, cmd/bootstrap_production.go:113-128
  DESCRIPTION: `bootstrap.MarkerCleanupCore` already exposes the public `CleanStaleMarkers()` method that satisfies `bootstrap.MarkerCleaner`, with all four wiring fields exported. The adapter `StaleMarkerCleaner` adds nothing beyond a constructor that builds those four fields — its `CleanStaleMarkers()` is a one-line pass-through. The sibling `FIFOSweeper` IS the implementation (no inner core), and `RestoreAdapter` wraps a non-bootstrap struct (`*restore.Orchestrator`) for method-name shaping — both have justified existence. `StaleMarkerCleaner` instead introduces (a) an unexported `staleMarkerClient` interface, (b) a `markerListerFunc` closure type, (c) a `NewStaleMarkerCleaner` constructor, and (d) a wrapper struct with one field — solely to forward to a struct that could be constructed directly at the cmd/bootstrap_production.go wiring site with a four-line struct literal.
  RECOMMENDATION: Either (a) delete `bootstrapadapter.StaleMarkerCleaner` and construct `&bootstrap.MarkerCleanupCore{...}` inline at cmd/bootstrap_production.go (parallel to how `cleanStaleAdapter` and `saverAdapter` live there); OR (b) inline the cleanup logic directly into `bootstrapadapter.StaleMarkerCleaner` (parallel to `FIFOSweeper`) and remove `MarkerCleanupCore` entirely. Either restructure removes one full layer of indirection. Option (a) is the lighter restructure since the unit tests already cover `*MarkerCleanupCore` directly.

- FINDING: bootstrap_production.go top-of-file adapter inventory drift
  SEVERITY: low
  FILES: cmd/bootstrap_production.go:16-17
  DESCRIPTION: The file-leading comment enumerates the adapters in `internal/bootstrapadapter` as "HookRegistrar, RestoringMarker, RestoreAdapter, FIFOSweeper". `StaleMarkerCleaner` (added in this work unit) is not listed. The comment is the canonical "where do new adapters go?" reference for future contributors.
  RECOMMENDATION: Add `StaleMarkerCleaner` to the inventory list at line 17. (If finding #2 is acted on by inlining or relocating, update accordingly.)

SUMMARY: Cycle-1 fixes verified clean. Three remaining low-severity concerns: a Logger-field type asymmetry that imposes a testability cost, a redundant adapter pass-through over MarkerCleanupCore, and stale inventory documentation in bootstrap_production.go. None are blocking.
