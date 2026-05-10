# Analysis Tasks: killed-sessions-resurrect-on-restart (Cycle 6)

- topic: killed-sessions-resurrect-on-restart
- cycle: 6
- total_proposed: 2

---

## Task 1: Collapse open-coded RestoreAdapter preamble and openTestLogger shim across cmd/bootstrap tests
status: approved
severity: low
sources: duplication, architecture

**Problem**: Two adjacent half-migrations remain in cmd/bootstrap test files. (a) Five sites in `cmd/bootstrap/phase5_integration_test.go` (lines 167-176, 261-270) and `cmd/bootstrap/reboot_roundtrip_test.go` (lines 326-334, 937-945, 1189-1203) still open-code the `restoreInner := &restore.Orchestrator{Client, StateDir, Logger}` + `&bootstrapadapter.RestoreAdapter{Inner: restoreInner}` two-step preamble that `bootstrapadapter.NewRestoreAdapter` exists to collapse — these files meet the "actively rewritten by this work unit" criterion (T1-4, T4-2, T4-4, T7-1) that motivated cycle 5's reattach migration. (b) The 12 in-package call sites in cmd/bootstrap call `openTestLogger`, a one-line shim at `cmd/bootstrap/orchestrator_builder_test.go:112-120` whose entire body is `return restoretest.OpenTestLogger(t, stateDir)` — a same-named-symbol-in-two-places drift surface that T7-1's precedent (collapse all call sites in one go) avoided. Both migrations target overlapping files, so consolidating them in a single pass minimises git churn.

**Solution**: In one pass, migrate both patterns across the six cmd/bootstrap test files. Replace each open-coded `restore.Orchestrator{...}` + `RestoreAdapter{Inner: restoreInner}` pair with `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)`. Find/replace `openTestLogger(` → `restoretest.OpenTestLogger(` across all 12 in-package call sites. Delete the shim wrapper at `orchestrator_builder_test.go:112-120`. Drop newly-unused imports.

**Outcome**: `restoretest.OpenTestLogger` is the single named call site for the helper across the entire codebase. `bootstrapadapter.NewRestoreAdapter` is adopted at every test site touched by this work unit (production wiring at `cmd/bootstrap_production.go` remains exempt per the constructor docstring). Net deletion: ~30 lines for the RestoreAdapter collapse + ~9 lines for the shim removal, plus unused imports.

**Do**:
1. In `cmd/bootstrap/phase5_integration_test.go` at lines 167-176 and 261-270, replace the `restoreInner := &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}` block plus `Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner}` field with `Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)`.
2. Repeat the same collapse in `cmd/bootstrap/reboot_roundtrip_test.go` at lines 326-334, 937-945, and 1189-1203.
3. Drop `"github.com/leeovery/portal/internal/restore"` import from both files if no other reference remains. Do not touch `orchestrator_builder_eager_default_test.go` lines 50, 88 — those use deliberately empty zero-value sentinels.
4. Find/replace `openTestLogger(` → `restoretest.OpenTestLogger(` across the 12 in-package call sites in cmd/bootstrap. Files containing call sites: `eager_signal_hydrate_integration_test.go`, `phase2_hook_fire_integration_test.go`, `scrollback_resumption_test.go`, `phase5_integration_test.go`, `phase5_marker_suppression_integration_test.go`, `reboot_roundtrip_test.go`.
5. Delete the wrapper function and its doc-comment at `cmd/bootstrap/orchestrator_builder_test.go:112-120`.
6. If the `state` import in `orchestrator_builder_test.go` becomes orphaned after wrapper removal, drop it (verify by build).
7. Ensure `"github.com/leeovery/portal/internal/restoretest"` is imported wherever the new call form is used.
8. Run `go build ./...` and `go test ./cmd/bootstrap/...`.

**Acceptance Criteria**:
- All five RestoreAdapter open-coded preambles in `phase5_integration_test.go` and `reboot_roundtrip_test.go` collapsed to `bootstrapadapter.NewRestoreAdapter(client, stateDir, logger)`.
- `grep -n "restore.Orchestrator{" cmd/bootstrap/phase5_integration_test.go cmd/bootstrap/reboot_roundtrip_test.go` returns no hits.
- `grep -rn "openTestLogger(" cmd/bootstrap/` returns no hits.
- The `openTestLogger` function in `orchestrator_builder_test.go` is deleted.
- `restoretest.OpenTestLogger` is the only named call site for the test-logger helper across the codebase.
- Production wiring at `cmd/bootstrap_production.go` is untouched.
- `go build ./...` succeeds; all cmd/bootstrap tests pass.

**Tests**:
- `go test ./cmd/bootstrap/...` — all tests in migrated files still pass.
- `go test ./...` — no broader regression.

---

## Task 2: Delete duplicate TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags in cmd-package
status: approved
severity: low
sources: duplication

**Problem**: `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` at `cmd/state_signal_hydrate_test.go:316-350` is byte-equivalent in assertions to `TestOpenFIFOForSignal_NonBlockingFlags` at `internal/state/signal_hydrate_test.go:244-274`. Both target the same production function `state.OpenFIFOForSignal` and run identical assertions: skip on Windows, mkfifo at fresh temp path, call `state.OpenFIFOForSignal` against a no-reader FIFO, record elapsed, assert `errors.Is(err, syscall.ENXIO)`, assert `elapsed < 100*time.Millisecond` proving `O_NONBLOCK`. Bodies differ only in failure-message prefix and path construction style. The cmd-package copy is redundant cross-package primitive coverage; canonical home is internal/state next to the function under test. Same drift class cycle 2 addressed for `RecordingFIFOSignaler`, applied here to the test itself.

**Solution**: Delete the redundant cmd-package test. Preserve the canonical state-package test.

**Outcome**: Single source of coverage for `state.OpenFIFOForSignal` non-blocking-flag behaviour at `internal/state/signal_hydrate_test.go`. Cmd-package retains its legitimate end-to-end signal-hydrate-via-cobra-Execute test at line 405. Net deletion: ~35 lines plus any now-unused imports.

**Do**:
1. Delete `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` (lines 316-350) from `cmd/state_signal_hydrate_test.go`.
2. Verify whether `"runtime"` and `"syscall"` imports in `cmd/state_signal_hydrate_test.go` have other consumers; if not, drop them.
3. Run `go build ./...` and `go test ./cmd/... ./internal/state/...`.

**Acceptance Criteria**:
- `TestSignalHydrate_OpenFIFOForSignalUsesNonBlockingFlags` removed from `cmd/state_signal_hydrate_test.go`.
- `TestOpenFIFOForSignal_NonBlockingFlags` in `internal/state/signal_hydrate_test.go` unchanged and passing.
- Any now-unused imports dropped.
- `go build ./...` succeeds; `go test ./cmd/... ./internal/state/...` passes.

**Tests**:
- `go test -run TestOpenFIFOForSignal_NonBlockingFlags ./internal/state/...` confirms canonical coverage persists.
- `go test ./cmd/...` confirms cmd-package suite still passes (cobra-Execute integration test at line 405 still covers the cmd-layer concern).
