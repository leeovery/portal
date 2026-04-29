---
agent: architecture
cycle: 3
findings_count: 5
---
# Architecture Analysis (Cycle 3)

## Summary

Five low-severity architectural concerns post-cycle-2. The four new packages (bootstrapadapter, tmuxout, tmuxtest, xdg) are well-sized and well-justified. The bootstrap.Logger interface is now genuinely load-bearing post-T7-4 (recordingLogger consumes it as a fake; *state.Logger satisfies it in production) and is no longer "interface theater." The `Orchestrator.fatalf` helper is a clean abstraction. Outstanding concerns are stylistic/locality issues, not foundational design problems.

---

## Findings

### FINDING: NoOpServer and NoOpRestoringMarker have zero callers — orphaned exported API surface
- **Severity**: low
- **Files**: `cmd/bootstrap/noop.go:21-24`, `:33-41`
- **Description**: T8-9 promoted six NoOp* step types into cmd/bootstrap/noop.go to give "tests across packages and production-side fallbacks" one canonical source. Of the six, four (NoOpHooks, NoOpSaver, NoOpRestorer, NoOpStaleCleaner) earn their keep — phase5_integration_test.go references them, and bootstrap_production.go uses NoOpStaleCleaner as a degradation fallback. NoOpServer and NoOpRestoringMarker have zero references outside their own declarations. The phase5 integration tests use the live tmux client for Server and the bootstrapadapter wrapping for RestoringMarker; production has no degradation fallback for these two steps because the spec mandates them as fatal-on-failure (Server failure = fatal step 1; RestoringMarker failure = fatal step 3/6). Keeping them around invites tests/code to reach for "a default" that violates the bootstrap contract.
- **Recommendation**: Delete NoOpServer and NoOpRestoringMarker from cmd/bootstrap/noop.go. Update the file's leading comment to note that NoOp implementations exist only for the four steps the spec permits to degrade-and-continue (Hooks, Saver, Restore, StaleCleaner).

### FINDING: Same method name on adjacent types creates an ambiguous seam at drift-warning site
- **Severity**: low
- **Files**: `internal/restore/restore.go:153-167`, `internal/restore/session.go:407-412`
- **Description**: T8-6 moved the drift WARN from `ApplySkeletonMarkers` (a write primitive) into the orchestrator. The new `Orchestrator.warnOnPaneKeyDrift` helper iterates predicted-vs-live keys and dispatches to `SessionRestorer.warnOnPaneKeyDrift` for the per-pane WARN. Both methods carry the same name on adjacent types in the same package; the call site `sr.warnOnPaneKeyDrift(...)` inside `o.warnOnPaneKeyDrift(...)` reads as a recursive-looking dispatch when it is actually a delegation across types. The split also fragments responsibility: predicted-key computation lives on the orchestrator, but the actual `Logger.Warn` call lives on SessionRestorer purely so it can reach `r.Logger`.
- **Recommendation**: Rename `SessionRestorer.warnOnPaneKeyDrift` → `SessionRestorer.logPaneKeyDrift` (or similar), or — preferred — emit the WARN directly from `Orchestrator.warnOnPaneKeyDrift` using `o.Logger.Warn`, and delete the SessionRestorer-side helper. The drift concern lives entirely on the orchestrator side now; SessionRestorer is a write/arm primitive and should not own a drift-logging method.

### FINDING: flattenSavedPanePositions / savedPanePos vestigial in session.go after the T8-6 pivot
- **Severity**: low
- **Files**: `internal/restore/session.go:372-391`, `internal/restore/restore.go:153-167`
- **Description**: After T8-6 simplified `ApplySkeletonMarkers`' signature, `flattenSavedPanePositions` and `savedPanePos` are consumed by exactly one caller — `Orchestrator.warnOnPaneKeyDrift` in restore.go — yet they remain in session.go (the create/arm/geometry/markers-write file). session.go's package doc describes its role as "create + geometry + skeleton-markers"; saved-position flattening is no longer part of any of those concerns. A reader tracing how drift detection works finds half the logic in restore.go and half buried in session.go's helper tail.
- **Recommendation**: Move `savedPanePos` and `flattenSavedPanePositions` from session.go to restore.go, alongside `Orchestrator.warnOnPaneKeyDrift` — colocating the drift-detection machinery with its sole consumer.

### FINDING: Production adapter naming asymmetric with internal/bootstrapadapter
- **Severity**: low
- **Files**: `internal/bootstrapadapter/adapters.go:31-58`, `cmd/bootstrap_production.go:30-100`
- **Description**: T7-8 created internal/bootstrapadapter for the two trivial adapters (`RestoringMarker`, `HookRegistrar`) that wrap *tmux.Client. The package's leading comment correctly notes that adapters with richer dependencies stay in cmd/bootstrap_production.go. The split solves a real test-import constraint and is defensible. However, the bootstrapadapter types use Pascal-case (`RestoringMarker`, `HookRegistrar`) while the cmd-side types use camelCase (`saverAdapter`, `restoreOrchestratorAdapter`, `cleanStaleAdapter`). A reader looking for "where do bootstrap step adapters live" finds two locations with different naming conventions.
- **Recommendation**: Borderline — the test-import constraint genuinely justifies the split. Either (a) export the cmd-side adapters with Pascal names (`SaverAdapter`, `RestoreAdapter`, `CleanStaleAdapter`) for cross-package consistency, or (b) leave as-is and add to cmd/bootstrap_production.go's leading comment an explicit note that "lowercase naming is the cmd-package signal that these adapters compose dependencies test code cannot reach."

### FINDING: Production Restore adapter conflates restoration with FIFO sweep, hiding a step boundary
- **Severity**: low
- **Files**: `cmd/bootstrap_production.go:63-78`, `cmd/bootstrap/bootstrap.go:1-13`, `cmd/bootstrap/phase5_integration_test.go:71-82`
- **Description**: The bootstrap.Orchestrator's leading doc-comment pins an eight-step sequence; step 5 is "Restore" and step 7 is "CleanStale". The production `restoreOrchestratorAdapter.Restore` runs the inner restore *and* sweeps orphan hydrate-*.fifo files via `state.SweepOrphanFIFOs`. The FIFO sweep is conceptually a separate post-restore cleanup — it has its own failure mode (best-effort, swallows errors), its own data source (`ListSkeletonMarkers`), and runs *after* Restore completes — yet it is bolted into the Restore adapter rather than being its own step. Two effects: (1) the orchestrator's leading comment lists eight steps but production wires nine concerns; (2) the test-side `restoreOrchestratorAdapter` in phase5_integration_test.go is a 1-line pass-through with no FIFO sweep, so the integration tests do not exercise the production sweep at all.
- **Recommendation**: Either promote the FIFO sweep to its own bootstrap step interface (e.g. `FIFOSweeper` with `SweepOrphanFIFOs`), wired alongside `StaleCleaner`, so the eight-step contract grows to nine and the sweep gets its own test surface; or, if eight steps is sacred, explicitly call out in the orchestrator's leading comment that "step 5 (Restore) includes orphan-FIFO sweep on the production wiring." Without one of these, the production wiring carries a behaviour the orchestrator's contract does not mention.
