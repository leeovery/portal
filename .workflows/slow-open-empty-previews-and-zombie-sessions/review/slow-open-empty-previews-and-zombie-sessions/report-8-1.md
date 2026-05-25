TASK: 8-1 — Replace local fingerprint helpers in composition_e2e_self_eject_integration_test.go with portaltest helpers

STATUS: Complete

SPEC CONTEXT: Analysis-cycle refactor (duplication-c2#1). T6-6 authored ~135 LOC of fingerprint diff/format/sort helpers concurrently with T7-1's consolidation. Goal: single source of truth across cmd/bootstrap.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/composition_e2e_self_eject_integration_test.go:349-361`
- Body inlined at call site using `portaltest.DiffFingerprints(snapBefore, snapAfter)` + `portaltest.FormatDelta(d)`
- Mirrors composition_abc canonical pattern with strictly-additive `strings.Join` indent for diag readability
- Also adopted `portaltest.SnapshotStateDir` (224, 338) and `portaltest.ReadPortalLogSafe` (311, 331)
- Repo-wide grep for the five removed helper names returns only workflow docs

TESTS:
- Status: Adequate
- `TestCompositeBootstrap_ExternalSaverKillTriggersSelfEject` preserves all original assertions (delta count gate, per-delta formatted diag, fatal on non-empty, pre/post snapshot pair, log blob, daemon.pid presence + PID-retention, INFO-marker substring)
- No new tests required — pure dedup refactor; `internal/portaltest/fingerprint_diff_test.go` covers `DiffFingerprints`/`FormatDelta` directly

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; build tag retained
- SOLID: Good; single source of truth; test responsibility narrowed back to eject scenario
- Complexity: Low; call site ~13 lines
- Modern idioms: `make + strings.Join` diag block
- Readability: Good; load-bearing comments untouched; assertion reads identically to composition_abc sibling

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Compile-time anchor `var _ = errors.Is` at line 432 — `errors` package not used inline post-refactor; anchor keeps import alive
