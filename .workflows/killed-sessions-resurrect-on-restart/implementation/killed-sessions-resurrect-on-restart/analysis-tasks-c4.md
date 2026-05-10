# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 4)

- topic: killed-sessions-resurrect-on-restart
- cycle: 4
- total_proposed: 2

---

## Task 1: Collapse pollUntilMarkersCleared into restoretest.WaitForSkeletonMarkersCleared via parameterised tick
status: approved
severity: low
sources: duplication

**Problem**: `pollUntilMarkersCleared` (cmd/bootstrap/eager_signal_hydrate_integration_test.go:434-451) is a near-byte-equivalent re-declaration of `restoretest.WaitForSkeletonMarkersCleared` (internal/restoretest/restoretest.go:291-306). Both run the same loop — `state.ListSkeletonMarkers(client)` → return-on-empty → `time.Sleep(tick)` → on-expiry `t.Fatalf` with `restoretest.SortedKeySet(markers)`. Material differences are diagnostic only: (a) failure-message prefix ("AC1 violation: …" vs plain), (b) `pollUntilMarkersCleared` parameterises `tick` while `WaitForSkeletonMarkersCleared` hardcodes `50*time.Millisecond`, (c) call sites pass different budgets. The local helper already imports `restoretest.SortedKeySet`, so it is not avoiding the dependency — only re-implementing the loop. Both files carry `//go:build integration`. Same cross-package poll-helper-after-extraction pattern cycle 1's `WaitForFileExists` extraction addressed.

**Solution**: Promote `restoretest.WaitForSkeletonMarkersCleared` to the single canonical helper by parameterising the tick (matching cycle 1's `WaitForFileExists` shape `(t, client, budget, tick)`). Drop `pollUntilMarkersCleared` and update all call sites.

**Outcome**: One canonical skeleton-marker poll helper in `restoretest`. `pollUntilMarkersCleared` and its docstring deleted. Two AC1 call sites and five existing `restoretest.WaitForSkeletonMarkersCleared` call sites migrated. Integration tests still build and pass.

**Do**:
1. Edit `internal/restoretest/restoretest.go` `WaitForSkeletonMarkersCleared` (around lines 291-306): add a `tick time.Duration` parameter. New signature: `func WaitForSkeletonMarkersCleared(t *testing.T, client *tmux.Client, timeout, tick time.Duration)`. Replace the hardcoded `50*time.Millisecond` sleep with `tick`. Keep the existing failure message format and `SortedKeySet` diagnostic.
2. Update the five existing call sites to append `, 50*time.Millisecond`:
   - `cmd/bootstrap/reboot_roundtrip_test.go:407`
   - `cmd/bootstrap/reboot_roundtrip_test.go:1021`
   - `cmd/bootstrap/reboot_roundtrip_test.go:1241`
   - `internal/restore/integration_full_test.go:250`
   - `internal/restore/exit_closes_pane_integration_test.go:410`
3. Delete `pollUntilMarkersCleared` (and its docstring) from `cmd/bootstrap/eager_signal_hydrate_integration_test.go:434-451`.
4. Update the two AC1 call sites in `cmd/bootstrap/eager_signal_hydrate_integration_test.go:240` and `:355` to call `restoretest.WaitForSkeletonMarkersCleared(t, client, 2*time.Second, 50*time.Millisecond)`.
5. Verify call site files import `restoretest` (they already do — local helper already calls `restoretest.SortedKeySet`).
6. Run `go build -tags=integration ./...` and `go vet -tags=integration ./...`.

**Acceptance Criteria**:
- `grep -rn "pollUntilMarkersCleared" --include='*.go'` returns zero hits.
- `restoretest.WaitForSkeletonMarkersCleared` signature is `(t, client, timeout, tick)`.
- All seven call sites (two former AC1 + five pre-existing) compile against the new signature.
- AC1 sites pass `2*time.Second, 50*time.Millisecond`; pre-existing sites pass `10*time.Second, 50*time.Millisecond`.
- `go build -tags=integration ./...` succeeds.
- AC1 diagnostic information remains discoverable via the test name and the existing `defer dumpPortalLogOnFailure(t, stateDir)` (the "AC1 violation:" prefix is intentionally dropped — diagnostic, not load-bearing).

**Tests**:
- Existing AC1 integration tests (`cmd/bootstrap/eager_signal_hydrate_integration_test.go` ~lines 240, 355) still fail-fast within the 2-second budget when markers are not cleared and pass within budget when bootstrap is correct.
- Existing reboot-round-trip tests (`cmd/bootstrap/reboot_roundtrip_test.go`) and integration-full + exit-closes-pane tests (`internal/restore/`) using the helper continue to pass.

---

## Task 2: Correct NewRestoreAdapter docstring to remove inaccurate production-site reuse claim
status: approved
severity: low
sources: architecture

**Problem**: `NewRestoreAdapter` in `internal/bootstrapadapter/adapters.go:93-103` carries a docstring justifying production-site open-coded form with: "Production wiring at cmd/bootstrap_production.go retains its open-coded form by design (that site reuses the inner Orchestrator beyond the adapter)." Inspecting `cmd/bootstrap_production.go:111-122`, `restoreInner` is declared once and used exactly once — wrapped at line 122 inside `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}`. No reuse beyond the adapter; the structural-reuse rationale is false. The actual reason production stays open-coded is planning-scope (T6-2 scoped adoption to the four new integration sites), not structural reuse. Same doc-drift class cycle 3's T6-1 paid down for `recordingFIFOSignaler` / `writeFIFOSignal`.

**Solution**: One-line docstring edit replacing the inaccurate carve-out justification with an accurate reason (parity with surrounding inline-struct adapters at the production call site).

**Outcome**: `NewRestoreAdapter`'s docstring no longer asserts a structural-reuse rationale that does not exist in the source. A maintainer reading the constructor's doc and inspecting `cmd/bootstrap_production.go` will not encounter the contradiction.

**Do**:
1. Open `internal/bootstrapadapter/adapters.go`.
2. Locate the `NewRestoreAdapter` docstring (around lines 97-99) — the sentence: "Production wiring at cmd/bootstrap_production.go retains its open-coded form by design (that site reuses the inner Orchestrator beyond the adapter)."
3. Replace with: "Production wiring at cmd/bootstrap_production.go retains its open-coded form for parity with the surrounding inline-struct adapters at that site (HookRegistrar, RestoringMarker, EagerSignalCore, MarkerCleanupCore); migrating it is mechanical and out of scope for the constructor's introduction."
4. Verify: `grep -n "reuses the inner Orchestrator" internal/bootstrapadapter/adapters.go` returns zero hits after edit.
5. Run `go build ./...` and `go vet ./...`.

**Acceptance Criteria**:
- `NewRestoreAdapter`'s docstring no longer claims production reuses the inner Orchestrator beyond the adapter.
- The replacement rationale (parity with sibling inline-struct adapters) accurately describes the four siblings at `cmd/bootstrap_production.go` (HookRegistrar, RestoringMarker, EagerSignalCore, MarkerCleanupCore).
- `grep -rn "reuses the inner Orchestrator" --include='*.go'` returns zero hits.
- `go build ./...` and `go vet ./...` succeed.
- No production wiring change at `cmd/bootstrap_production.go` (doc-only; production migration remains explicitly out of scope).

**Tests**:
- No new tests required — doc-only edit, no behavioural surface.
- Existing `internal/bootstrapadapter` tests pass (`go test ./internal/bootstrapadapter/...`).
- Full unit suite green (`go test ./...`).
