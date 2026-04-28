---
agent: architecture
cycle: 2
findings_count: 4
---
# Architecture Analysis (Cycle 2)

## Summary

Two structural findings worth resolving — `ApplySkeletonMarkers`' false-error signature and the leakage of base-index prediction parameters into the marker-write contract — both direct consequences of the T7-9 pivot leaving residual plumbing behind. Two lower-severity seam concerns: a missing FIFO→marker convenience helper that would lock down the hydrate-path naming invariant, and a scattered set of bootstrap-step noop implementations that should consolidate into `cmd/bootstrap` as the canonical sources.

---

## Findings

### FINDING: ApplySkeletonMarkers returns error but never produces one
- **Severity**: low
- **Files**: `internal/restore/session.go:356,385`, `internal/restore/restore.go:144`
- **Description**: ApplySkeletonMarkers is declared as `(...) error` but every code path returns nil — per-pane setSkeletonMarker failures are logged-and-swallowed (session.go:433-437), the count-mismatch path warns but doesn't error, and there is no other error source. The single caller in restore.go:144 dutifully checks `if err := sr.ApplySkeletonMarkers(...); err != nil` and logs — a permanently-dead branch. The signature now lies about its contract and forces a dead-code error check at the only call site.
- **Recommendation**: Change ApplySkeletonMarkers' signature to drop the `error` return. Update the lone caller in restoreOne to drop the if/err branch.

### FINDING: ApplySkeletonMarkers couples marker writing to base-index prediction plumbing
- **Severity**: medium
- **Files**: `internal/restore/session.go:356-374,419-428`, `internal/restore/restore.go:137-145`
- **Description**: After T7-9 pivoted to `respawn-pane -k` with live coords threaded through, `predictedBase` and `predictedPaneBase` no longer drive any tmux call target. Yet ApplySkeletonMarkers still accepts both ints as parameters purely so it can compute `predictedKey` and emit a drift WARN on mismatch (session.go:370-371). This couples a write primitive to a diagnostic concern. The orchestrator already calls PredictLiveIndices; that's the right place to compute and emit the drift warning by comparing predicted vs live keys directly.
- **Recommendation**: Drop predictedBase / predictedPaneBase from ApplySkeletonMarkers' signature. Move the drift comparison + warnOnPaneKeyDrift loop into restoreOne (or a helper called from restoreOne).

### FINDING: Hydrate file-missing path silently rebuilds livePaneKey while marker-key invariant lives elsewhere
- **Severity**: low
- **Files**: `cmd/state_hydrate.go:99,178,302-303`, `internal/state/markers.go:104-117`, `internal/state/paths.go:97-112`
- **Description**: runHydrate computes `livePaneKey := state.PaneKeyFromFIFOPath(cfg.FIFO)` at line 99 and uses it on the success path's UnsetSkeletonMarker (line 178). handleHydrateFileMissing recomputes the same value at line 302. There's a load-bearing convention (FIFOPath embeds paneKey, PaneKeyFromFIFOPath inverts it, the helper uses that inverse to find the right marker to unset) that's never expressed as a single derived relationship. If a future refactor of FIFOPath's filename shape breaks the inverse, the failure is silent.
- **Recommendation**: Add a `state.UnsetSkeletonMarkerForFIFO(w ServerOptionWriter, fifoPath string) error` helper that composes PaneKeyFromFIFOPath + UnsetSkeletonMarker. Replace both hydrate-side call sites.

### FINDING: noopStaleCleaner sits in cmd/bootstrap_production.go without a structural home
- **Severity**: low
- **Files**: `cmd/bootstrap_production.go:102-109`, `cmd/bootstrap/phase5_integration_test.go:81-109`
- **Description**: cmd/bootstrap_production.go declares a private `noopStaleCleaner` used as a fallback when loadHookStore fails. The bootstrap package's own integration tests independently declare four sibling types (noopSaver, noopRestorer, noopCleaner, noopHooks). The test-side noopCleaner is a verbatim duplicate. With each new bootstrap step interface, this pattern multiplies; there is no canonical "no-op fulfilment of bootstrap.{Step}" anywhere.
- **Recommendation**: Move the noop step types into a file in the cmd/bootstrap package itself (e.g. `cmd/bootstrap/noop.go`) as exported types: NoOpServer, NoOpHooks, NoOpRestoringMarker, NoOpSaver, NoOpRestorer, NoOpStaleCleaner. Tests across all packages get one canonical source.
