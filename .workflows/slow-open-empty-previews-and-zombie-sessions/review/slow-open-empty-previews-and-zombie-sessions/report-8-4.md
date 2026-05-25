TASK: 8-4 — Unify recordingLogger and captureLogger into a single Logger fake

STATUS: Complete

SPEC CONTEXT: c2 duplication-finding cleanup. `analysis-duplication-c5.md:14`: "captureLogger deleted; single exported `bootstrap.RecordingLogger` consumed everywhere." Original cross-package justification no longer applied — both fakes lived in same package.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/bootstrap_test.go:89-145` — exported `RecordingLogger` struct with `Debug/Info/Warn/Error` + new `AllEntries() []string` aggregator (128-145) spanning all four level slices in DEBUG/INFO/WARN/ERROR fixed order
  - `cmd/bootstrap/composition_e2e_convergence_integration_test.go:87,175-188` — migrated; uses `bootstrap.RecordingLogger` and `logger.AllEntries()`
  - `cmd/bootstrap/orphan_sweep_integration_test.go:233,245-250,432` — migrated + compile-time guard `var _ bootstrap.Logger = (*bootstrap.RecordingLogger)(nil)`
  - `cmd/bootstrap/upgrade_path_integration_test.go:82` — header references exported fake
- `captureLogger` fully absent (grep returns zero matches)
- Type promoted to exported so cross-package `bootstrap_test` files can share
- Internal consumers (`bootstrap_test.go` ~20 sites, `orphan_sweep_test.go` ~12, `stale_marker_cleanup_test.go` 2, `eager_signal_hydrate_test.go` 1) use same exported type

TESTS:
- Status: Adequate
- No new test (refactor whose acceptance is "existing tests pass")
- Compile-time interface assertion durable guard

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; single responsibility; minimal exported surface
- Complexity: Low; `AllEntries` single linear pass with pre-sized capacity
- Modern idioms: Yes
- Readability: Good; doc warns level-order not chronological

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `AllEntries()` aggregates messages but not components; future test needing "string X at DEBUG under ComponentBootstrap" must reach into unexported slice or add parallel accessor
- [idea] Level ordering fixed (DEBUG→INFO→WARN→ERROR), not chronological; separate chronological accessor would be cleaner if ordering-sensitive tests needed
