# Review Report: built-in-session-resurrection-13-1

**TASK**: Extract shared reboot-round-trip + portal-binary test helpers into `internal/restoretest/`

**ACCEPTANCE CRITERIA**:
- Eliminate ~150 lines of duplication across `cmd/bootstrap/reboot_roundtrip_test.go`, `cmd/reattach_integration_test.go`, and `internal/restore/integration_full_test.go`.
- Avoid retry-budget drift hazard (single canonical implementation).
- Apply build-tag scoping so the package contributes zero compile cost / zero surface to default `go test ./...`.
- Avoid an import cycle against `internal/tmux`.

**STATUS**: Complete

**SPEC CONTEXT**:
Phase 13 (Analysis Cycle 6) remediation task. The relevant spec touchpoint is the production retry ladder (`Signal Mechanism: FIFO Per Pane`, 10/20/40/80/160/190 ms = 500 ms total) which the test fallback intentionally diverges from for CI flake tolerance.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/restoretest/restoretest.go` (319 lines, single file, package `restoretest`)
- Notes:
  - Build tag `//go:build integration` is applied at line 1.
  - Package depends only on `os`, `os/exec`, `path/filepath`, `sort`, `syscall`, `testing`, `time`, `errors`, `fmt`, plus `internal/state` and `internal/tmux`. No import cycle.
  - Exported helpers: `ProjectRoot`, `BuildPortalBinaryDir`, `BuildPortalBinaryStable`, `PrependPATH`, `DriveSignalHydrate`, `DriveSignalHydrateBinary`, `WaitForSkeletonMarkersCleared`, `SortedKeySet`. Unexported helpers: `buildPortalBinaryInto`, `openAndSignalFIFO`.
  - All three previously-duplicated test files now import `github.com/leeovery/portal/internal/restoretest` and call its helpers.
  - Two flavours of "build the portal CLI" are documented and justified.
  - Retry-budget drift hazard is avoided: `DriveSignalHydrate`'s 50 ms × 10 s budget is now defined once and explicitly justified.

**TESTS**:
- Status: Adequate
- Coverage: This is a test-helper package; correctness is exercised transitively by every integration test that imports it. Three callers wired:
  - `cmd/bootstrap/reboot_roundtrip_test.go` exercises `BuildPortalBinaryDir`, `PrependPATH`, `DriveSignalHydrate`, `DriveSignalHydrateBinary`, `WaitForSkeletonMarkersCleared`, `SortedKeySet`.
  - `internal/restore/integration_full_test.go` exercises `BuildPortalBinaryDir`, `PrependPATH`, `DriveSignalHydrate`, `WaitForSkeletonMarkersCleared`.
  - `cmd/reattach_integration_test.go` exercises `BuildPortalBinaryStable` (under `sync.Once`).
- Notes:
  - No dedicated unit tests for the helpers themselves. Acceptable for a test-only support package.

**CODE QUALITY**:
- Project conventions: Followed. Standard Go layout; small functions; no naked returns; explicit error wrapping with `%w`; `t.Helper()` on every helper that takes `*testing.T`.
- SOLID: Good. Single-purpose functions; the two-flavour `BuildPortalBinaryDir`/`BuildPortalBinaryStable` split is a deliberate dependency-inversion (lifetime ownership); both delegate to a single `buildPortalBinaryInto` private function (DRY).
- Complexity: Low.
- Modern idioms: Yes. Uses `errors.Is`, `fmt.Errorf` with `%w`, `t.Setenv` (auto-restore), `t.Helper()`, `t.TempDir()`, `t.Fatalf`/`t.Errorf` distinction.
- Readability: Excellent. Each function carries a meaningful godoc explaining the rationale.
- Issues: None blocking.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [quickfix] `internal/restoretest/restoretest.go:54` — `ProjectRoot` is exported but only called from the unexported `buildPortalBinaryInto`. Phase 14 task 14-3 already followed up on a sibling case; consider unexporting `ProjectRoot` for consistency.
- [idea] `restoretest.go:163` — `DriveSignalHydrate`'s 10-second budget and 50 ms cadence are encoded as untyped local constants. Lift to package-level named constants (e.g. `FallbackRetryDelay`, `FallbackRetryBudget`) for clarity.
- [idea] `restoretest.go:218` — `DriveSignalHydrateBinary`'s `cmd.Env` construction concatenates `os.Environ()` then appends overrides. Brief comment confirming overrides win over inherited environment would harden against accidental future reordering.
