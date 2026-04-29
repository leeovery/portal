---
agent: duplication
cycle: 6
findings_count: 4
status: issues_found
---
# Duplication Analysis (Cycle 6)

## Summary

Two new integration test files copy-paste ~115 lines of identical helpers; both build the portal binary independently from a third reattach-test variant. Production-side logger-defer pattern is consistent boilerplate, not duplication.

---

## Duplications Found

### FINDING D1: Reboot round-trip helper functions duplicated verbatim across two integration test files

- **SEVERITY:** high
- **FILES:** `cmd/bootstrap/reboot_roundtrip_test.go:511-573, 583-617, 783-825`; `internal/restore/integration_full_test.go:521-580, 588-621, 628-671`
- **DESCRIPTION:** Seven helpers appear in both files with byte-identical bodies (modulo doc-comment wording):
  - `driveSignalHydrate(t, client, stateDir, sessions)` (~30 lines, marker-listing + FIFO byte-write loop)
  - `openAndSignalFIFO(path, delay, budget)` (~22 lines, ENXIO/EAGAIN retry ladder, deadline). Both copies even include the same self-aware comment "duplicated here (rather than imported)".
  - `waitForSkeletonMarkersCleared(t, client, timeout)` (~15 lines, marker-clearance poll)
  - `sortedKeySet(map[string]struct{})` (~14 lines, n^2 sort)
  - `buildPortalBinaryDir(t)` (~12 lines, `go build -o portal .`)
  - `prependPathDir(t, dir)` (4 lines, PATH prefix via `t.Setenv`)
  - `projectRoot(t)` (~17 lines, walk-up to go.mod)

  Total ~115 lines of test infra copy-pasted. The duplication is acknowledged in source comments but no shared home is provided — pure drift hazard (e.g. budget tweaks to one copy but not the other).
- **RECOMMENDATION:** Extract into a new `internal/restoretest/` package (or grow `internal/tmuxtest/` with a `restoretest.go`) exporting:
  - `BuildPortalBinaryDir(t) string`, `PrependPATH(t, dir)`, `ProjectRoot(t) string`
  - `DriveSignalHydrate(t, *tmux.Client, stateDir, []string)`
  - `OpenAndSignalFIFO(path, delay, budget) error`
  - `WaitForSkeletonMarkersCleared(t, *tmux.Client, timeout)`
  - `SortedKeySet(map[string]struct{}) []string`

  Both `_test.go` files import; the package itself can be `//go:build integration`. A shared sibling test-support package introduces no circular imports — the package-isolation argument doesn't hold.

### FINDING D2: Three near-identical `go build portal` helpers across integration tests

- **SEVERITY:** medium
- **FILES:** `cmd/reattach_integration_test.go:116-153` (`buildPortalBinaryForReattach` + `reattachProjectRoot`); `cmd/bootstrap/reboot_roundtrip_test.go:783-825`; `internal/restore/integration_full_test.go:628-671`
- **DESCRIPTION:** Phase 12 added three variants of "compile portal CLI for integration tests":
  - `buildPortalBinaryForReattach()` — `os.MkdirTemp` for process-lifetime, returns `(string, error)`.
  - `buildPortalBinaryDir(t)` × 2 — `t.TempDir`, t-fatal on error (byte-identical between bootstrap and restore copies).

  All three resolve project root by walking up to `go.mod` (~17 lines each, in two flavours: `reattachProjectRoot` returns error; `projectRoot` t-fatals). The reattach variant's only meaningful differences are (a) `os.MkdirTemp` for sync.Once survivability across sub-tests and (b) error-returning for use inside `sync.Once.Do`.
- **RECOMMENDATION:** Consolidate alongside the round-trip helpers in `internal/restoretest/`. Provide both `BuildPortalBinaryDir(t) string` (t.TempDir-based) and `BuildPortalBinaryStable() (string, error)` (os.MkdirTemp-based for sync.Once callers), backed by a single `ProjectRoot() (string, error)`. This subsumes `buildPortalBinaryForReattach`, `reattachProjectRoot`, and both `projectRoot`/`buildPortalBinaryDir` copies.

## Near-Duplicates

### FINDING D3: `openNoRotateLogger() + defer Close` idiom repeated in five command files

- **SEVERITY:** low
- **FILES:** `cmd/state_signal_hydrate.go:149-150`, `cmd/state_hydrate.go:366-367`, `cmd/state_notify.go:47-48`, `cmd/state_migrate_rename.go:33-34`, `cmd/state_cleanup.go:49` (plus pre-existing `cmd/bootstrap_production.go:96`)
- **DESCRIPTION:** Five Phase 12 RunE bodies repeat:
  ```go
  logger, _ := openNoRotateLogger()
  defer func() { _ = logger.Close() }()
  ```
  This is consistent boilerplate, not copy-paste drift: `openNoRotateLogger` already centralises the open path, and inlining the defer at each RunE preserves the documented nil-receiver-is-no-op contract and keeps the logger close-scope tied to the cobra handler. Wrapping in a `withLogger(func(*state.Logger) error)` would lose readability for two lines.
- **RECOMMENDATION:** No action. Below the extraction threshold; pattern is framework-shaped cobra-handler boilerplate.

### FINDING D4: Round-trip verification helpers exist in both files with similar shape but different fixture types

- **SEVERITY:** low
- **FILES:** `cmd/bootstrap/reboot_roundtrip_test.go:622-755`, `internal/restore/integration_full_test.go:367-512`
- **DESCRIPTION:** Both files contain `verifyLiveStructure`, `verifyANSIInScrollback`/`verifyANSIScrollback`, and a session-env check with similar list-sessions/capture-pane/show-environment substring assertions. The bootstrap version is parameterised by `roundTripCfg` (base-index pairs); the restore version is variadic over `fixtureSession`. The data shapes diverge enough that DRYing would require generics or parallel test-only structs — net cost likely exceeds benefit since each file's granularity matches its round-trip's needs.
- **RECOMMENDATION:** No action. Similarity is at the "both verify the same dimensions" conceptual level, not copy-pasted executable code.

## Extraction Candidates

Highest-value extraction: the seven duplicated test helpers in the two reboot round-trip files plus the reattach-test build helpers — combined ~150 lines of test infrastructure now copy-pasted across three files (`cmd/bootstrap/reboot_roundtrip_test.go`, `internal/restore/integration_full_test.go`, `cmd/reattach_integration_test.go`). A single `internal/restoretest/` (or `internal/tmuxtest/restoretest.go`) package eliminates all drift hazards and lets future round-trip tests reuse the harness.
