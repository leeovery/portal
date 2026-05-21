# Analysis — Duplication (cycle 1)

STATUS: findings
FINDINGS_COUNT: 2

## Findings

### killerStub (internal pkg) vs mockSessionKiller (external pkg) — structural twin
- SEVERITY: low
- FILES: internal/tui/kill_refresh_filter_test.go:11-21, internal/tui/model_test.go:721-730
- DESCRIPTION: `killerStub` in the new internal-package test (`package tui`) is a byte-equivalent reimplementation of `mockSessionKiller` in `model_test.go` (`package tui_test`). Both are 3-field structs (`killedName string`, `err error`) with an identical single-method `KillSession(name string) error` body. The spec explicitly flagged this as a known consequence of the Go package boundary — `mockSessionKiller` lives in `tui_test` and is not importable from `tui`. Moving `kill_refresh_filter_test.go` to `tui_test` would lose access to unexported model state the test depends on. Exporting a test fake from `tui` would bleed test scaffolding into the production surface. With N=2 across a deliberate package boundary, Rule-of-Three is not met.
- RECOMMENDATION: No extraction. Add a one-line cross-reference comment on `killerStub` so future maintainers know the twin exists and why.

### Filter-commit setup block duplicated between the two new tests
- SEVERITY: low
- FILES: internal/tui/pagepreview_refetch_test.go:313-317, internal/tui/kill_refresh_filter_test.go:67-71
- DESCRIPTION: Both tests open with the same five-line "commit a filter" sequence — `SetFilterText("alpha")`, `SetFilterState(list.FilterApplied)`, `IsFiltered()` invariant guard. The spec explicitly directs the kill-refresh test to mirror the preview test's drive, so identical shape was intentional. N=2 sits below Rule-of-Three. The post-refresh assertion triplet is also a near-duplicate.
- RECOMMENDATION: No action this cycle. If a third filter-aware refresh test lands, extract `commitSessionFilter(t, m, text)` and `assertFilterIntact(t, m, want)` into a shared helpers file.

## Summary

Two low-severity test-only duplications. The `killerStub` ↔ `mockSessionKiller` twin is a deliberate, spec-acknowledged consequence of the internal/external test-package split. Filter-commit setup is N=2, below Rule-of-Three. No production-code duplication.
