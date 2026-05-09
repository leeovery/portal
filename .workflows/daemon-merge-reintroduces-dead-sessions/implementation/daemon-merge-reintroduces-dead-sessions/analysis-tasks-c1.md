# Analysis Tasks: daemon-merge-reintroduces-dead-sessions (Cycle 1)

```
topic: daemon-merge-reintroduces-dead-sessions
cycle: 1
total_proposed: 6
```

## Discarded Findings
- **buildLiveStructure flat-vs-nested** (architecture, low) — agent flagged as deferrable ("Not a blocker") and noted future filter rules may want session/window granularity. No clustering.
- **MarkerLister func-shim** (architecture, low) — agent labels recommendation "Cosmetic." No drift risk.

---

## Task 1: Consolidate duplicated daemon-tick test helpers in cmd/bootstrap
status: approved
severity: medium
sources: duplication

**Problem**: `cmd/bootstrap/reboot_roundtrip_test.go:533-569` (`captureAndCommit`) and `cmd/bootstrap/scrollback_resumption_test.go:421-465` (`runDaemonTick`) contain near-identical ~25-line `t.Helper`s driving a daemon-tick-equivalent capture-and-commit (ListSkeletonMarkers → CaptureStructure → walk sessions/windows/panes → state.Commit). Both live in `package bootstrap_test` behind `//go:build integration`. Material differences: `runDaemonTick` honours a per-pane skipSet guard and calls `CaptureAndHashPane`; `captureAndCommit` always writes empty bytes. Drift risk if `state.Commit` grows new args.

**Solution**: Promote one shared helper in a new `cmd/bootstrap/daemon_tick_test_helpers_test.go` (build-tagged `//go:build integration`) accepting `skipSet` and a `useEmptyScrollback bool` knob.

**Outcome**: One source of truth for daemon-tick simulation; future `state.Commit` signature changes touch one helper.

**Do**:
1. Read both helpers to inventory parameters and assertion paths.
2. Create `cmd/bootstrap/daemon_tick_test_helpers_test.go` with `//go:build integration` and `package bootstrap_test`.
3. Define `runDaemonTick(t, client, stateDir, opts)` with optional `skipSet map[string]struct{}` (nil = no guard) and `useEmptyScrollback bool` (false = production-shape `CaptureAndHashPane`, true = empty bytes).
4. Replace both call sites in `reboot_roundtrip_test.go` and `scrollback_resumption_test.go`.
5. Delete the two inline definitions.
6. Run `go test -tags=integration ./cmd/bootstrap/...`.

**Acceptance Criteria**:
- Only one daemon-tick helper exists in `cmd/bootstrap/` test files.
- Both former call sites pass with original semantics.
- Helper gated `//go:build integration`.

**Tests**:
- All existing integration tests in `reboot_roundtrip_test.go` and `scrollback_resumption_test.go` pass under `-tags=integration`.

---

## Task 2: Extract shared bootstrap.Orchestrator builder for integration tests
status: approved
severity: medium
sources: duplication

**Problem**: Eleven sites across `cmd/bootstrap/scrollback_resumption_test.go` (114-127, 228-238, 333-346), `cmd/bootstrap/reboot_roundtrip_test.go` (341-354, 997-1010, 1258-1271), `cmd/bootstrap/phase5_integration_test.go` (69-78, 211-220, 329-342), `cmd/bootstrap/phase5_marker_suppression_integration_test.go` (154-163), and `cmd/reattach_integration_test.go` (167-176) rebuild a `bootstrap.Orchestrator{...}` literal wiring the same eight-step shape. `cmd/reattach_integration_test.go` already has `buildReattachOrchestrator`. Adding a new step interface (as `StaleMarkers` was just added) requires touching all eleven literals.

**Solution**: Promote a single test-only orchestrator builder in `cmd/bootstrap/orchestrator_builder_test.go` (`//go:build integration`) modeled on `buildReattachOrchestrator`, with an `orchestratorOpts` struct defaulting unset fields to NoOp.

**Outcome**: One place to update when a new step interface is added.

**Do**:
1. Inventory every Orchestrator literal across the eleven sites; record NoOp vs production per field.
2. Create `cmd/bootstrap/orchestrator_builder_test.go` with `//go:build integration` and `package bootstrap_test`.
3. Define `orchestratorOpts` with optional fields (`Saver`, `Restore`, `StaleMarkers`, `Sweeper`, `Hooks`, `CleanStale`); unset → NoOp form.
4. Define `buildIntegrationOrchestrator(t, client, opts orchestratorOpts) *bootstrap.Orchestrator`. RestoringMarker is always real.
5. Replace all eleven inline literals.
6. If `buildReattachOrchestrator` is fully subsumed, replace its call sites; otherwise leave as a thin wrapper.
7. Run `go test -tags=integration ./cmd/...`.

**Acceptance Criteria**:
- Zero inline `bootstrap.Orchestrator{...}` literals in any `*_integration_test.go` or `_test.go` under `cmd/bootstrap/`.
- All eleven sites compile and pass with previous semantics.
- Adding a hypothetical new step interface requires editing exactly one file.

**Tests**:
- All existing integration tests in `cmd/bootstrap/` and `cmd/reattach_integration_test.go` pass under `-tags=integration`.

---

## Task 3: Extract shared stateDir + logger preamble for cmd/bootstrap integration tests
status: approved
severity: low
sources: duplication

**Problem**: The boilerplate `stateDir := …; t.Setenv("PORTAL_STATE_DIR", stateDir); state.EnsureDir(); … state.OpenLogger(stateDir, …)` is repeated across nine sites (~9 lines per copy). `cmd/reattach_integration_test.go` already has `setupReattachEnv`.

**Solution**: Add a combined helper in `cmd/bootstrap/integration_helpers_test.go` (`//go:build integration`).

**Outcome**: One place to update if state-dir bootstrap or logger conventions change.

**Do**:
1. Confirm the preamble is byte-identical across all nine sites.
2. Create `cmd/bootstrap/integration_helpers_test.go` with `//go:build integration` and `package bootstrap_test`.
3. Define `setupIntegrationStateAndLogger(t) (stateDir string, logger *state.Logger)` creating a `t.TempDir()`-rooted state dir, calling `t.Setenv("PORTAL_STATE_DIR", …)`, `state.EnsureDir()`, opening a non-rotating `portal.log` logger registered with `t.Cleanup` for close.
4. Replace all nine call-site preambles.
5. Run `go test -tags=integration ./cmd/bootstrap/...`.

**Acceptance Criteria**:
- All nine sites use the new helper; no remaining copies.
- Helper gated `//go:build integration`.
- All integration tests pass.

**Tests**:
- All existing integration tests in `cmd/bootstrap/` pass under `-tags=integration`.

---

## Task 4: Align bootstrap step-count nomenclature (nine-step vs ten-step)
status: approved
severity: low
sources: standards, architecture

**Problem**: `cmd/bootstrap/bootstrap.go`'s package docstring (lines 1-21) and `Run` docstring/log labels (lines 159, 175) call the orchestrator a "ten-step" sequence with "Return" as step 10. But (a) `CLAUDE.md` describes nine-step with "Return" as step 9 (post-step boundary, not numbered); (b) `cmd/bootstrap/bootstrap_test.go:141-151` lists 9 step names; (c) "Return" has no dependency, no seam, not testable in isolation.

**Solution**: Realign on nine-step semantics everywhere: CleanStaleMarkers as step 7, SweepOrphanFIFOs as step 8, CleanStale as step 9, "Return" as the post-step boundary.

**Outcome**: One vocabulary across `CLAUDE.md`, `bootstrap.go` doc, `Run` comments, and tests.

**Do**:
1. Locate every "ten-step" / "ten bootstrap steps" / step-10 reference in `cmd/bootstrap/bootstrap.go`.
2. Rewrite to nine-step framing: 1=EnsureServer, 2=RegisterPortalHooks, 3=Set @portal-restoring, 4=EnsureSaver, 5=Restore, 6=Clear @portal-restoring, 7=CleanStaleMarkers, 8=SweepOrphanFIFOs, 9=CleanStale. "Return" = post-step boundary.
3. Verify `cmd/bootstrap/bootstrap_test.go:141-151`'s step-name list aligns.
4. Update `CLAUDE.md` "Server bootstrap" section: insert CleanStaleMarkers between Clear and Sweep.
5. Run `go test ./cmd/bootstrap/...`.
6. `go build -o portal .`.

**Acceptance Criteria**:
- `cmd/bootstrap/bootstrap.go` package doc, `Run` docstring, and internal comments use nine-step framing with CleanStaleMarkers as step 7.
- `CLAUDE.md` "Server bootstrap" lists CleanStaleMarkers at correct position.
- No remaining "ten-step" / "ten bootstrap steps" in `cmd/bootstrap/`.

**Tests**:
- Existing tests in `cmd/bootstrap/bootstrap_test.go` pass without modification.
- `go build -o portal .` succeeds.

---

## Task 5: Resolve StaleMarkerCleaner dual-type collision and per-call inner construction
status: approved
severity: low
sources: architecture

**Problem**: Two clustered findings: (a) `cmd/bootstrap.StaleMarkerCleaner` (`stale_marker_cleanup.go:68`) and `bootstrapadapter.StaleMarkerCleaner` (`adapters.go:193`) are concrete types with identical names; the doc at `bootstrap.go:92-94` explicitly acknowledges the awkwardness. (b) `bootstrapadapter.(*StaleMarkerCleaner).CleanStaleMarkers` constructs a fresh `&bootstrap.StaleMarkerCleaner{...}` plus `markerListerFunc` closure on every invocation (`adapters.go:206-226`), even though the inner cleaner has no per-tick mutable state.

**Solution**: Rename the cmd/bootstrap concrete type to `MarkerCleanupCore` (or unexported `staleMarkerCleanupRunner`); construct the inner cleaner once at adapter construction.

**Outcome**: No identifier collision, adapter shape consistent with peers (`RestoreAdapter`, `HookRegistrar`), one allocation at startup.

**Do**:
1. In `cmd/bootstrap/stale_marker_cleanup.go`, rename the `StaleMarkerCleaner` struct to `MarkerCleanupCore`. Update doc at lines 92-94 to drop the collision-avoidance caveat.
2. Update all in-package references in `cmd/bootstrap/`.
3. In `internal/bootstrapadapter/adapters.go`:
   - Add private field on `bootstrapadapter.StaleMarkerCleaner` (e.g. `inner *bootstrap.MarkerCleanupCore`) initialised once.
   - Move `&bootstrap.MarkerCleanupCore{...}` literal and `markerListerFunc` closure to construction.
   - Reduce `CleanStaleMarkers()` to a single delegating call.
4. If no `New…` factory exists, add one or initialise the field at the wiring site (`cmd/root.go`).
5. Confirm `MarkerCleaner` interface name remains unchanged.
6. Run `go test ./...`, `go test -tags=integration ./cmd/bootstrap/...`, `go build -o portal .`.

**Acceptance Criteria**:
- No two types named `StaleMarkerCleaner` in the codebase.
- `bootstrapadapter.StaleMarkerCleaner.CleanStaleMarkers` does not construct a fresh inner cleaner per call.
- `MarkerCleaner` interface still satisfied by the bootstrapadapter type.
- `cmd/bootstrap/stale_marker_cleanup.go` doc no longer claims collision-avoidance wording.
- All tests pass; `portal` builds.

**Tests**:
- Existing unit tests for `cmd/bootstrap/stale_marker_cleanup.go` updated to use the new type name and pass.
- Existing wiring tests for `bootstrapadapter.StaleMarkerCleaner` updated and pass.
- Add one targeted test confirming two consecutive `CleanStaleMarkers` calls operate against the same inner-cleaner instance.
- `go test -tags=integration ./cmd/bootstrap/...` passes.

---

## Task 6: Reclassify mass-unset hazard guard from error to warn-and-return-nil
status: approved
severity: medium
sources: architecture

**Problem**: In `cmd/bootstrap/stale_marker_cleanup.go:12-20, 122-128`, when `parseLivePaneSet` returns empty AND markers exist, `CleanStaleMarkers` returns the sentinel `ErrZeroLivePanesWithMarkers` to mean "I refused to act, treat as soft warning." The orchestrator at `bootstrap.go:267-270` Warn-and-swallows indiscriminately, so behaviorally nothing leaks — but the guard's behavior is "skip this run; next bootstrap retries," a successful soft outcome, not a failure. Mixing defensive deferral with genuine `ListAllPanesWithFormat` failures on the same return channel makes error semantics fuzzy.

**Solution**: Return `nil` for the zero-panes-with-markers case; surface the deferral via `Logger.Warn` (function already has the logger). Reserve the error return for genuine failures.

**Outcome**: Error channel now means "genuine failure." Deferral signal moves to portal.log under ComponentBootstrap.

**Do**:
1. In `cmd/bootstrap/stale_marker_cleanup.go`, identify the guard branch returning `ErrZeroLivePanesWithMarkers` (~lines 122-128).
2. Replace return with: `Logger.Warn(...)` (component=ComponentBootstrap, message="skipping stale-marker cleanup: zero live panes but markers present; deferring to next bootstrap", with structured fields for marker count) and `return nil`.
3. `ErrZeroLivePanesWithMarkers` sentinel:
   - If no other caller/test references it, delete.
   - If tests reference it (likely `stale_marker_cleanup_test.go`), update to assert `err == nil` plus a captured Warn log entry. Then remove the sentinel.
4. Update orchestrator at `bootstrap.go:267-270` if it special-cases the sentinel; collapse to plain error-or-nil semantics.
5. Run `go test ./cmd/bootstrap/...` and `go test -tags=integration ./cmd/bootstrap/...`.
6. `go build -o portal .`.

**Acceptance Criteria**:
- Zero-live-panes-with-markers branch returns `nil` and emits `Logger.Warn` with ComponentBootstrap.
- `ErrZeroLivePanesWithMarkers` either deleted, or no longer returned for the deferral case.
- Orchestrator no longer special-cases the sentinel; treats any non-nil error as genuine failure (still soft-warned per existing posture).
- Existing kill-mid-flight / mass-unset hazard tests continue to assert no markers are unset in the zero-panes case.
- `portal` builds and all tests pass.

**Tests**:
- Update existing tests asserting `errors.Is(err, ErrZeroLivePanesWithMarkers)` to instead assert `err == nil` plus a captured Warn log entry (component, message).
- Retain (do not weaken) the assertion that no markers are unset in the zero-live-panes case.
- Add one new table-driven case verifying a genuine `ListAllPanesWithFormat` error still propagates.
