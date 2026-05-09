# Analysis Tasks: daemon-merge-reintroduces-dead-sessions (Cycle 2)

```
topic: daemon-merge-reintroduces-dead-sessions
cycle: 2
total_proposed: 4
```

## Discarded Findings
- **`staleClientStub` cross-file duplication** (duplication, low) — explicitly acknowledged in source comments with a correct explanation of the Go visibility constraint between white-box `bootstrapadapter` and black-box `bootstrapadapter_test` packages. Trade-off documented; ~25 lines sits at the low end of extraction thresholds.

---

## Task 1: Fix step-number docstring drift in cmd/bootstrap_production.go and add missing adapter to inventory
status: approved
severity: low
sources: standards, architecture

**Problem**: Two docstring drifts in `cmd/bootstrap_production.go`: (a) line 51 docstring on `cleanStaleAdapter` says "Step 8 of the bootstrap sequence" but per the post-cycle-1 nine-step sequence (CleanStaleMarkers=7, SweepOrphanFIFOs=8, CleanStale=9) it is step 9; (b) the file-leading adapter inventory comment at lines 16-17 enumerates adapters as "HookRegistrar, RestoringMarker, RestoreAdapter, FIFOSweeper" and omits `StaleMarkerCleaner` (added in this work unit). Wiring is correct — only docs drift.

**Solution**: Edit `cmd/bootstrap_production.go` to correct both comments in a single pass.

**Outcome**: Docstring matches the canonical nine-step sequence; inventory comment lists every adapter currently exported by `internal/bootstrapadapter`.

**Do**:
1. At line 51, change "Step 8 of the bootstrap sequence" to "Step 9 of the bootstrap sequence".
2. At line 17, add `StaleMarkerCleaner` to the adapter inventory list.
3. Run `go build -o portal .` and `go test ./cmd/...` to confirm no regression.

**Acceptance Criteria**:
- `cmd/bootstrap_production.go:51` reads "Step 9 of the bootstrap sequence".
- `cmd/bootstrap_production.go:16-17` inventory comment includes `StaleMarkerCleaner`.
- Build and tests pass unchanged.

**Tests**:
- No new tests required. Existing `cmd/bootstrap/...` and `internal/bootstrapadapter/...` tests must pass.

---

## Task 2: Fix step-number drift in adapters_test.go FIFOSweeper docstring
status: approved
severity: low
sources: standards

**Problem**: `internal/bootstrapadapter/adapters_test.go:41` docstring on `TestFIFOSweeper_PropagatesListSkeletonMarkersError` says "the orchestrator's step-7 Warn-and-swallow path can log it uniformly" — but `FIFOSweeper` is now step 8. The matching production-side docstrings in `adapters.go:107` and `:131` were correctly updated; only the test comment escaped the rename. Line 139 of the same file correctly references "step-7" for the `StaleMarkerCleaner` test and must not change.

**Solution**: Update the single test docstring at line 41.

**Outcome**: Test docstrings match production docstrings and the canonical nine-step sequence.

**Do**:
1. At line 41, change "step-7 Warn-and-swallow path" to "step-8 Warn-and-swallow path".
2. Verify line 139 still reads "step-7" — do not change.
3. Run `go test ./internal/bootstrapadapter/...`.

**Acceptance Criteria**:
- `adapters_test.go:41` references "step-8 Warn-and-swallow path".
- `adapters_test.go:139` still references "step-7" (StaleMarkerCleaner).
- `go test ./internal/bootstrapadapter/...` passes.

**Tests**:
- Existing tests must pass; no new tests required.

---

## Task 3: Re-type MarkerCleanupCore.Logger to the bootstrap.Logger interface
status: approved
severity: low
sources: architecture

**Problem**: `MarkerCleanupCore.Logger` (`cmd/bootstrap/stale_marker_cleanup.go:62`) is typed as concrete `*state.Logger`, breaking the orchestrator's convention where every other step seam depends on the abstract `Logger` interface (Debug/Warn/Error) defined in `cmd/bootstrap/bootstrap.go:139-159`. The asymmetry forces `stale_marker_cleanup_test.go` to spin up a real `state.OpenLogger` to a tempfile and re-read it to assert Warn fired, when other orchestrator-level tests inject `recordingLogger{}` and assert in-memory.

**Solution**: Re-type `MarkerCleanupCore.Logger` as `bootstrap.Logger` and replace the tempfile-based logger plumbing in tests with the existing `recordingLogger{}` pattern.

**Outcome**: Logger field type matches the rest of cmd/bootstrap; tests assert log output in-memory; the `openZeroPanesGuardLogger`/`readLogFile` test helpers can be removed.

**Do**:
1. Change the type of the `Logger` field on `MarkerCleanupCore` (line 62) from `*state.Logger` to `Logger` (the interface defined in bootstrap.go).
2. Replace the `openZeroPanesGuardLogger` helper and any `readLogFile` plumbing in `stale_marker_cleanup_test.go` with the existing `recordingLogger{}`. Assert Warn message contents against captured entries.
3. Update wiring sites (production `*state.Logger` already satisfies `bootstrap.Logger` — verify no compile break).
4. Delete `openZeroPanesGuardLogger` / `readLogFile` test helpers if no longer referenced.
5. Run `go test ./cmd/bootstrap/...` and `go test ./...`.

**Acceptance Criteria**:
- `MarkerCleanupCore.Logger` is typed as `bootstrap.Logger`.
- `stale_marker_cleanup_test.go` uses `recordingLogger{}` and asserts Warn message contents in-memory.
- `openZeroPanesGuardLogger` and any associated tempfile helpers are removed if unused.
- All cmd/bootstrap tests pass.

**Tests**:
- Existing zero-panes-guard Warn assertion and normal cleanup paths ported to use `recordingLogger`; assertions on log line contents (component, marker count) preserved.

---

## Task 4: Eliminate redundant StaleMarkerCleaner adapter pass-through
status: approved
severity: low
sources: architecture

**Problem**: `bootstrapadapter.StaleMarkerCleaner` (`internal/bootstrapadapter/adapters.go:201-245`) is a one-line pass-through over `bootstrap.MarkerCleanupCore`, which already exposes the public `CleanStaleMarkers()` method satisfying `bootstrap.MarkerCleaner` with all four wiring fields exported. The adapter adds (a) an unexported `staleMarkerClient` interface, (b) a `markerListerFunc` closure type, (c) a `NewStaleMarkerCleaner` constructor, and (d) a wrapper struct with one field — solely to forward to a struct that could be constructed directly at the cmd/bootstrap_production.go wiring site.

**Solution**: Apply option (a): delete `bootstrapadapter.StaleMarkerCleaner` and construct `&bootstrap.MarkerCleanupCore{...}` inline at the cmd/bootstrap_production.go wiring site.

**Outcome**: One layer of indirection removed. `bootstrapadapter` package shrinks by ~45 lines.

**Do**:
1. At cmd/bootstrap_production.go's `StaleMarkerCleaner` wiring site (~lines 113-128), replace the `bootstrapadapter.NewStaleMarkerCleaner(...)` call with a direct `&bootstrap.MarkerCleanupCore{...}` struct literal, populating the four exported fields. Match the pattern used by `cleanStaleAdapter` / `saverAdapter`.
2. Delete `StaleMarkerCleaner` struct, `CleanStaleMarkers()` method, `NewStaleMarkerCleaner` constructor, unexported `staleMarkerClient` interface, and `markerListerFunc` type from `internal/bootstrapadapter/adapters.go`.
3. Delete tests exercising `StaleMarkerCleaner` directly (`staleClientStub`-driven tests in `adapters_test.go` and `adapters_internal_test.go`). Remove `staleClientStub` declarations.
4. If task 1's inventory-comment update has landed, remove `StaleMarkerCleaner` from the `cmd/bootstrap_production.go` adapter inventory comment.
5. Confirm `*MarkerCleanupCore`-level tests in `cmd/bootstrap/stale_marker_cleanup_test.go` cover the deleted adapter-level assertions. Add equivalent coverage at the core level if any case is uniquely covered at the adapter level.
6. Run `go build -o portal .` and `go test ./...`.

**Acceptance Criteria**:
- `bootstrapadapter.StaleMarkerCleaner`, `NewStaleMarkerCleaner`, `staleMarkerClient`, and `markerListerFunc` are deleted from `internal/bootstrapadapter/adapters.go`.
- `cmd/bootstrap_production.go` constructs `&bootstrap.MarkerCleanupCore{...}` directly.
- `staleClientStub` declarations removed from both test files.
- All behaviour formerly asserted by `StaleMarkerCleaner` tests is asserted at `*MarkerCleanupCore` level.
- `go build` and `go test ./...` pass.

**Tests**:
- Verify `cmd/bootstrap/stale_marker_cleanup_test.go` covers: zero-panes-guard Warn path, list-skeleton-markers error propagation, normal stale-marker cleanup. Add cases as needed.
