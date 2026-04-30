# Analysis Tasks: built-in-session-resurrection (Cycle 7)

```yaml
topic: built-in-session-resurrection
cycle: 7
total_proposed: 3
```

---

## Task 1: Add deferred logger Close in state_cleanup RunE

- **status:** approved
- **severity:** low
- **sources:** duplication (D1)

**Problem**: `cmd/state_cleanup.go`'s RunE acquires a `*state.Logger` via `buildStateCleanupDeps()` but never calls `logger.Close()`. Every other cmd RunE that uses `openNoRotateLogger` (`state_signal_hydrate.go:150-151`, `state_hydrate.go:366-367`, `state_notify.go:47-48`, `state_migrate_rename.go:33-34`) pairs acquisition with `defer func() { _ = logger.Close() }()` to flush the buffered writer. INFO/WARN lines emitted by `killSaver` and `purgeStateDir` may not flush before process exit. Phase 13 task 13-5 doc-touched this file but did not normalize the lifecycle.

**Solution**: Add `defer func() { _ = logger.Close() }()` immediately after the `buildStateCleanupDeps()` call in `cmd/state_cleanup.go`'s RunE so the logger is closed on every exit path.

**Outcome**: `state_cleanup` matches the lifecycle shape of its four sibling RunE bodies; buffered log lines from `killSaver` and `purgeStateDir` are guaranteed to flush before process exit.

**Do**:
1. Open `cmd/state_cleanup.go`.
2. Locate the RunE body that calls `buildStateCleanupDeps()` (around line 49) and binds `client, unregister, logger`.
3. Immediately after that line (and after the existing `unregister`-related defer if present, matching the ordering used in the sibling RunE bodies), insert `defer func() { _ = logger.Close() }()`.
4. Confirm `*state.Logger.Close()` is nil-receiver-safe (it is) so test injection that passes a nil logger continues to work.
5. Run `go build ./...` and `go test ./cmd/...` to confirm no regressions.

**Acceptance Criteria**:
- `cmd/state_cleanup.go`'s RunE has a deferred `logger.Close()` call placed in the same relative position as in `state_signal_hydrate.go`, `state_hydrate.go`, `state_notify.go`, and `state_migrate_rename.go`.
- `go build ./...` succeeds.
- Existing `cmd` package tests pass with no modification.

**Tests**:
- No new test required — single-line consistency fix verified by build + existing test suite.

---

## Task 2: Fix or remove misleading init() rationale on openTUIFunc / openPathFunc seams

- **status:** approved
- **severity:** low
- **sources:** architecture (A1)

**Problem**: `cmd/open.go:21` and `cmd/open.go:28` declare `openTUIFunc` and `openPathFunc` and the godoc on these (and the init() block at `cmd/open.go:428-432`) claims they are initialized in init() to "break the openTUIFunc → openTUI → openCmd → openTUIFunc cycle." No such cycle exists — `openTUI` and `openPath` do not reference `openCmd`. The sibling pattern at `cmd/state_signal_hydrate.go:127` (`signalHydrateRunFunc = runSignalHydrate`) and the seam in `state_hydrate.go` use direct package-level `var x = fn` initialization in structurally identical circumstances and compile fine. The init()-based assignment here is defensive, not load-bearing, and the comment will mislead future maintainers extending this pattern to a third seam.

**Solution**: Convert both seams to direct package-level initialization (`var openTUIFunc = openTUI` and `var openPathFunc = openPath`), drop the corresponding init() block, and align the godoc with the new shape. If direct init does cause a real compile cycle for any reason, retain init() but rewrite the godoc to describe the actual reason rather than the fictitious cycle.

**Outcome**: `cmd/open.go`'s test seams use the same direct `var x = fn` pattern as `state_signal_hydrate.go` and `state_hydrate.go`; godoc accurately reflects what the code does; future seam additions in this package can copy any of the three precedents without inheriting a misleading rationale.

**Do**:
1. Read `cmd/open.go` lines 21, 28, and 428-432 to capture the current declarations, godoc, and init() block.
2. Confirm by inspection that `openTUI` and `openPath` do not transitively reference `openCmd`.
3. Change the declarations to `var openTUIFunc = openTUI` and `var openPathFunc = openPath`.
4. Remove the init() block that assigns these (lines 428-432) if it contains only those two assignments; otherwise remove only those two lines.
5. Update the godoc on each seam to drop the "break the cycle" claim. Replace with a brief, accurate one-line note such as "Test seam: overridden via t.Cleanup-restored assignment in tests to capture argv shape without invoking the real handler."
6. Run `go build ./...` and `go test ./cmd/...`. If a compile cycle actually surfaces, revert the init() removal but rewrite the godoc to describe the real reason — do not retain the false rationale.
7. Verify the existing compile-time signature assertions for both seams still hold.

**Acceptance Criteria**:
- `openTUIFunc` and `openPathFunc` are either (a) initialized via direct `var x = fn` with no init() block needed, with corrected godoc, or (b) retained as init()-assigned with godoc rewritten to describe the actual reason (no fictional cycle).
- The "openTUIFunc → openTUI → openCmd → openTUIFunc cycle" wording is removed from the codebase.
- `go build ./...` succeeds.
- All existing `cmd` package tests, including any that override `openTUIFunc` or `openPathFunc` via `t.Cleanup`, pass unchanged.

**Tests**:
- No new test required — change is documentation/structural and covered by existing seam-override tests.

---

## Task 3: Unexport OpenAndSignalFIFO in internal/restoretest

- **status:** approved
- **severity:** low
- **sources:** architecture (A2)

**Problem**: `internal/restoretest/restoretest.go:262` exports `OpenAndSignalFIFO` with godoc claiming it is needed by "integration round-trip tests across multiple layers", but the only caller is the sibling `DriveSignalHydrate` inside the same file. Both `internal/restore/integration_full_test.go` and the cmd-package integration tests reach FIFO writes via the higher-level `DriveSignalHydrate` / `DriveSignalHydrateBinary` helpers, not the lower primitive. Exporting preemptively widens restoretest's API surface and could let future callers couple to the `(delay, budget)` tuple shape instead of the curated Drive helpers.

**Solution**: Rename `OpenAndSignalFIFO` to `openAndSignalFIFO` (unexport) and update its single in-package caller `DriveSignalHydrate`. Re-export only when a second caller outside this file actually materializes.

**Outcome**: `internal/restoretest`'s public API surface is narrowed to the helpers that have real external consumers; the lower-level FIFO primitive remains available within the package without inviting external coupling to its tuple shape.

**Do**:
1. Open `internal/restoretest/restoretest.go`.
2. Rename the function declaration `OpenAndSignalFIFO` (line 262) to `openAndSignalFIFO`. Preserve the body and signature unchanged.
3. Update the call site inside `DriveSignalHydrate` to use the new lowercase name.
4. Update the function's godoc to drop the "needed by integration round-trip tests across multiple layers" framing — replace with a brief internal-helper docline.
5. Search the rest of the repo for `OpenAndSignalFIFO` to confirm zero external references. If any are found outside `internal/restoretest/restoretest.go`, surface them.
6. Run `go build -tags integration ./...` and `go test -tags integration ./...` to confirm no regressions.

**Acceptance Criteria**:
- `internal/restoretest/restoretest.go` no longer exports `OpenAndSignalFIFO`; the symbol is `openAndSignalFIFO`.
- `DriveSignalHydrate` continues to compile and behave identically.
- A repo-wide search for `OpenAndSignalFIFO` returns no matches.
- Integration build and tests (`go build -tags integration ./...`, `go test -tags integration ./...`) pass.

**Tests**:
- No new test required — covered by existing integration tests that exercise `DriveSignalHydrate` / `DriveSignalHydrateBinary`.

---

## Discarded findings (for the orchestrator's record)

- **D2** (`waitForSessionMarkerCleared` near-duplicate of `WaitForSkeletonMarkersCleared`) — single call site, below Rule-of-Three; agent recommended no action.
- **D3** (`verifySwitchClientLiveStructure` subset of `verifyLiveStructure`) — single call site, below Rule-of-Three; agent recommended no action.
- **S1** (`TestReattachIntegration_OpenPathResolvesSavedOnlySession` planning-bullet wording mismatch) — agent recommended no code change; godoc already documents the trade-off honestly.
- **S2** (`DriveSignalHydrateBinary` exec's inner subcommand only, not full hook-content wrapper) — agent recommended deferral; godoc is honest about the divergence.
