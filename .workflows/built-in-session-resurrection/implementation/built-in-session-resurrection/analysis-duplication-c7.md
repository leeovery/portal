---
agent: duplication
cycle: 7
findings_count: 1
status: issues_found
---
# Duplication Analysis (Cycle 7)

## Summary

One actionable consistency finding (state_cleanup.go's `openNoRotateLogger` lacks the `defer Close` pattern its four siblings use). Two near-duplicate helpers below Rule-of-Three threshold; openPathFunc seam and spec wording harmonization verified clean. Cycle-6's D1/D2 extraction (`internal/restoretest/`) successfully landed.

---

## Duplications Found

### FINDING D1: `openNoRotateLogger` result discarded without `defer Close` in `state_cleanup.go`

- **SEVERITY:** low (consistency)
- **FILES:**
  - `cmd/state_cleanup.go:49` (acquires, no defer)
  - `cmd/state_signal_hydrate.go:150-151`
  - `cmd/state_hydrate.go:366-367`
  - `cmd/state_notify.go:47-48`
  - `cmd/state_migrate_rename.go:33-34`
- **DESCRIPTION:** Phase 13 task 13-5 doc-touched `state_cleanup.go` but did not normalize the logger lifecycle. Every other cmd RunE that calls `openNoRotateLogger()` pairs it with `defer func() { _ = logger.Close() }()` to flush the buffered writer. `buildStateCleanupDeps` returns the `*state.Logger` to the cobra `RunE` without a deferred Close anywhere — INFO/WARN lines emitted by `killSaver` and `purgeStateDir` may never flush before process exit.
- **RECOMMENDATION:** Add `defer func() { _ = logger.Close() }()` in `cmd/state_cleanup.go`'s RunE after the `client, unregister, logger := buildStateCleanupDeps()` call. Single-line fix; brings `state_cleanup` into the same shape as the other four RunE bodies. The nil-receiver no-op semantics make Close safe even when test injection passes a nil logger.

## Near-Duplicates

### FINDING D2: Single-use scoped marker-clearance helper shaped almost identically to `WaitForSkeletonMarkersCleared`

- **SEVERITY:** low
- **FILES:**
  - `cmd/bootstrap/reboot_roundtrip_test.go:913-937` (`waitForSessionMarkerCleared`)
  - `internal/restoretest/restoretest.go:293-308` (`WaitForSkeletonMarkersCleared`)
- **DESCRIPTION:** T13-2 added `waitForSessionMarkerCleared(t, client, session, timeout)` as a session-scoped predicate variant. Both bodies repeat the same poll-loop shape: deadline = now+timeout, periodic `ListSkeletonMarkers`, sleep 50ms, fatal on expiry with a `SortedKeySet` diagnostic. The session predicate adds a `strings.HasPrefix(k, prefix)` filter; otherwise the loop is identical. Single call site today.
- **RECOMMENDATION:** No action. If a second consumer ever copies this helper, promote it to `restoretest.WaitForSessionMarkersCleared(t, client, session, timeout)` (with `session=""` matching all). Below Rule-of-Three.

### FINDING D3: `verifySwitchClientLiveStructure` is a hard-coded subset of `verifyLiveStructure`

- **SEVERITY:** low
- **FILES:**
  - `cmd/bootstrap/reboot_roundtrip_test.go:564-590` (`verifyLiveStructure`)
  - `cmd/bootstrap/reboot_roundtrip_test.go:944-959` (`verifySwitchClientLiveStructure`)
- **DESCRIPTION:** T13-2 added `verifySwitchClientLiveStructure` for the new switch-client subtest. It asserts `list-sessions` contains alpha+beta and that each session has 0:0 in `list-panes` — the exact shape `verifyLiveStructure` already implements parameterized by `roundTripCfg`. The new helper hard-codes both session names and pane index. Single use site today.
- **RECOMMENDATION:** No action this cycle. If a third sub-test ever needs a structural-equivalence check at default base-index, fold both into a parametric form. Below Rule-of-Three.

## Verification (no action)

- **`openPathFunc` / `openTUIFunc` seam pattern internally consistent** — T13-3's openPathFunc follows the existing openTUIFunc pattern: package-level vars, init() initialization, identical override-and-restore-via-t.Cleanup test pattern, matching compile-time signature assertions. Clean replication.
- **Spec/error wording harmonization complete** — both warning strings appear byte-identically across production source (errors.go:54-69), test assertions (errors_test.go:67-97, bootstrap_warnings_test.go:222-227), and the spec (specification.md:1372-1380). No file missed.
