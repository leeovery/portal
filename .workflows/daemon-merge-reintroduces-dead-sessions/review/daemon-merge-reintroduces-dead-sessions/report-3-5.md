TASK: Resolve StaleMarkerCleaner dual-type collision and per-call inner construction (3-5)

ACCEPTANCE CRITERIA:
- Rename `cmd/bootstrap.StaleMarkerCleaner` to `MarkerCleanupCore`.
- `bootstrapadapter.StaleMarkerCleaner` constructs inner cleaner once.
- `MarkerCleaner` interface name unchanged.
- No two types named `StaleMarkerCleaner` in the codebase.
- Collision-avoidance doc removed.

STATUS: Complete

SPEC CONTEXT: C1 architecture analysis flagged dual-type collision and per-call inner construction. Task 3-5 renamed bootstrap concrete type to `MarkerCleanupCore`. Phase 4-4 then collapsed redundant adapter pass-through entirely.

IMPLEMENTATION:
- Status: Implemented (final state reflects 3-5 + 4-4 follow-on)
- Location:
  - `cmd/bootstrap/stale_marker_cleanup.go:61` — type renamed to `MarkerCleanupCore`.
  - `cmd/bootstrap/stale_marker_cleanup.go:110` — `func (c *MarkerCleanupCore) CleanStaleMarkers() error`.
  - `cmd/bootstrap/bootstrap.go:102` — `MarkerCleaner` interface name preserved.
  - `cmd/bootstrap/bootstrap.go:171` — `StaleMarkers MarkerCleaner` field on Orchestrator.
  - `cmd/bootstrap_production.go:123-128` — inline `&bootstrap.MarkerCleanupCore{Markers: client, Panes: client, Unsetter: client, Logger: logger}` (per 4-4 inlining).
  - `internal/bootstrapadapter/adapters.go` — no `StaleMarkerCleaner` type remains (verified via grep).
- Notes: Zero production-Go matches for `StaleMarkerCleaner` anywhere (only workflow docs and tasks.jsonl). No collision-avoidance caveat language.

TESTS:
- Status: Adequate
- Coverage: Test sites in `stale_marker_cleanup_test.go` and `scrollback_resumption_test.go` reference only `MarkerCleanupCore`. `NoOpMarkerCleaner` consistent. End-to-end production-shape via `newProductionMarkerCleaner`.

CODE QUALITY:
- Project conventions: Followed. "Core" suffix differentiates concrete struct from `MarkerCleaner` interface.
- SOLID: Good. Single-method interface; clear single responsibility.
- Complexity: Low. Mechanical renames.
- Modern idioms: Yes. `*tmux.Client` directly satisfies seams via structural typing.
- Readability: Good. Cross-reference at bootstrap.go:94-96 makes interface↔concrete explicit.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [quickfix] `cmd/bootstrap/stale_marker_cleanup.go:49-51` doc still says "later tasks layer on..." — forward-reference is stale (all those tasks landed). Same comment as report-2-1.
