# Implementation Review: Daemon Merge Reintroduces Dead Sessions

**Plan**: daemon-merge-reintroduces-dead-sessions
**QA Verdict**: Approve

## Summary

The implementation cleanly resolves the bug across both fix components and four follow-up analysis cycles. Fix Component A (live-set filter in `mergeSkippedPanes`) lands as a single-file change in `internal/state/capture.go` with structural filtering at session, window, and pane levels via a locally-built map; helpers remain pristine; the buggy codifying test is replaced with its inverse and a kill-mid-flight self-heal regression test mirrors the empirical scenario. Fix Component B (bootstrap stale-marker cleanup step) lands as a new step 7 in the orchestrator with three independently-mockable seams (`state.ServerOptionLister`, `LivePaneLister`, `MarkerUnsetter`), error-propagating live-pane enumeration, mass-unset hazard guard, soft-warning posture matching `CleanStale`, and an end-to-end scrollback-save resumption integration test. The four analysis cycles successfully tightened the architecture (renamed `StaleMarkerCleaner` → `MarkerCleanupCore`, eliminated redundant adapter pass-through, re-typed seams to canonical interfaces, reclassified the zero-panes guard from sentinel error to `nil`+Warn) and aligned step numbering nomenclature throughout. All 24 tasks verify Complete with zero blocking issues.

## QA Verification

### Specification Compliance

Implementation aligns with specification. Both fix components match the spec exactly:

- **Fix Component A**: `mergeSkippedPanes` filters at all three structural levels using a locally-built `buildLiveStructure(idx)` map; public signature unchanged; helpers untouched. All eight phase 1 acceptance criteria met.
- **Fix Component B**: New cleanup step inserted between step 6 (Clear `@portal-restoring`) and the existing FIFO sweep step, renumbered to nine-step framing with CleanStaleMarkers as step 7. Uses error-propagating `ListAllPanesWithFormat` (not the swallowing `ListAllPanes`). Mass-unset hazard guard prevents the empty-live-set failure mode. Soft-warning posture matches `CleanStale`. PaneKey normalisation correctness fixture (`session:window.pane` ↔ `session__window.pane`) is in place with positive recognition, negative non-equivalence, and rightmost-colon split coverage. All eleven phase 2 acceptance criteria met.

The four analysis-cycle phases (3, 4, 5) successfully addressed duplication, standards drift, and architectural asymmetries surfaced during implementation review. Final architecture is symmetric with `FIFOSweeper` (canonical `state.ServerOptionLister` seam, inline production wiring matching `cleanStaleAdapter`/`saverAdapter`).

### Plan Completion

- [x] Phase 1 acceptance criteria met (5/5 tasks)
- [x] Phase 2 acceptance criteria met (7/7 tasks)
- [x] Phase 3 (Analysis Cycle 1) tasks completed (6/6)
- [x] Phase 4 (Analysis Cycle 2) tasks completed (4/4)
- [x] Phase 5 (Analysis Cycle 3) tasks completed (2/2)
- [x] All 24 tasks completed
- [x] No scope creep — out-of-scope marker-set path (`internal/restore/session.go:380-384`) and hydrate-helper unset path (`cmd/state_hydrate.go:312`) confirmed unmodified

### Code Quality

No issues found. Implementation follows project conventions throughout:

- Small (1-3 method) interface seams matching the bootstrap orchestrator pattern.
- `*tmux.Client` directly satisfies all three cleanup seams via structural typing — no closure adapter glue.
- `errors.Join` for per-marker unset aggregation; `errors.Is`/`errors.As` in tests.
- `noopLogger{}` nil-substitution pattern matches Orchestrator-wide convention.
- Doc comments cross-reference spec sections (e.g. "Fix Component A → Filtering Levels"); soft-warning posture rationale is documented.
- Step numbering aligned across `cmd/bootstrap/bootstrap.go`, CLAUDE.md, `internal/bootstrapadapter/adapters.go`, and integration tests in the nine-step framing.
- No `t.Parallel()` per project convention.

### Test Quality

Tests adequately verify requirements:

- **Phase 1**: replacement of the buggy codifying test with its inverse; window-level and pane-level filter tests; canonical-ordering preservation; empirical-scenario regression test (`kill_mid_flight_self_heal`) seeds `prev` to discriminate against the false-green that the `prev != nil` gate would produce on buggy code; positive hydrate-in-progress merge test confirms legitimate flow.
- **Phase 2**: stale-unset, live-preservation, format pinning, prefix composition; paneKey normalisation correctness fixture (positive/negative/rightmost-colon); mass-unset hazard guard for enum-error and zero-panes branches; soft-warning posture for per-unset failure and malformed-line skip; orchestrator-level ordering and continues-past-failure tests; end-to-end scrollback-save resumption integration test with negative-control regression coverage.
- Test homes follow architecture: unit tests for `MarkerCleanupCore` co-located in `cmd/bootstrap/`; integration tests gated `//go:build integration`; analysis-cycle 3 consolidated duplicated daemon-tick helpers and orchestrator-builder literals.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. `cmd/bootstrap/stale_marker_cleanup.go:49-51` — `MarkerCleanupCore` doc still says "later tasks layer on normalisation correctness, the mass-unset hazard guard, soft-warning posture, orchestrator wiring, adapter wiring, and end-to-end regression". All those tasks have landed; tighten to describe current state. (Reports 2-1, 3-5)
2. `cmd/bootstrap/orchestrator_builder_test.go` — missing `//go:build integration` build tag explicitly called out in analysis-tasks-c1.md task 2. All 10 call sites are integration-tagged; helpers compile cleanly under non-integration builds. Trivial single-line addition. (Report 3-2)
3. `cmd/bootstrap/reboot_roundtrip_test.go:509-515` preamble references daemon-tick helper behaviour twice; could be trimmed since the helper file's own docstring covers both knobs. (Report 3-1)
4. `internal/state/capture_test.go:570` — new positive subtest uses snake_case `hydrate_in_progress_pane_merges_from_prev_at_matching_coords` while peers use sentence-case prose. Mechanical rename for stylistic consistency. (Report 1-5)

### Ideas

5. `buildLiveStructure` allocates three nested maps per merge call on every tick where `len(skipSet) > 0`. Likely irrelevant at typical N (<100 panes); flag if scale grows. (Report 1-2)
6. `kill_mid_flight_self_heal` subtest tick 2 reuses the same `mock`; a one-line comment confirming "tmux state unchanged across ticks; only prev differs" would short-circuit a future reviewer's read. (Report 1-4)
7. `kill_mid_flight_self_heal` subtest name uses snake_case while peers use sentence-case prose. (Report 1-4)
8. New positive subtest at `capture_test.go:570-641` and the immediately-preceding "preserves prior pane data when its key is in the skip set" subtest share an almost-identical fixture. Consider folding both, or adding cross-reference comment. (Report 1-5)
9. `Logger` field mutated inside `CleanStaleMarkers` (`c.Logger = noopLogger{}` when nil) — same pattern as orchestrator's Run, but a local var would avoid surprising mutation if `MarkerCleanupCore` were ever reused across calls. (Reports 2-1, 3-6)
10. `parseLivePaneSet` requires `logger` non-nil. A breadcrumb-callback parameter (rather than full `Logger` interface) would tighten the contract since only `.Warn` is ever called. (Report 2-1)
11. Sub-test 3 in `TestStaleMarkerCleanup_PaneKeyNormalisation` uses `host:1234` for rightmost-colon-split exercise. Future fixture could pair this with a session name where colon handling could plausibly change in the sanitiser. Optional. (Report 2-2)
12. Sub-test 3 name reads as describing the parser; alternative framing could tighten contract framing. Cosmetic. (Report 2-2)
13. `liveFormat` literal duplicated in test assertion and producer doc. Intentional pinning. No action. (Report 2-3)
14. Deferral Warn message format string at `stale_marker_cleanup.go:142-143` not factored to constant. Tests grep for substrings, coupling test-to-implementation textually. Acceptable for human-facing log output. (Reports 2-3, 3-6)
15. Four `Logger.Warn` call sites in `parseLivePaneSet` duplicate prefix. A small helper could reduce repetition; current spelling is clear and grep-friendly. (Report 2-4)
16. Spec phrase "soft warning when unexpected" implemented as "always Warn on malformed line" — strict superset. Revisit if log volume becomes a concern. (Report 2-4)
17. No explicit `var _ bootstrap.MarkerCleaner = (*MarkerCleanupCore)(nil)` compile-time assertion. Structural assignment at `cmd/bootstrap_production.go:123` is sufficient enforcement, but explicit assertion would lock contract at type's declaration site. (Report 2-6)
18. Three test bodies in `scrollback_resumption_test.go` share substantial setup; a small `seedLeakedMarker(t, ts, sessionName)` helper would reduce duplication. (Report 2-7)
19. `TestScrollbackResumption_LiveHydrateInProgressMarkerPreserved` overlaps marker presence/absence with cheaper unit coverage. Could be slimmed if runtime concern. (Report 2-7)
20. Plan task 3-1 wording says "skipSet and useEmptyScrollback knobs" but implementation correctly fetches skipSet internally and exposes `skipGuard` boolean. Plan wording could be tightened. (Report 3-1)
21. Helper exports `DaemonTickOption` (capitalised) but in `package bootstrap_test`; effectively unused outside package. Could be lowercase. (Report 3-1)
22. Helper file co-locates orchestrator builder (3-2) and stateDir/logger preamble helpers (3-3 scope). Could migrate to dedicated file if it grows. (Report 3-2)
23. Plan acceptance for 3-3 required `//go:build integration` gating but `orchestrator_builder_test.go` has no build tag because `phase5_integration_test.go` (one of 9 call sites) is also untagged. Either update plan to acknowledge helper must be untagged, or add `//go:build integration` to phase5_integration_test.go. (Report 3-3)
24. Plan listed single combined helper `setupIntegrationStateAndLogger`; implementer split into two with documented rationale. Worth confirming planning doc updated. (Report 3-3)
25. Helpers live in `orchestrator_builder_test.go` rather than dedicated `integration_helpers_test.go` as spec named. Functional equivalent. (Report 3-3)
26. Historical planning artifacts under `.workflows/daemon-merge-reintroduces-dead-sessions/planning/` (phase-2-tasks.md:203,207,211,246; review-traceability-tracking-c1.md:79) still contain "ten-step" wording. Out of scope per acceptance criterion. (Report 3-4)
27. Builder docstring at `orchestrator_builder_test.go:19-20` is honest that adding a step requires editing TWO files (this + reattach sibling). Plan acceptance bar could read "exactly one file per package". (Report 3-2)
28. `cmd/bootstrap_production.go` package comment dense; if a fifth exported adapter is added later, bullet list at that point would help. (Report 4-1)
29. No explicit `var _ bootstrap.Logger = (*state.Logger)(nil)` compile-time assertion. Production wiring provides equivalent enforcement; explicit assertion would create reverse import. (Report 4-3)
30. `cmd/bootstrap/stale_marker_cleanup.go:65` and `stale_marker_cleanup_test.go:18` retain `markerListerFunc` references in migration-narrative docstrings. Could collapse to one-line "Markers mirrors FIFOSweeper.Client" comment. (Report 5-1)
