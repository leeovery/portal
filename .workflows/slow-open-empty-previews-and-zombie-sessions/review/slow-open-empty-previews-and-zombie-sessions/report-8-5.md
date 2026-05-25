TASK: 8-5 — Replace sorted-map-keys helpers with slices.Sorted + maps.Keys

STATUS: Complete

SPEC CONTEXT: c2 F5. Three test-side helpers (`sortedSnapKeys`, `sortedKeys` test-variant, `sortedSnapshotKeys`) duplicated pattern. Go 1.21 `slices.Sorted(maps.Keys(m))` collapses inline.

IMPLEMENTATION:
- Status: Implemented
- Locations:
  - `cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go:207`
  - `cmd/state_daemon_self_supervision_integration_test.go:858-859`
  - `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:229, 296`
  - Imports `"maps"` + `"slices"` added in all three files
- `grep "func sortedSnapKeys|func sortedKeys|func sortedSnapshotKeys"` returns only `internal/state/capture.go:294` — unrelated production helper, explicitly out of scope

TESTS:
- Status: Adequate (pure refactor — existing integration tests are regression net)

CODE QUALITY:
- Project conventions: Followed
- Complexity: Reduced (net -LOC)
- Modern idioms: Yes — target met
- Readability: Good

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `internal/state/capture.go:294` `sortedKeys` is same pre-1.21 pattern on production hot path; could be converted for parity but out of T8-5 scope
